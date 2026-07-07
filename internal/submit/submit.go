// Package submit is the shared submit→poll→score path for both the kaggle/submit
// STEP (cmd/kaggle-submit) and the ad-hoc `kaggle submit` CLI (cmd/kaggle). It
// owns the one correct blocking-poll (newest-submission-correlated) so the two
// callers can never drift (ARCH-DRY, kaggle#5). It touches no IO of its own beyond
// the injected Submitter + clock (ARCH-PURE): each caller decides what to do with
// the returned Submission (the step writes submission.json+metrics; the CLI prints
// the score).
package submit

import (
	"path/filepath"
	"time"

	"github.com/xianxu/kaggle/pkg/kaggle"
)

// Submitter is the kagglecli seam SubmitAndPoll needs — kagglecli.CLI satisfies it
// structurally, so the poll logic never depends on os/exec.
type Submitter interface {
	Submit(slug, file, msg string) error
	Submissions(slug string) (string, error)
}

// SubmitAndPoll uploads csvPath to slug, then blocking-polls for THIS upload's
// public score. It is the single submit+poll path shared by the step and the CLI.
// Returns the submission (scored on success; the newest own/pending row on
// timeout) and whether it scored. maxAttempts/delay come from the caller (the
// KAGGLE_SUBMIT_* envs via EnvInt/EnvDuration).
func SubmitAndPoll(cli Submitter, slug, csvPath, message string, maxAttempts int, delay time.Duration) (kaggle.Submission, bool, error) {
	if err := cli.Submit(slug, csvPath, message); err != nil {
		return kaggle.Submission{}, false, err
	}
	return pollScore(
		func() (string, error) { return cli.Submissions(slug) },
		filepath.Base(csvPath), // correlate the score to the file WE just uploaded
		maxAttempts,
		func(int) { time.Sleep(delay) }, // injected clock (ARCH-PURE): tests pass a no-op
	)
}

// pollScore polls submissionsFn up to maxAttempts times, returning the score of
// the submission THIS run just uploaded — not "any scored submission". Kaggle
// lists submissions newest-first, so our upload is subs[0]; keying off the newest
// row (and, when wantFile is set, requiring its File to match what we uploaded) is
// what correlates the reported score to OUR file. Using kaggle.LatestScored here
// would be a bug: a competition with prior scored submissions would report an
// OLDER submission's score for our still-pending upload. If the newest row isn't
// ours yet (eventual consistency after submit), we keep polling; on exhaustion we
// never report someone else's score — we time out (scored=false, safe).
//
// sleep is INJECTED (ARCH-PURE / controllable-time) and called only BETWEEN
// attempts, so tests drive the loop with zero wall-clock. On exhaustion it returns
// the newest observed OWN submission (unscored) with scored=false so the caller can
// persist a pending record; a synthetic pending marker if ours never appeared.
func pollScore(submissionsFn func() (string, error), wantFile string, maxAttempts int, sleep func(attempt int)) (kaggle.Submission, bool, error) {
	last := kaggle.Submission{Status: kaggle.StatusPending}
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		out, err := submissionsFn()
		if err != nil {
			return kaggle.Submission{}, false, err
		}
		subs, err := kaggle.ParseSubmissions(out)
		if err != nil {
			return kaggle.Submission{}, false, err
		}
		// subs[0] is the newest = the one we just uploaded (unless it hasn't
		// registered yet, or a concurrent submit raced in — in both cases keep
		// polling rather than report a wrong score).
		if len(subs) > 0 && (wantFile == "" || subs[0].File == wantFile) {
			newest := subs[0]
			last = newest // our newest row, for the debug record on timeout/error
			if newest.Scored() {
				return newest, true, nil
			}
			if newest.Status == kaggle.StatusError {
				// Terminal: Kaggle rejected our submission — it will NEVER score, so
				// fast-fail instead of burning the whole budget.
				return newest, false, nil
			}
		}
		if attempt < maxAttempts {
			sleep(attempt)
		}
	}
	return last, false, nil
}
