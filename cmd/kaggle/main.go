// Command kaggle is the thin user-facing Kaggle CLI (kaggle#5): ad-hoc verbs over
// the same kagglecli/submit path the kaggle/* steps use. First verb: submit — the
// ad-hoc "I ran an offline sweep, promoted a winner, now submit that ONE run's
// submission.csv and tell me the score" flow, with no pipeline edit.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/xianxu/kaggle/internal/kagglecli"
	"github.com/xianxu/kaggle/internal/submit"
)

const usage = "usage: kaggle submit [--run <id> | -f <file>] [-c <slug>] [-m <msg>]"

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "kaggle:", err)
		os.Exit(1)
	}
}

// run threads stdout (ARCH-PURE — tests assert on a bytes.Buffer, mirroring
// cmd/fake-kaggle's run(args, stdout) rather than swapping os.Stdout).
func run(args []string, stdout io.Writer) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		fmt.Fprintln(stdout, usage)
		if len(args) == 0 {
			return fmt.Errorf("no subcommand")
		}
		return nil
	}
	switch args[0] {
	case "submit":
		return cmdSubmit(args[1:], stdout)
	default:
		return fmt.Errorf("unknown subcommand %q (want: submit)", args[0])
	}
}

func cmdSubmit(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("submit", flag.ContinueOnError)
	fs.SetOutput(io.Discard) // we own the usage/error surface (main prefixes "kaggle:")
	runID := fs.String("run", "", "run id → runs/<id>/submission/submission.csv")
	file := fs.String("f", "", "explicit submission.csv path (overrides --run)")
	slug := fs.String("c", "", "competition slug (overrides the run record)")
	msg := fs.String("m", "", "submission message")
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp { // `kaggle submit -h/--help` → clean usage, exit 0
			fmt.Fprintln(stdout, usage)
			return nil
		}
		return err
	}

	csvPath := *file
	if csvPath == "" {
		if *runID == "" {
			return fmt.Errorf("need --run <id> or -f <file>")
		}
		csvPath = filepath.Join("runs", *runID, "submission", "submission.csv")
	}
	if _, err := os.Stat(csvPath); err != nil {
		return fmt.Errorf("submission csv %s: %w", csvPath, err)
	}

	comp := *slug
	if comp == "" && *runID != "" {
		// Best-effort: read the slug from the run's record.json (a kaggle step's
		// resolved `with`). -c overrides; absence is not fatal here.
		if b, err := os.ReadFile(filepath.Join("runs", *runID, "record.json")); err == nil {
			if s, ok := slugFromRecordJSON(b); ok {
				comp = s
			}
		}
	}
	if comp == "" {
		return fmt.Errorf("competition slug required: pass -c <slug> (or use --run <id> with a record.json that has one)")
	}

	maxAttempts := submit.EnvInt("KAGGLE_SUBMIT_MAX_ATTEMPTS", 30)
	delay := submit.EnvDuration("KAGGLE_SUBMIT_DELAY", 5*time.Second)
	sub, scored, err := submit.SubmitAndPoll(kagglecli.New(), comp, csvPath, *msg, maxAttempts, delay)
	if err != nil {
		return err
	}
	if !scored {
		// status=error is a terminal rejection (fast-failed on attempt 1); a
		// pending status means the poll budget (maxAttempts) was exhausted.
		return fmt.Errorf("%q did not score (status=%s; polled up to %d attempts)", comp, sub.Status, maxAttempts)
	}
	fmt.Fprintf(stdout, "public_score: %g\n", *sub.PublicScore)
	return nil
}
