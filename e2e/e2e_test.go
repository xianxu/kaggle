// Package e2e is the hermetic end-to-end proof for kaggle#1 M2: it drives the REAL
// built `metis` binary through the download → make-submission → submit thread
// against the process-level fake kaggle CLI, and asserts the issue's Done-when —
// a scored Submission + public_score, with data downloaded as loose files. Driving
// the actual metis binary (not an in-process stub) is what makes stepio's
// drift-guard genuine: metis emits the real METIS_* names; a rename there would
// fail the run RED.
package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/xianxu/kaggle/internal/kaggletest"
	"github.com/xianxu/kaggle/pkg/kaggle"
)

// kaggleRoot returns the kaggle repo root (this file is at <root>/e2e/e2e_test.go).
func kaggleRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Dir(filepath.Dir(file))
}

func TestKaggleThread_EndToEnd(t *testing.T) {
	root := kaggleRoot(t)
	metisDir := filepath.Join(root, "..", "metis")
	if _, err := os.Stat(filepath.Join(metisDir, "go.mod")); err != nil {
		t.Skipf("sibling metis not found at %s — skipping M2's primary Done-when e2e", metisDir)
	}

	// Build the real metis binary from ITS OWN module (so metis's ../ariadne
	// replace resolves) + the fake kaggle CLI.
	metisBin := kaggletest.BuildBin(t, "./cmd/metis", metisDir)
	fake := kaggletest.BuildBin(t, "github.com/xianxu/kaggle/cmd/fake-kaggle", "")

	// A temp experiment dir (runs land at <expDir>/runs/<id>/).
	expDir := t.TempDir()
	expPath := filepath.Join(expDir, "kaggle-thread.md")
	copyFile(t, filepath.Join(root, "testdata", "experiment", "kaggle-thread.md"), expPath)

	// Step path: kaggle's own steps/ + the test's make-submission stub (Decision B —
	// metis exposes no --step-path flag; METIS_STEP_PATH is the seam).
	stepPath := filepath.Join(root, "steps") + string(os.PathListSeparator) + filepath.Join(root, "testdata", "steps")

	cmd := exec.Command(metisBin, "run", "--run", "run-e2e", expPath)
	cmd.Env = append(os.Environ(),
		"METIS_STEP_PATH="+stepPath,
		"KAGGLE_CLI="+fake,
		"KAGGLE_FAKE_STATE="+t.TempDir(),
		"KAGGLE_FAKE_SCORE_AFTER=1", // forces submit's poll to iterate (pending → scored)
		"KAGGLE_SUBMIT_MAX_ATTEMPTS=5",
		"KAGGLE_SUBMIT_DELAY=0",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("metis run failed: %v\n%s", err, out)
	}
	t.Logf("metis run:\n%s", out)

	// run.json is the record of truth.
	var run struct {
		Status    string             `json:"status"`
		Metrics   map[string]float64 `json:"metrics"`
		Artifacts []string           `json:"artifacts"`
	}
	kaggletest.ReadJSON(t, filepath.Join(expDir, "runs", "run-e2e", "run.json"), &run)
	if run.Status != "ok" {
		t.Fatalf("run.status = %q, want ok\n%s", run.Status, out)
	}
	if run.Metrics["public_score"] <= 0 {
		t.Errorf("run.metrics.public_score = %v, want > 0", run.Metrics["public_score"])
	}

	// submit emitted a scored, typed Submission.
	var sub kaggle.Submission
	kaggletest.ReadJSON(t, filepath.Join(expDir, "runs", "run-e2e", "submit", "submission.json"), &sub)
	if !sub.Scored() {
		t.Errorf("submit/submission.json not scored: %+v", sub)
	}
	if sub.Competition != "titanic" {
		t.Errorf("submission.competition = %q, want titanic", sub.Competition)
	}

	// download landed LOOSE data (not a stray zip).
	if _, err := os.Stat(filepath.Join(expDir, "runs", "run-e2e", "download", "train.csv")); err != nil {
		t.Errorf("download/train.csv (loose) missing: %v", err)
	}
	if zips, _ := filepath.Glob(filepath.Join(expDir, "runs", "run-e2e", "download", "*.zip")); len(zips) != 0 {
		t.Errorf("download left a zip artifact: %v", zips)
	}
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	b, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, b, 0o644); err != nil {
		t.Fatal(err)
	}
}
