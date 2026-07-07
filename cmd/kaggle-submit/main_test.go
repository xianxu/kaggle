package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/xianxu/kaggle/internal/kaggletest"
	"github.com/xianxu/kaggle/pkg/kaggle"
)

// TestRun_SubmitsAndPollsToScore is the subprocess-integration proof: run() drives
// the real fake through submit → poll → scored, writing a scored submission.json +
// metrics.json{public_score}. KAGGLE_FAKE_SCORE_AFTER=1 forces the poll to iterate.
func TestRun_SubmitsAndPollsToScore(t *testing.T) {
	kaggletest.WireFake(t, t.TempDir())
	t.Setenv("KAGGLE_FAKE_SCORE_AFTER", "1") // poll #1 pending, poll #2 scored
	t.Setenv("KAGGLE_SUBMIT_MAX_ATTEMPTS", "5")
	t.Setenv("KAGGLE_SUBMIT_DELAY", "0")

	stepDir, runDir := kaggletest.WireStep(t, "submit",
		`{"competition":{"slug":"titanic"},"submission":"make-submission","message":"e2e"}`)
	kaggletest.WriteUpstream(t, runDir, "make-submission", submissionFile, "PassengerId,Survived\n892,0\n")

	if err := run(); err != nil {
		t.Fatalf("run: %v", err)
	}
	var sub kaggle.Submission
	kaggletest.ReadJSON(t, filepath.Join(stepDir, "submission.json"), &sub)
	if !sub.Scored() {
		t.Fatalf("submission.json not scored: %+v", sub)
	}
	if sub.Competition != "titanic" {
		t.Errorf("submission.competition = %q, want titanic", sub.Competition)
	}
	var m map[string]float64
	kaggletest.ReadJSON(t, filepath.Join(stepDir, "metrics.json"), &m)
	if m["public_score"] != *sub.PublicScore {
		t.Errorf("metrics public_score = %v, want %v", m["public_score"], *sub.PublicScore)
	}
}

// TestRun_ReportsOwnScoreWithPriorScoredSubmission is the end-to-end regression
// for the close-review Critical, through the REAL fake: a competition that already
// has a scored submission (KAGGLE_FAKE_PRIOR_SCORE=0.99). submit must report OUR
// upload's score (fakeScore 0.775), never the pre-existing 0.99.
func TestRun_ReportsOwnScoreWithPriorScoredSubmission(t *testing.T) {
	kaggletest.WireFake(t, t.TempDir())
	t.Setenv("KAGGLE_FAKE_SCORE_AFTER", "1")
	t.Setenv("KAGGLE_FAKE_PRIOR_SCORE", "0.99") // a prior scored submission already exists
	t.Setenv("KAGGLE_SUBMIT_MAX_ATTEMPTS", "5")
	t.Setenv("KAGGLE_SUBMIT_DELAY", "0")

	stepDir, runDir := kaggletest.WireStep(t, "submit",
		`{"competition":{"slug":"titanic"},"submission":"make-submission"}`)
	kaggletest.WriteUpstream(t, runDir, "make-submission", submissionFile, "PassengerId,Survived\n892,0\n")

	if err := run(); err != nil {
		t.Fatalf("run: %v", err)
	}
	var sub kaggle.Submission
	kaggletest.ReadJSON(t, filepath.Join(stepDir, "submission.json"), &sub)
	if sub.File != submissionFile {
		t.Errorf("reported file = %q, want %q (ours, not the prior)", sub.File, submissionFile)
	}
	if sub.PublicScore == nil || *sub.PublicScore == 0.99 {
		t.Fatalf("reported %v — must be OUR score, never the pre-existing 0.99", sub.PublicScore)
	}
}

// TestRun_TimeoutFailsWithPendingRecord pins the timeout contract: retries
// exhausted still-unscored → non-zero exit, a pending submission.json (debug aid),
// and NO metrics.json (an unscored run emits no score metric).
func TestRun_TimeoutFailsWithPendingRecord(t *testing.T) {
	kaggletest.WireFake(t, t.TempDir())
	t.Setenv("KAGGLE_FAKE_SCORE_AFTER", "5")    // scored only after 5 polls...
	t.Setenv("KAGGLE_SUBMIT_MAX_ATTEMPTS", "2") // ...but we only poll twice
	t.Setenv("KAGGLE_SUBMIT_DELAY", "0")

	stepDir, runDir := kaggletest.WireStep(t, "submit",
		`{"competition":{"slug":"titanic"},"submission":"make-submission"}`)
	kaggletest.WriteUpstream(t, runDir, "make-submission", submissionFile, "PassengerId,Survived\n892,0\n")

	if err := run(); err == nil {
		t.Fatal("run on scoring timeout: want error, got nil")
	}
	var sub kaggle.Submission
	kaggletest.ReadJSON(t, filepath.Join(stepDir, "submission.json"), &sub)
	if sub.Scored() {
		t.Errorf("submission.json should be pending on timeout, got %+v", sub)
	}
	if sub.Status != kaggle.StatusPending {
		t.Errorf("status = %q, want pending", sub.Status)
	}
	if _, err := os.Stat(filepath.Join(stepDir, "metrics.json")); err == nil {
		t.Error("metrics.json must not exist for an unscored (failed) run")
	}
}

// TestRun_MissingUpstreamArtifact: submit errors (before any CLI call) when the
// upstream step's submission.csv is absent — a fixture is not an upstream artifact.
func TestRun_MissingUpstreamArtifact(t *testing.T) {
	kaggletest.WireStep(t, "submit",
		`{"competition":{"slug":"titanic"},"submission":"make-submission"}`)
	// deliberately no WriteUpstream — the artifact does not exist
	if err := run(); err == nil {
		t.Fatal("run with missing upstream submission.csv: want error, got nil")
	}
}
