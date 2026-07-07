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
	"time"

	"github.com/xianxu/kaggle/internal/kagglecli"
	"github.com/xianxu/kaggle/internal/stepio"
	"github.com/xianxu/kaggle/internal/submit"
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

	// Submit + poll via the shared helper (one path with the `kaggle submit` CLI —
	// kaggle#5). The step keeps the step-specific tail (persist submission.json +
	// metrics); the submit+poll+auth core lives in internal/submit.
	maxAttempts := submit.EnvInt("KAGGLE_SUBMIT_MAX_ATTEMPTS", 30)
	delay := submit.EnvDuration("KAGGLE_SUBMIT_DELAY", 5*time.Second)
	sub, scored, err := submit.SubmitAndPoll(kagglecli.New(), w.Competition.Slug, csvPath, w.Message, maxAttempts, delay)
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

func writeSubmission(ctx stepio.Context, sub kaggle.Submission) error {
	b, err := json.MarshalIndent(sub, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(ctx.OutPath("submission.json"), append(b, '\n'), 0o644)
}
