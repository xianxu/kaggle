// Command kaggle-submit is the `kaggle/submit` step-type: it uploads an upstream
// step's submission.csv to a competition, then POLLS the CLI until Kaggle assigns
// a public score (scoring is async), and writes a typed submission.json + a
// metrics.json{public_score}. Its purpose is to return a score, so an unscored run
// (poll retries exhausted) is a FAILED run: it still writes a pending
// submission.json for debugging, then exits non-zero. See workshop/plans/000001-*
// Chunk 2 Task 3.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/xianxu/kaggle/internal/kagglecli"
	"github.com/xianxu/kaggle/internal/stepio"
	"github.com/xianxu/kaggle/pkg/kaggle"
)

// submissionFile is the conventional filename the upstream step writes and this
// step reads (the metis upstream-artifact convention: with names the upstream step
// id, the filename is fixed by the step-type pair — cf. metis's folds.json).
const submissionFile = "submission.csv"

// submitWith is this step's `with` config, read from with.json.
type submitWith struct {
	Competition kaggle.Competition `json:"competition"`
	Submission  string             `json:"submission"` // upstream step id producing submission.csv
	Message     string             `json:"message,omitempty"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "kaggle/submit:", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, err := stepio.New()
	if err != nil {
		return err
	}
	var w submitWith
	if err := ctx.ReadWith(&w); err != nil {
		return err
	}
	if err := w.Competition.Validate(); err != nil {
		return err
	}
	if w.Submission == "" {
		return fmt.Errorf("with.submission (the upstream step id producing %s) is required", submissionFile)
	}
	csvPath := ctx.UpstreamPath(w.Submission, submissionFile)
	if _, err := os.Stat(csvPath); err != nil {
		return fmt.Errorf("upstream artifact %s/%s: %w", w.Submission, submissionFile, err)
	}

	cli := kagglecli.New()
	if err := cli.Submit(w.Competition.Slug, csvPath, w.Message); err != nil {
		return err
	}

	maxAttempts := envInt("KAGGLE_SUBMIT_MAX_ATTEMPTS", 30)
	delay := envDuration("KAGGLE_SUBMIT_DELAY", 5*time.Second)
	sub, scored, err := pollScore(
		func() (string, error) { return cli.Submissions(w.Competition.Slug) },
		filepath.Base(csvPath), // correlate the score to the file WE just uploaded
		maxAttempts,
		func(int) { time.Sleep(delay) }, // injected clock (ARCH-PURE): tests pass a no-op
	)
	if err != nil {
		return err
	}
	sub.Competition = w.Competition.Slug

	// Always persist submission.json (scored on success, pending on timeout) — a
	// pending record is a debugging aid for a failed run.
	if err := writeSubmission(ctx, sub); err != nil {
		return err
	}
	if !scored {
		if sub.Status == kaggle.StatusError {
			return fmt.Errorf("%q submission rejected by kaggle (status=error) — see submission.json", w.Competition.Slug)
		}
		return fmt.Errorf("%q not scored after %d attempts (submission.json status=%s)", w.Competition.Slug, maxAttempts, sub.Status)
	}
	return ctx.WriteMetrics(map[string]float64{"public_score": *sub.PublicScore})
}

// pollScore polls submissionsFn up to maxAttempts times, returning the score of
// the submission THIS step just uploaded — not "any scored submission". Kaggle
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

func writeSubmission(ctx stepio.Context, sub kaggle.Submission) error {
	b, err := json.MarshalIndent(sub, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(ctx.OutPath("submission.json"), append(b, '\n'), 0o644)
}

func envInt(name string, def int) int {
	v := os.Getenv(name)
	if v == "" {
		return def
	}
	if n, err := strconv.Atoi(v); err == nil {
		return n
	}
	fmt.Fprintf(os.Stderr, "kaggle/submit: ignoring malformed %s=%q, using %d\n", name, v, def)
	return def
}

// envDuration accepts a Go duration ("5s", "0") or a bare integer read as seconds.
// A malformed value warns (rather than silently defaulting — a hidden misconfig).
func envDuration(name string, def time.Duration) time.Duration {
	v := os.Getenv(name)
	if v == "" {
		return def
	}
	if d, err := time.ParseDuration(v); err == nil {
		return d
	}
	if n, err := strconv.Atoi(v); err == nil {
		return time.Duration(n) * time.Second
	}
	fmt.Fprintf(os.Stderr, "kaggle/submit: ignoring malformed %s=%q, using %s\n", name, v, def)
	return def
}
