package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sha(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// TestVerifyPins_ComputesAndExcludesContractFiles: the computed map covers data files
// (recursive, slash-relative keys) and EXCLUDES the top-level runner contract files —
// mirroring metis collectArtifacts, so a config can never pin its own hash (with.json
// in the pin block would be a self-reference loop).
func TestVerifyPins_ComputesAndExcludesContractFiles(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"train.csv":       "a,b\n1,2\n",
		"sub/extra.csv":   "x\n",
		"with.json":       `{"competition":{"slug":"t"}}`,
		"metrics.json":    `{}`,
		"reads.json":      `{}`,
		"sub/reads.json":  "nested is data\n", // nested name-alike is a genuine artifact
	})
	computed, err := verifyPins(dir, nil)
	if err != nil {
		t.Fatalf("verifyPins: %v", err)
	}
	want := map[string]string{
		"train.csv":      sha("a,b\n1,2\n"),
		"sub/extra.csv":  sha("x\n"),
		"sub/reads.json": sha("nested is data\n"),
	}
	if len(computed) != len(want) {
		t.Errorf("computed keys = %v, want exactly %v", keys(computed), keys(want))
	}
	for k, v := range want {
		if computed[k] != v {
			t.Errorf("computed[%s] = %s, want %s", k, computed[k], v)
		}
	}
}

// TestVerifyPins_MatchOK: matching pins over all files → nil error.
func TestVerifyPins_MatchOK(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"train.csv": "A", "test.csv": "B"})
	pins := map[string]string{"train.csv": sha("A"), "test.csv": sha("B")}
	if _, err := verifyPins(dir, pins); err != nil {
		t.Fatalf("matching pins must verify: %v", err)
	}
}

// TestVerifyPins_ReportsAllFailures: one mismatch + one missing pin + one unpinned
// extra file — ALL named in one error (a declared identity is complete, not partial;
// an extra file is changed content just like a mismatch).
func TestVerifyPins_ReportsAllFailures(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"train.csv": "MUTATED", "surprise.csv": "new"})
	pins := map[string]string{
		"train.csv": sha("original"), // mismatch
		"gone.csv":  sha("gone"),     // missing
	}
	_, err := verifyPins(dir, pins)
	if err == nil {
		t.Fatal("want error, got nil")
	}
	for _, wantSub := range []string{"train.csv", "gone.csv", "surprise.csv"} {
		if !strings.Contains(err.Error(), wantSub) {
			t.Errorf("error must name %q; got: %v", wantSub, err)
		}
	}
}

// TestVerifyPins_EmptyPinsNoError: no pins → nil error, computed map still returned
// (feeds the paste-ready block).
func TestVerifyPins_EmptyPinsNoError(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"train.csv": "A"})
	computed, err := verifyPins(dir, map[string]string{})
	if err != nil || len(computed) != 1 {
		t.Fatalf("computed=%v err=%v; want 1 entry, nil", computed, err)
	}
}

// TestPinBlock_SortedPasteReady: renders a sorted, paste-ready YAML sha256 block.
func TestPinBlock_SortedPasteReady(t *testing.T) {
	got := pinBlock(map[string]string{"b.csv": "22", "a.csv": "11"})
	want := "sha256:\n  a.csv: 11\n  b.csv: 22\n"
	if got != want {
		t.Errorf("pinBlock = %q, want %q", got, want)
	}
}

func keys(m map[string]string) []string {
	var ks []string
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}
