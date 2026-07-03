package kagglecli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeStub writes an executable shell stub that appends its argv to $STUB_ARGS
// and prints stdout, returning its path. Lets the test assert the exact argv the
// CLI builds without a real kaggle binary.
func writeStub(t *testing.T, dir, stdout string) string {
	t.Helper()
	path := filepath.Join(dir, "fake-cli")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$STUB_ARGS\"\nprintf '%s' '" + stdout + "'\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCLIInvokesInjectedBinary(t *testing.T) {
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args.log")
	stub := writeStub(t, dir, "csvout")
	t.Setenv("KAGGLE_CLI", stub)
	t.Setenv("STUB_ARGS", argsFile)

	c := New()
	if err := c.Download("titanic", "/tmp/x"); err != nil {
		t.Fatalf("Download: %v", err)
	}
	if err := c.Submit("titanic", "sub.csv", "msg"); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	out, err := c.Submissions("titanic")
	if err != nil {
		t.Fatalf("Submissions: %v", err)
	}
	if out != "csvout" {
		t.Errorf("Submissions stdout = %q, want csvout", out)
	}
	logged, _ := os.ReadFile(argsFile)
	got := string(logged)
	for _, want := range []string{
		"competitions download -c titanic -p /tmp/x",
		"competitions submit -c titanic -f sub.csv -m msg",
		"competitions submissions -c titanic --csv",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("argv missing %q in:\n%s", want, got)
		}
	}
}

// TestCLIError_Propagates is the safety net that replaces the deleted credential
// precheck: we no longer validate creds ourselves, so a creds-less run must fail
// via the CLI's OWN error surfacing through the wrapper. The stub mimics the real
// CLI refusing on missing credentials (stderr + non-zero exit); wrap() must carry
// that stderr out so the operator sees the actionable message.
func TestCLIError_Propagates(t *testing.T) {
	dir := t.TempDir()
	stub := filepath.Join(dir, "failing-cli")
	script := "#!/bin/sh\necho 'no Kaggle API credentials found; run: kaggle auth login' >&2\nexit 1\n"
	if err := os.WriteFile(stub, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KAGGLE_CLI", stub)

	c := New()
	// Both paths (run → output, and Submissions → output) must surface the
	// CLI's stderr — cover each so neither error branch silently swallows it.
	if err := c.Download("titanic", t.TempDir()); err == nil {
		t.Error("Download against a failing CLI: want error, got nil")
	} else if !strings.Contains(err.Error(), "no Kaggle API credentials found") {
		t.Errorf("Download: wrapped error must carry the CLI's stderr; got: %v", err)
	}
	if _, err := c.Submissions("titanic"); err == nil {
		t.Error("Submissions against a failing CLI: want error, got nil")
	} else if !strings.Contains(err.Error(), "no Kaggle API credentials found") {
		t.Errorf("Submissions: wrapped error must carry the CLI's stderr; got: %v", err)
	}
}
