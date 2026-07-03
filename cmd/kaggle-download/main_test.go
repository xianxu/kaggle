package main

import (
	"archive/zip"
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

// TestUnzip_RejectsZipSlip pins the zip-slip guard: an entry escaping destDir via
// ../ is refused, not written outside the step dir.
func TestUnzip_RejectsZipSlip(t *testing.T) {
	dir := t.TempDir()
	zpath := filepath.Join(dir, "evil.zip")
	f, err := os.Create(zpath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	w, err := zw.Create("../escape.txt") // malicious entry
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("pwned")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	f.Close()

	dest := filepath.Join(dir, "out")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := unzip(zpath, dest); err == nil {
		t.Fatal("unzip must reject a ../ escape entry")
	}
	if _, err := os.Stat(filepath.Join(dir, "escape.txt")); err == nil {
		t.Error("zip-slip wrote outside destDir")
	}
}
