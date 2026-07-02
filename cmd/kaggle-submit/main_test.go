package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/xianxu/kaggle/internal/kaggletest"
	"github.com/xianxu/kaggle/pkg/kaggle"
)

// TestPollScore_IteratesThenScores exercises the PURE poll seam with an injected
// no-op clock and a scripted submissionsFn — no subprocess, no wall-clock. It
// proves the loop iterates through the async pending→scored transition (the whole
// reason submit polls) and sleeps only BETWEEN attempts.
func TestPollScore_IteratesThenScores(t *testing.T) {
	calls := 0
	subFn := func() (string, error) {
		calls++
		s := kaggle.Submission{File: "submission.csv", Status: kaggle.StatusPending}
		if calls >= 2 { // scored no earlier than the second poll
			score := 0.8
			s.Status = kaggle.StatusComplete
			s.PublicScore = &score
		}
		return kaggle.FormatSubmissionsCSV([]kaggle.Submission{s}), nil
	}
	slept := 0
	sub, scored, err := pollScore(subFn, "submission.csv", 5, func(int) { slept++ })
	if err != nil || !scored {
		t.Fatalf("scored=%v err=%v", scored, err)
	}
	if calls != 2 {
		t.Errorf("submissionsFn calls = %d, want 2 (iterated through pending)", calls)
	}
	if slept != 1 {
		t.Errorf("sleeps = %d, want 1 (only between the two attempts)", slept)
	}
	if !sub.Scored() || *sub.PublicScore != 0.8 {
		t.Errorf("expected scored 0.8, got %+v", sub)
	}
}

func TestPollScore_TimeoutReturnsPending(t *testing.T) {
	subFn := func() (string, error) {
		return kaggle.FormatSubmissionsCSV([]kaggle.Submission{{File: "s.csv", Status: kaggle.StatusPending}}), nil
	}
	sub, scored, err := pollScore(subFn, "s.csv", 3, func(int) {})
	if err != nil {
		t.Fatal(err)
	}
	if scored {
		t.Error("want scored=false on exhaustion")
	}
	if sub.Status != kaggle.StatusPending {
		t.Errorf("status = %q, want pending", sub.Status)
	}
}

// TestPollScore_ReportsOwnSubmissionNotPriorScored is the regression guard for the
// close-review Critical: with a PRIOR scored submission present, submit must wait
// for and report OUR (newest) submission's score, not the older already-scored one.
// The old LatestScored-based logic would return the prior score on poll #1.
func TestPollScore_ReportsOwnSubmissionNotPriorScored(t *testing.T) {
	prior, mine := 0.99, 0.55
	calls := 0
	subFn := func() (string, error) {
		calls++
		newest := kaggle.Submission{File: "submission.csv", Status: kaggle.StatusPending}
		if calls >= 2 { // OUR row scores only on poll #2
			newest.Status = kaggle.StatusComplete
			newest.PublicScore = &mine
		}
		older := kaggle.Submission{File: "prior.csv", Status: kaggle.StatusComplete, PublicScore: &prior}
		return kaggle.FormatSubmissionsCSV([]kaggle.Submission{newest, older}), nil // newest-first
	}
	sub, scored, err := pollScore(subFn, "submission.csv", 5, func(int) {})
	if err != nil || !scored {
		t.Fatalf("scored=%v err=%v", scored, err)
	}
	if sub.PublicScore == nil || *sub.PublicScore != mine {
		t.Fatalf("reported %v, want OUR score %v (not the prior scored 0.99)", sub.PublicScore, mine)
	}
	if calls != 2 {
		t.Errorf("polls = %d, want 2 (waited for OUR pending→scored, not the already-scored prior)", calls)
	}
}

// TestPollScore_TerminalErrorFastFails proves a rejected submission (status=error)
// fast-fails on the first poll instead of burning the whole retry budget — no
// sleep, exactly one poll.
func TestPollScore_TerminalErrorFastFails(t *testing.T) {
	calls := 0
	subFn := func() (string, error) {
		calls++
		return kaggle.FormatSubmissionsCSV([]kaggle.Submission{{File: "s.csv", Status: kaggle.StatusError}}), nil
	}
	sub, scored, err := pollScore(subFn, "s.csv", 30, func(int) { t.Fatal("must not sleep on a terminal error") })
	if err != nil {
		t.Fatal(err)
	}
	if scored {
		t.Error("status=error must not be scored")
	}
	if sub.Status != kaggle.StatusError {
		t.Errorf("status = %q, want error", sub.Status)
	}
	if calls != 1 {
		t.Errorf("polls = %d, want 1 (fast-fail, no budget burn)", calls)
	}
}

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
