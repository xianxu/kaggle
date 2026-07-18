package main

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
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

// --- metis#25: declared content pins (fixed-output-derivation identity) ---

// pinWith builds a download with.json carrying sha256 pins.
func pinWith(pins map[string]string) string {
	m := map[string]any{"competition": map[string]any{"slug": "titanic"}, "sha256": pins}
	b, _ := json.Marshal(m)
	return string(b)
}

// fakeData points the process-level fake at a temp data dir served byte-for-byte,
// returning the dir (3-file realism: the real download carries gender_submission.csv).
func fakeData(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("KAGGLE_FAKE_DATA_DIR", dir)
	return dir
}

func TestRun_PinnedContentVerifies(t *testing.T) {
	kaggletest.WireFake(t, t.TempDir())
	fakeData(t, map[string]string{"train.csv": "T", "test.csv": "E", "gender_submission.csv": "G"})
	kaggletest.WireStep(t, "download", pinWith(map[string]string{
		"train.csv": shaHex("T"), "test.csv": shaHex("E"), "gender_submission.csv": shaHex("G"),
	}))
	if err := run(); err != nil {
		t.Fatalf("pinned+matching download must succeed: %v", err)
	}
}

func TestRun_MutatedPayloadUnderPinFailsLoudly(t *testing.T) {
	kaggletest.WireFake(t, t.TempDir())
	fakeData(t, map[string]string{"train.csv": "T-MUTATED", "test.csv": "E", "gender_submission.csv": "G"})
	kaggletest.WireStep(t, "download", pinWith(map[string]string{
		"train.csv": shaHex("T"), "test.csv": shaHex("E"), "gender_submission.csv": shaHex("G"),
	}))
	err := run()
	if err == nil {
		t.Fatal("mutated payload under a pin must FAIL the step")
	}
	if !strings.Contains(err.Error(), "train.csv") {
		t.Errorf("failure must name the mutated file; got: %v", err)
	}
}

func TestRun_UnpinnedPrintsPasteReadyBlock(t *testing.T) {
	kaggletest.WireFake(t, t.TempDir())
	fakeData(t, map[string]string{"train.csv": "T", "test.csv": "E"})
	kaggletest.WireStep(t, "download", `{"competition":{"slug":"titanic"}}`)

	// capture stderr around run()
	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	err := run()
	w.Close()
	os.Stderr = old
	out, _ := io.ReadAll(r)

	if err != nil {
		t.Fatalf("unpinned download must still succeed: %v", err)
	}
	for _, want := range []string{"UNPINNED ingest", "sha256:", "train.csv: " + shaHex("T")} {
		if !strings.Contains(string(out), want) {
			t.Errorf("stderr missing %q; got:\n%s", want, out)
		}
	}
}

func shaHex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}
