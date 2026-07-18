package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xianxu/kaggle/internal/kaggletest"
)

// writeFile writes content to path, creating parent dirs (a submission.csv lives
// at runs/<id>/submission/submission.csv).
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// scoreEnv wires the fake for a fast, deterministic scored submission.
func scoreEnv(t *testing.T) {
	t.Helper()
	kaggletest.WireFake(t, t.TempDir()) // MUST precede t.Chdir — it go-builds the fake in this module
	t.Setenv("KAGGLE_FAKE_SCORE_AFTER", "1")
	t.Setenv("KAGGLE_SUBMIT_MAX_ATTEMPTS", "5")
	t.Setenv("KAGGLE_SUBMIT_DELAY", "0")
}

// TestSubmit_RunResolvesAndScores is the Done-when: `kaggle submit --run winner`
// resolves runs/winner/submission/submission.csv, reads the slug from record.json,
// submits, and prints the real public_score — no pipeline edit, no -c.
func TestSubmit_RunResolvesAndScores(t *testing.T) {
	scoreEnv(t)
	ws := t.TempDir()
	writeFile(t, filepath.Join(ws, "runs", "winner", "submission", "submission.csv"), "PassengerId,Survived\n892,0\n")
	writeFile(t, filepath.Join(ws, "runs", "winner", "record.json"),
		`{"steps":[{"step_id":"download","with":{"competition":{"slug":"titanic"}}}]}`)
	t.Chdir(ws) // Go 1.26: auto-restored; runs/<id>/... resolves cwd-relative

	var out bytes.Buffer
	if err := run([]string{"submit", "--run", "winner"}, &out); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(out.String(), "public_score: 0.775") {
		t.Fatalf("want public_score 0.775 in output; got %q", out.String())
	}
}

// TestSubmit_CFlagOverridesRecord: -c provides the slug with no record.json.
func TestSubmit_CFlagOverridesRecord(t *testing.T) {
	scoreEnv(t)
	ws := t.TempDir()
	writeFile(t, filepath.Join(ws, "runs", "w", "submission", "submission.csv"), "PassengerId,Survived\n892,0\n")
	t.Chdir(ws)

	var out bytes.Buffer
	if err := run([]string{"submit", "--run", "w", "-c", "titanic"}, &out); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(out.String(), "public_score: 0.775") {
		t.Fatalf("want public_score; got %q", out.String())
	}
}

// TestSubmit_FFlagExplicitPath: -f submits an explicit file (no runs/ layout).
func TestSubmit_FFlagExplicitPath(t *testing.T) {
	scoreEnv(t)
	ws := t.TempDir()
	csv := filepath.Join(ws, "my-submission.csv")
	writeFile(t, csv, "PassengerId,Survived\n892,0\n")

	var out bytes.Buffer
	if err := run([]string{"submit", "-f", csv, "-c", "titanic"}, &out); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(out.String(), "public_score: 0.775") {
		t.Fatalf("want public_score; got %q", out.String())
	}
}

// TestSubmit_SlugMissingErrors: --run with no record slug and no -c → actionable error.
func TestSubmit_SlugMissingErrors(t *testing.T) {
	scoreEnv(t)
	ws := t.TempDir()
	writeFile(t, filepath.Join(ws, "runs", "w", "submission", "submission.csv"), "x\n")
	// no record.json → no slug
	t.Chdir(ws)

	err := run([]string{"submit", "--run", "w"}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "-c") {
		t.Fatalf("want a slug-missing error naming -c; got %v", err)
	}
}

// TestSubmit_NeedsRunOrFile: neither --run nor -f → usage error.
func TestSubmit_NeedsRunOrFile(t *testing.T) {
	if err := run([]string{"submit"}, &bytes.Buffer{}); err == nil {
		t.Fatal("want an error when neither --run nor -f is given")
	}
}

// TestRun_HelpAndUnknown: --help prints usage (no error); an unknown verb errors.
func TestRun_HelpAndUnknown(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"--help"}, &out); err != nil {
		t.Errorf("--help: unexpected error %v", err)
	}
	if !strings.Contains(out.String(), "kaggle submit") {
		t.Errorf("--help should print usage; got %q", out.String())
	}
	if err := run([]string{"bogus"}, &bytes.Buffer{}); err == nil {
		t.Error("unknown subcommand should error")
	}
	// `kaggle submit --help` → clean usage on stdout, exit 0 (not flag.ErrHelp/exit 1).
	var subHelp bytes.Buffer
	if err := run([]string{"submit", "--help"}, &subHelp); err != nil {
		t.Errorf("submit --help: unexpected error %v", err)
	}
	if !strings.Contains(subHelp.String(), "kaggle submit") {
		t.Errorf("submit --help should print usage; got %q", subHelp.String())
	}
}

// TestSubmit_DashCDirAnchorsRunsFromForeignCwd (metis#34): `-C <pipeline dir>` anchors
// runs/<id> so submit works from ANY cwd — the one surface the #34 audit found genuinely
// cwd-dependent. Same fixture as RunResolvesAndScores, but cwd is an unrelated dir.
func TestSubmit_DashCDirAnchorsRunsFromForeignCwd(t *testing.T) {
	scoreEnv(t)
	ws := t.TempDir()
	writeFile(t, filepath.Join(ws, "runs", "winner", "submission", "submission.csv"), "PassengerId,Survived\n892,0\n")
	writeFile(t, filepath.Join(ws, "runs", "winner", "record.json"),
		`{"steps":[{"step_id":"download","with":{"competition":{"slug":"titanic"}}}]}`)
	t.Chdir(t.TempDir()) // foreign cwd — without -C this exact invocation fails

	var out bytes.Buffer
	if err := run([]string{"submit", "-C", ws, "--run", "winner"}, &out); err != nil {
		t.Fatalf("run with -C: %v", err)
	}
	if !strings.Contains(out.String(), "public_score: 0.775") {
		t.Fatalf("want scored submit via -C anchor; got %q", out.String())
	}
	// and the failure mode names the anchor: no -C from the foreign cwd
	if err := run([]string{"submit", "--run", "winner"}, &out); err == nil || !strings.Contains(err.Error(), "-C") {
		t.Errorf("foreign-cwd submit without -C must fail mentioning -C; got %v", err)
	}
}
