package kaggle

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseSubmissions(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "submissions.csv"))
	if err != nil {
		t.Fatal(err)
	}
	subs, err := ParseSubmissions(string(raw))
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
