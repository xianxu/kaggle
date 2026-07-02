// Package kaggletest holds shared test helpers so the process-level fake (and, in
// the e2e, the metis binary) is built one consistent way across the step-type and
// integration tests (ARCH-DRY — one build helper, not a copy per test package).
package kaggletest

import (
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
