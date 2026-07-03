// Package kaggletest holds shared test helpers so the process-level fake (and, in
// the e2e, the metis binary) is built one consistent way across the step-type and
// integration tests, and the METIS_*/KAGGLE_* env a step sees is wired one way
// (ARCH-DRY — one set of helpers, not a copy per test package).
package kaggletest

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// BuildBin compiles a Go main package (by import path or filesystem path) into a
// temp binary and returns its path. dir, when non-empty, is the working directory
// for `go build` — needed to build a package in a SIBLING module (e.g. metis) via
// `-C dir` so that module's own replace directives resolve.
func BuildBin(t *testing.T, target string, dir string) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), filepath.Base(target))
	args := []string{"build", "-o", bin}
	if dir != "" {
		args = append([]string{"-C", dir}, args...)
	}
	args = append(args, target)
	cmd := exec.Command("go", args...)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("build %s: %v", target, err)
	}
	return bin
}

// WireStep sets the METIS_* contract env for a step run in-process (playing the
// runner's role — the literal names mirror what real metis emits) and writes the
// step's with.json into a fresh step dir. Returns (stepDir, runDir).
func WireStep(t *testing.T, stepID, withJSON string) (stepDir, runDir string) {
	t.Helper()
	stepDir, runDir = t.TempDir(), t.TempDir()
	t.Setenv("METIS_STEP_DIR", stepDir)
	t.Setenv("METIS_RUN_DIR", runDir)
	t.Setenv("METIS_STEP_ID", stepID)
	if err := os.WriteFile(filepath.Join(stepDir, "with.json"), []byte(withJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	return stepDir, runDir
}

// WireFake builds the process-level fake, points KAGGLE_CLI at it, and sets the
// no-auth + state env. Returns the fake's path.
func WireFake(t *testing.T, stateDir string) string {
	t.Helper()
	fake := BuildBin(t, "github.com/xianxu/kaggle/cmd/fake-kaggle", "")
	t.Setenv("KAGGLE_CLI", fake)
	t.Setenv("KAGGLE_FAKE", "1")
	t.Setenv("KAGGLE_FAKE_STATE", stateDir)
	return fake
}

// WriteUpstream creates <runDir>/<stepID>/<file> with content — an upstream step's
// artifact, as the metis runner would leave it for a downstream step to read.
func WriteUpstream(t *testing.T, runDir, stepID, file, content string) {
	t.Helper()
	dir := filepath.Join(runDir, stepID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, file), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// ReadJSON reads and unmarshals a JSON file, failing the test on any error.
func ReadJSON(t *testing.T, path string, v any) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
}
