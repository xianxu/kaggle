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

// pollScore polls submissionsFn up to maxAttempts times, returning the newest
// scored Submission (scored=true) as soon as one appears. sleep is INJECTED
// (ARCH-PURE / controllable-time) and called only BETWEEN attempts, so tests drive
// the loop with zero wall-clock. On exhaustion it returns the newest observed
// (unscored) submission with scored=false so the caller can persist a pending
// record; a synthetic pending marker if no rows were ever returned.
func pollScore(submissionsFn func() (string, error), maxAttempts int, sleep func(attempt int)) (kaggle.Submission, bool, error) {
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
		if s, ok := kaggle.LatestScored(subs); ok {
			return s, true, nil
		}
		if len(subs) > 0 {
			last = subs[0] // newest row, for the debug record on timeout/error
			// Terminal error: Kaggle rejected the submission (e.g. bad format) — it
			// will NEVER score, so fast-fail instead of burning the whole poll
			// budget. The caller distinguishes this from a timeout by Status.
			if last.Status == kaggle.StatusError {
				return last, false, nil
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
