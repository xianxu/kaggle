package kaggle

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stripComments drops the fixture's `#` provenance header — the production parser
// is fixture-agnostic (no comment handling), so the test that feeds it the fixture
// strips the header itself.
func stripComments(s string) string {
	var keep []string
	for _, ln := range strings.Split(s, "\n") {
		if strings.HasPrefix(strings.TrimSpace(ln), "#") {
			continue
		}
		keep = append(keep, ln)
	}
	return strings.Join(keep, "\n")
}

func TestParseSubmissions(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "submissions.csv"))
	if err != nil {
		t.Fatal(err)
	}
	subs, err := ParseSubmissions(stripComments(string(raw)))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(subs) != 5 {
		t.Fatalf("want 5 submissions, got %d: %+v", len(subs), subs)
	}
	// Row 0 is the newest CAPTURED row: complete + scored, with the live wire spelling
	// normalized and the raw preserved.
	// A QUOTED, comma-bearing description must round-trip intact — 8 of the 18 captured
	// rows have one, and a parser rewritten on strings.Split would pass every other test
	// in this suite and then break on the first live poll.
	var quoted *Submission
	for i := range subs {
		if subs[i].Ref == "54996757" {
			quoted = &subs[i]
		}
	}
	if quoted == nil {
		t.Fatal("fixture lost the quoted comma-bearing row (54996757)")
	}
	if !strings.Contains(quoted.Message, "LOO 9.164, matched-CV 9.544") {
		t.Errorf("quoted description lost its embedded commas: %q", quoted.Message)
	}

	if subs[0].Ref != "55058575" || subs[0].File != "submission.csv" {
		t.Errorf("row0 ref/file wrong: %+v", subs[0])
	}
	if subs[0].Status != StatusComplete {
		t.Errorf("row0 status should NORMALIZE to %q, got %q", StatusComplete, subs[0].Status)
	}
	if subs[0].StatusRaw != "SubmissionStatus.COMPLETE" {
		t.Errorf("row0 should preserve the wire spelling, got %q", subs[0].StatusRaw)
	}
	if !subs[0].Scored() || *subs[0].PublicScore != 9.662 {
		t.Errorf("row0 want scored @9.662, got %+v", subs[0])
	}
	// The live date shape: space-separated with microseconds, NO timezone.
	if subs[0].SubmittedAt != "2026-07-28 15:17:26.513000" {
		t.Errorf("row0 date shape drifted: %q", subs[0].SubmittedAt)
	}

	// DIVERGENCE 4 (captured, kaggle#8): COMPLETE does NOT imply scored. Submission
	// 54846753 is complete with an empty publicScore — Scored() must be the test, and
	// LatestScored must skip it.
	var unscoredComplete *Submission
	for i := range subs {
		if subs[i].Ref == "54846753" {
			unscoredComplete = &subs[i]
		}
	}
	if unscoredComplete == nil {
		t.Fatal("fixture lost the complete-but-unscored row (54846753)")
	}
	if unscoredComplete.Status != StatusComplete {
		t.Errorf("54846753 should be complete, got %q", unscoredComplete.Status)
	}
	if unscoredComplete.Scored() {
		t.Error("54846753 is COMPLETE with an empty publicScore — Scored() must be false")
	}

	// LatestScored returns the newest row carrying a score, skipping both the pending
	// row and the complete-but-unscored one.
	best, ok := LatestScored(subs)
	if !ok {
		t.Fatal("LatestScored: want ok=true")
	}
	if best.Ref != "55058575" || *best.PublicScore != 9.662 {
		t.Errorf("LatestScored = %+v, want 55058575 @9.662", best)
	}
	// All-pending → ok=false.
	if _, ok := LatestScored([]Submission{{Status: StatusPending}}); ok {
		t.Error("LatestScored over all-pending: want ok=false")
	}
	// Empty input → nil, no error.
	if got, err := ParseSubmissions("  "); err != nil || got != nil {
		t.Errorf("empty input: want nil,nil got %v,%v", got, err)
	}
}

// The pending path: the live capture was taken after everything scored, so the
// pending-unscored-LEADS case (which is what pollScore actually sees) is pinned with
// a synthetic table rather than lost in the fixture swap.
func TestLatestScoredSkipsLeadingPending(t *testing.T) {
	csv := "ref,fileName,status,publicScore\n" +
		"3,submission.csv,SubmissionStatus.PENDING,\n" +
		"2,submission.csv,SubmissionStatus.COMPLETE,0.81\n"
	subs, err := ParseSubmissions(csv)
	if err != nil {
		t.Fatal(err)
	}
	if subs[0].Status != StatusPending || subs[0].Scored() {
		t.Fatalf("row0 should be pending+unscored: %+v", subs[0])
	}
	best, ok := LatestScored(subs)
	if !ok || best.Ref != "2" || *best.PublicScore != 0.81 {
		t.Errorf("LatestScored should skip the leading pending row, got %+v ok=%v", best, ok)
	}
}

// NormalizeStatus is TOTAL: it reconciles the wire vocabulary, and an UNKNOWN status
// must pass through normalized-but-preserved rather than being rejected or coerced.
// This package once shipped three constants that matched no live value (kaggle#8);
// this is the regression that keeps the tolerance.
func TestNormalizeStatus(t *testing.T) {
	cases := map[string]string{
		"SubmissionStatus.COMPLETE":     StatusComplete,
		"SubmissionStatus.PENDING":      StatusPending,
		"submissionstatus.complete":     StatusComplete, // case-folded prefix
		"  SubmissionStatus.COMPLETE  ": StatusComplete,
		"complete":                      StatusComplete, // already normalized
		"SubmissionStatus.WEIRD":        "weird",        // unknown: preserved, not rejected
		"":                              "",
	}
	for in, want := range cases {
		if got := NormalizeStatus(in); got != want {
			t.Errorf("NormalizeStatus(%q) = %q, want %q", in, got, want)
		}
	}
}

// An unrecognized status must survive a full parse with the raw value intact.
func TestParseSubmissionsUnknownStatusPreserved(t *testing.T) {
	csv := "ref,fileName,status,publicScore\n9,submission.csv,SubmissionStatus.QUARANTINED,\n"
	subs, err := ParseSubmissions(csv)
	if err != nil {
		t.Fatalf("an unknown status must not fail the parse: %v", err)
	}
	if subs[0].Status != "quarantined" || subs[0].StatusRaw != "SubmissionStatus.QUARANTINED" {
		t.Errorf("unknown status not preserved: %+v", subs[0])
	}
}

// Ref is optional: a pre-kaggle#8 capture (no ref column) must still parse.
func TestParseSubmissionsRefOptional(t *testing.T) {
	csv := "fileName,status,publicScore\nsubmission.csv,SubmissionStatus.COMPLETE,0.5\n"
	subs, err := ParseSubmissions(csv)
	if err != nil {
		t.Fatal(err)
	}
	if len(subs) != 1 || subs[0].Ref != "" || subs[0].Status != StatusComplete {
		t.Fatalf("ref-less parse wrong: %+v", subs)
	}
}

// Column order must not matter — parsing is header-driven.
func TestParseSubmissionsReordered(t *testing.T) {
	csv := "status,fileName,publicScore\nSubmissionStatus.COMPLETE,x.csv,0.5\n"
	subs, err := ParseSubmissions(csv)
	if err != nil {
		t.Fatal(err)
	}
	if len(subs) != 1 || subs[0].File != "x.csv" || subs[0].Status != StatusComplete ||
		subs[0].PublicScore == nil || *subs[0].PublicScore != 0.5 {
		t.Fatalf("reordered parse wrong: %+v", subs)
	}
}

// A single non-numeric publicScore (e.g. "-" on an errored/pending row) must NOT
// discard the valid rows — it degrades that one row to unscored. This is the
// fragile boundary M2's poll loop leans on (review FIX-THEN-SHIP Important).
func TestParseSubmissionsBadScoreDegrades(t *testing.T) {
	csv := "fileName,status,publicScore\n" +
		"good.csv,complete,0.81\n" +
		"weird.csv,error,-\n"
	subs, err := ParseSubmissions(csv)
	if err != nil {
		t.Fatalf("a bad score must not fail the whole parse: %v", err)
	}
	if len(subs) != 2 {
		t.Fatalf("want both rows kept, got %d: %+v", len(subs), subs)
	}
	if !subs[0].Scored() || *subs[0].PublicScore != 0.81 {
		t.Errorf("good row lost its score: %+v", subs[0])
	}
	if subs[1].Scored() {
		t.Errorf("row with non-numeric score should be unscored: %+v", subs[1])
	}
	if best, ok := LatestScored(subs); !ok || best.File != "good.csv" {
		t.Errorf("LatestScored should find good.csv past the bad row, got %+v ok=%v", best, ok)
	}
}

// A header lacking the fileName column is structurally malformed → error (we can't
// identify submissions without it).
func TestParseSubmissionsMissingFileNameColumn(t *testing.T) {
	csv := "status,publicScore\ncomplete,0.5\n"
	if _, err := ParseSubmissions(csv); err == nil {
		t.Fatal("want an error when the fileName column is missing")
	}
}

// FormatSubmissionsCSV (used by the fake) and ParseSubmissions share one schema —
// this round-trip is what keeps fake and parser from drifting apart.
func TestFormatSubmissionsRoundTrip(t *testing.T) {
	score := 0.775
	in := []Submission{
		{Ref: "101", File: "submission.csv", SubmittedAt: "2026-07-01 15:00:00.000000", Message: "pending",
			Status: StatusPending, StatusRaw: "SubmissionStatus.PENDING"},
		{Ref: "100", File: "submission.csv", SubmittedAt: "2026-07-01 12:00:00.000000", Message: "done",
			Status: StatusComplete, StatusRaw: "SubmissionStatus.COMPLETE", PublicScore: &score},
	}
	rendered := FormatSubmissionsCSV(in)
	// The fake renders the WIRE spelling, so consumers of the fake see real shapes.
	if !strings.Contains(rendered, "SubmissionStatus.COMPLETE") {
		t.Errorf("rendered CSV should carry the wire status spelling:\n%s", rendered)
	}
	out, err := ParseSubmissions(rendered)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 || out[0].Ref != "101" || out[1].Ref != "100" {
		t.Fatalf("round-trip ref mismatch: %+v", out)
	}
	if out[0].Status != StatusPending || out[1].Status != StatusComplete {
		t.Errorf("round-trip status mismatch: %+v", out)
	}
	if out[0].Scored() {
		t.Error("pending row should round-trip unscored")
	}
	if !out[1].Scored() || *out[1].PublicScore != score {
		t.Errorf("scored row lost score: %+v", out[1])
	}
}

// The exported header is what the live-conformance test asserts against; it must
// stay the single definition (ARCH-DRY) and must not be mutable by callers.
func TestSubmissionsCSVHeaderExported(t *testing.T) {
	h := SubmissionsCSVHeader()
	if len(h) == 0 || h[0] != "ref" {
		t.Fatalf("exported header wrong: %v", h)
	}
	h[0] = "mutated"
	if SubmissionsCSVHeader()[0] != "ref" {
		t.Error("SubmissionsCSVHeader must return a copy, not the package var")
	}
}
