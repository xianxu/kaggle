package submit

import (
	"encoding/json"
	"testing"

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

// fakeSubmitter is a Submitter test double: records submitted slugs, scripts the
// Submissions output. No subprocess.
type fakeSubmitter struct {
	submitted []string
	subsOut   func() (string, error)
}

func (f *fakeSubmitter) Submit(slug, file, msg string) error {
	f.submitted = append(f.submitted, slug)
	return nil
}
func (f *fakeSubmitter) Submissions(slug string) (string, error) { return f.subsOut() }

// TestSubmitAndPoll_SubmitsThenScores proves the shared helper submits once then
// polls to OUR upload's score — the one path both the step and the CLI use.
func TestSubmitAndPoll_SubmitsThenScores(t *testing.T) {
	calls := 0
	f := &fakeSubmitter{subsOut: func() (string, error) {
		calls++
		s := kaggle.Submission{File: "submission.csv", Status: kaggle.StatusPending}
		if calls >= 2 {
			score := 0.775
			s.Status = kaggle.StatusComplete
			s.PublicScore = &score
		}
		return kaggle.FormatSubmissionsCSV([]kaggle.Submission{s}), nil
	}}
	sub, scored, err := SubmitAndPoll(f, "titanic", "/x/submission.csv", "msg", 5, 0)
	if err != nil || !scored {
		t.Fatalf("scored=%v err=%v", scored, err)
	}
	if len(f.submitted) != 1 || f.submitted[0] != "titanic" {
		t.Errorf("submit calls = %v, want [titanic]", f.submitted)
	}
	if sub.PublicScore == nil || *sub.PublicScore != 0.775 {
		t.Fatalf("score = %v, want 0.775", sub.PublicScore)
	}
}

// kaggle#8 divergence 1, the LIVE-PATH regression: real Kaggle reports statuses as
// Python enum reprs (`SubmissionStatus.ERROR`), not bare words. Before normalization
// landed in ParseSubmissions, `newest.Status == kaggle.StatusError` here was
// always-false on live, so a REJECTED submission never hit its terminal fast-fail —
// it burned the entire poll budget and then reported a misleading timeout. This
// asserts the behavior (fast-fail after ONE poll), not the literal.
//
// CAVEAT, deliberate: the `SubmissionStatus.ERROR` spelling is SHAPE-INFERRED from
// the observed `SubmissionStatus.` prefix — no errored submission has ever been
// captured. This test proves "a wire-shaped error status fast-fails"; it does NOT
// validate that Kaggle spells it ERROR.
func TestPollScore_WireErrorFastFails(t *testing.T) {
	calls := 0
	subFn := func() (string, error) {
		calls++
		return "ref,fileName,status,publicScore\n" +
			"77,submission.csv,SubmissionStatus.ERROR,\n", nil
	}
	slept := 0
	sub, scored, err := pollScore(subFn, "submission.csv", 5, func(int) { slept++ })
	if err != nil {
		t.Fatal(err)
	}
	if scored {
		t.Error("an errored submission must not report scored")
	}
	if calls != 1 {
		t.Errorf("want fast-fail after 1 poll, got %d — the terminal check is dead on live", calls)
	}
	if slept != 0 {
		t.Errorf("fast-fail must not burn the poll budget, slept %d times", slept)
	}
	if sub.Status != kaggle.StatusError {
		t.Errorf("status = %q, want normalized %q", sub.Status, kaggle.StatusError)
	}
	if sub.StatusRaw != "SubmissionStatus.ERROR" {
		t.Errorf("wire spelling should be preserved for diagnostics, got %q", sub.StatusRaw)
	}
}

// CROSS-REPO CONTRACT (kaggle#8): submission.json's `status` is consumed outside this
// repo — kbench/e2e/thread_test.py asserts `submit["status"] == "complete"`. Storing
// the raw wire value would silently break a peer's e2e; normalizing at the parse
// boundary keeps the artifact vocabulary stable. StatusRaw is additive.
func TestSubmissionJSONStatusIsNormalized(t *testing.T) {
	subs, err := kaggle.ParseSubmissions("ref,fileName,status,publicScore\n" +
		"77,submission.csv,SubmissionStatus.COMPLETE,0.5\n")
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(subs[0])
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got["status"] != "complete" {
		t.Errorf("submission.json status = %v, want \"complete\" (kbench e2e asserts this)", got["status"])
	}
	if got["status_raw"] != "SubmissionStatus.COMPLETE" {
		t.Errorf("status_raw = %v, want the wire spelling", got["status_raw"])
	}
}
