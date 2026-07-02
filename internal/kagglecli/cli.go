// Package kagglecli is the thin IO seam wrapping the official `kaggle` CLI. It
// shells an INJECTABLE command (${KAGGLE_CLI:-kaggle}) so a process-level fake can
// replace it in tests. It does NO parsing — CLI text becomes typed state in
// pkg/kaggle (ParseSubmissions). ARCH-PURE: this package is the boundary; the
// auth DECISION is the pure kaggle.CredentialSource, fed by the IO gathered here.
package kagglecli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/xianxu/kaggle/pkg/kaggle"
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

// checkCredentials gathers the auth IO (env pair + ~/.kaggle/kaggle.json existence)
// and defers the DECISION to the pure kaggle.CredentialSource. Skipped entirely on
// an explicit KAGGLE_FAKE=1 signal (never inferred from the binary name, which
// would wrongly skip a real CLI installed at a full path like .venv/bin/kaggle).
// Existence-only for the file — the secret value is never read.
func checkCredentials() error {
	if os.Getenv("KAGGLE_FAKE") == "1" {
		return nil
	}
	fileExists := false
	if home, err := os.UserHomeDir(); err == nil {
		if _, err := os.Stat(filepath.Join(home, ".kaggle", "kaggle.json")); err == nil {
			fileExists = true
		}
	}
	_, err := kaggle.CredentialSource(os.Getenv("KAGGLE_USERNAME"), os.Getenv("KAGGLE_KEY"), fileExists)
	return err
}

// Download pulls the competition's data into dest (a .zip, from the real CLI).
func (c CLI) Download(slug, dest string) error {
	if err := checkCredentials(); err != nil {
		return err
	}
	return c.run("competitions", "download", "-c", slug, "-p", dest)
}

// Submit uploads file as a submission to the competition, with a message.
func (c CLI) Submit(slug, file, msg string) error {
	if err := checkCredentials(); err != nil {
		return err
	}
	return c.run("competitions", "submit", "-c", slug, "-f", file, "-m", msg)
}

// Submissions returns the raw `--csv` stdout listing the competition's submissions.
// Parse it with pkg/kaggle.ParseSubmissions (no parsing happens in this layer).
func (c CLI) Submissions(slug string) (string, error) {
	if err := checkCredentials(); err != nil {
		return "", err
	}
	out, err := exec.Command(c.bin, "competitions", "submissions", "-c", slug, "--csv").Output()
	if err != nil {
		return "", wrap(c.bin, err)
	}
	return string(out), nil
}

func (c CLI) run(args ...string) error {
	if _, err := exec.Command(c.bin, args...).Output(); err != nil {
		return wrap(c.bin, err)
	}
	return nil
}

func wrap(bin string, err error) error {
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return fmt.Errorf("kaggle: %s failed: %w\n%s", bin, err, ee.Stderr)
	}
	return fmt.Errorf("kaggle: %s: %w", bin, err)
}
