package kaggle

import (
	"encoding/json"
	"testing"
)

func TestSubmissionScored(t *testing.T) {
	if (Submission{Status: StatusPending}).Scored() {
		t.Error("nil PublicScore: Scored() should be false")
	}
	score := 0.775
	if !(Submission{Status: StatusComplete, PublicScore: &score}).Scored() {
		t.Error("set PublicScore: Scored() should be true")
	}
}

func TestSubmissionJSONRoundTrip(t *testing.T) {
	score := 0.775
	in := Submission{
		Competition: "titanic",
		File:        "submission.csv",
		Message:     "baseline",
		SubmittedAt: "2026-07-01T12:00:00Z",
		Status:      StatusComplete,
		PublicScore: &score,
	}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out Submission
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if out.File != in.File || out.Status != in.Status || out.Competition != in.Competition ||
		out.Message != in.Message || out.SubmittedAt != in.SubmittedAt {
		t.Fatalf("round-trip field mismatch:\n got %+v\nwant %+v", out, in)
	}
	if out.PublicScore == nil || *out.PublicScore != *in.PublicScore {
		t.Fatalf("PublicScore lost in round-trip: %v", out.PublicScore)
	}
}
