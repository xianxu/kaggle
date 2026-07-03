// Package kagglecli is the thin IO seam wrapping the official `kaggle` CLI. It
// shells an INJECTABLE command (${KAGGLE_CLI:-kaggle}) so a process-level fake can
// replace it in tests. It does NO parsing — CLI text becomes typed state in
// pkg/kaggle (ParseSubmissions).
//
// Auth is DELEGATED entirely to the wrapped CLI. The CLI is the single source of
// truth for its own credentials — it accepts OAuth (`kaggle auth login`), the
// KAGGLE_API_TOKEN env var, ~/.kaggle/access_token (its modern default), or the
// legacy ~/.kaggle/kaggle.json — and it emits its own clear error when none is
// present, which surfaces here through wrap(). We deliberately do NOT pre-check
// credentials: any local mirror of the CLI's auth rules drifts as those rules
// evolve and false-negatives valid setups (e.g. access_token or an OAuth login,
// which leaves no file and no env var). ARCH-PURE: this package is the process
// boundary; ARCH-DRY: the CLI owns auth, we don't restate it.
package kagglecli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
)

// CLI wraps the official kaggle command; bin is ${KAGGLE_CLI:-kaggle}.
type CLI struct {
	bin string
}

// New returns a CLI bound to ${KAGGLE_CLI:-kaggle}.
func New() CLI {
	bin := os.Getenv("KAGGLE_CLI")
	if bin == "" {
		bin = "kaggle"
	}
	return CLI{bin: bin}
}

// Download pulls the competition's data into dest (a .zip, from the real CLI).
func (c CLI) Download(slug, dest string) error {
	return c.run("competitions", "download", "-c", slug, "-p", dest)
}

// Submit uploads file as a submission to the competition, with a message.
func (c CLI) Submit(slug, file, msg string) error {
	return c.run("competitions", "submit", "-c", slug, "-f", file, "-m", msg)
}

// Submissions returns the raw `--csv` stdout listing the competition's submissions.
// Parse it with pkg/kaggle.ParseSubmissions (no parsing happens in this layer).
func (c CLI) Submissions(slug string) (string, error) {
	out, err := c.output("competitions", "submissions", "-c", slug, "--csv")
	return string(out), err
}

// run execs the CLI discarding stdout; a non-zero exit surfaces as a wrapped error.
func (c CLI) run(args ...string) error {
	_, err := c.output(args...)
	return err
}

// output execs the CLI and returns its stdout; on failure it wraps the error,
// carrying the CLI's own stderr (e.g. its auth-failure message) out to the caller.
// The single exec+wrap path shared by run and Submissions.
func (c CLI) output(args ...string) ([]byte, error) {
	out, err := exec.Command(c.bin, args...).Output()
	if err != nil {
		return nil, wrap(c.bin, err)
	}
	return out, nil
}

func wrap(bin string, err error) error {
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return fmt.Errorf("kaggle: %s failed: %w\n%s", bin, err, ee.Stderr)
	}
	return fmt.Errorf("kaggle: %s: %w", bin, err)
}
