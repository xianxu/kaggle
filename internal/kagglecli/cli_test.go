package kagglecli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xianxu/kaggle/pkg/kaggle"
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
	t.Setenv("KAGGLE_FAKE", "1") // skip the credential precheck for the stub
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

// checkCredentials is where the auth IO (env read + kaggle.json stat) happens and
// feeds the pure kaggle.CredentialSource. Exercised here (not in pkg/kaggle).
func TestCheckCredentials(t *testing.T) {
	// KAGGLE_FAKE=1 → skip the precheck entirely (fake needs no auth).
	t.Setenv("KAGGLE_FAKE", "1")
	if err := checkCredentials(); err != nil {
		t.Fatalf("KAGGLE_FAKE=1 should skip precheck, got %v", err)
	}

	// Real path, env pair present → ok (temp HOME has no kaggle.json).
	t.Setenv("KAGGLE_FAKE", "")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("KAGGLE_USERNAME", "u")
	t.Setenv("KAGGLE_KEY", "k")
	if err := checkCredentials(); err != nil {
		t.Fatalf("env pair present: want nil, got %v", err)
	}

	// Real path, nothing present → ErrNoCredentials.
	t.Setenv("KAGGLE_USERNAME", "")
	t.Setenv("KAGGLE_KEY", "")
	if err := checkCredentials(); !errors.Is(err, kaggle.ErrNoCredentials) {
		t.Fatalf("no creds: want ErrNoCredentials, got %v", err)
	}

	// Real path, kaggle.json present (env empty) → ok.
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".kaggle"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".kaggle", "kaggle.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := checkCredentials(); err != nil {
		t.Fatalf("kaggle.json present: want nil, got %v", err)
	}
}
