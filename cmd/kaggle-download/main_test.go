package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/xianxu/kaggle/internal/kaggletest"
)

func TestRun_DownloadsAndUnzipsToLooseFiles(t *testing.T) {
	kaggletest.WireFake(t, t.TempDir())
	stepDir, _ := kaggletest.WireStep(t, "download", `{"competition":{"slug":"titanic"}}`)

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
	kaggletest.WireStep(t, "download", `{"competition":{}}`)
	if err := run(); err == nil {
		t.Fatal("run with empty competition slug: want error, got nil")
	}
}
