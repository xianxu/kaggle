package kaggle

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stripComments drops the authored fixture's `#` provenance header — the
// production parser is fixture-agnostic (no comment handling), so the test that
// feeds it the fixture strips the header itself.
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
	if len(subs) != 3 {
		t.Fatalf("want 3 submissions, got %d: %+v", len(subs), subs)
	}
	// Row 0 is the newest (pending) — no score.
	if subs[0].Status != StatusPending || subs[0].Scored() {
		t.Errorf("row0 want pending+unscored, got %+v", subs[0])
	}
	if subs[1].File != "submission_v2.csv" || !subs[1].Scored() || *subs[1].PublicScore != 0.78229 {
		t.Errorf("row1 want scored submission_v2 @0.78229, got %+v", subs[1])
	}
	// LatestScored skips the newest (pending) and returns the newest SCORED row.
	best, ok := LatestScored(subs)
	if !ok {
		t.Fatal("LatestScored: want ok=true")
	}
	if best.File != "submission_v2.csv" || *best.PublicScore != 0.78229 {
		t.Errorf("LatestScored = %+v, want submission_v2 @0.78229", best)
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

// Column order must not matter — parsing is header-driven.
func TestParseSubmissionsReordered(t *testing.T) {
	csv := "status,fileName,publicScore\ncomplete,x.csv,0.5\n"
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
		{File: "a.csv", SubmittedAt: "2026-07-01T15:00:00Z", Message: "pending", Status: StatusPending},
		{File: "b.csv", SubmittedAt: "2026-07-01T12:00:00Z", Message: "done", Status: StatusComplete, PublicScore: &score},
	}
	out, err := ParseSubmissions(FormatSubmissionsCSV(in))
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 || out[0].File != "a.csv" || out[1].File != "b.csv" {
		t.Fatalf("round-trip file mismatch: %+v", out)
	}
	if out[0].Scored() {
		t.Error("pending row should round-trip unscored")
	}
	if !out[1].Scored() || *out[1].PublicScore != score {
		t.Errorf("scored row lost score: %+v", out[1])
	}
}
