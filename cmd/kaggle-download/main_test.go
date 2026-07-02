package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/xianxu/kaggle/internal/kaggletest"
)

// wireStep sets the METIS_* contract env for a step launched from stepDir/runDir
// and writes its with.json. Returns the step dir.
func wireStep(t *testing.T, stepID, withJSON string) (stepDir, runDir string) {
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

func TestRun_DownloadsAndUnzipsToLooseFiles(t *testing.T) {
	fake := kaggletest.BuildBin(t, "github.com/xianxu/kaggle/cmd/fake-kaggle", "")
	t.Setenv("KAGGLE_CLI", fake)
	t.Setenv("KAGGLE_FAKE", "1")
	t.Setenv("KAGGLE_FAKE_STATE", t.TempDir())

	stepDir, _ := wireStep(t, "download", `{"competition":{"slug":"titanic"}}`)

	if err := run(); err != nil {
		t.Fatalf("run: %v", err)
	}
	// The fake's zip carries train.csv + test.csv — assert they landed loose.
	for _, f := range []string{"train.csv", "test.csv"} {
		if _, err := os.Stat(filepath.Join(stepDir, f)); err != nil {
			t.Errorf("expected loose artifact %s: %v", f, err)
		}
	}
	// And the zip is cleaned up (not left as a stray artifact).
	if zips, _ := filepath.Glob(filepath.Join(stepDir, "*.zip")); len(zips) != 0 {
		t.Errorf("zip not removed after extraction: %v", zips)
	}
}

func TestRun_RejectsMissingSlug(t *testing.T) {
	// Validate() fails before any CLI call, so no fake needed.
	wireStep(t, "download", `{"competition":{}}`)
	if err := run(); err == nil {
		t.Fatal("run with empty competition slug: want error, got nil")
	}
}
