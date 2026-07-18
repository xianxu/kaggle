package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// verifyPins hashes the step dir's data files (recursive; slash-relative keys) and,
// when pins are declared, verifies the DOWNLOADED CONTENT IS the declared identity
// (metis#25, the Nix fixed-output-derivation model): every pinned file must exist and
// match, and every file present must be pinned — an extra file is changed content just
// like a mismatch. All failures are reported in one error. The top-level runner
// contract files (with.json / metrics.json / reads.json) are excluded, mirroring metis
// collectArtifacts — otherwise the paste-ready block would pin with.json's own hash and
// pasting it would mutate with.json into a permanent mismatch loop.
func verifyPins(dir string, pins map[string]string) (map[string]string, error) {
	computed := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Dir(path) == dir {
			if n := d.Name(); n == "with.json" || n == "metrics.json" || n == "reads.json" {
				return nil // runner contract channels (top level only)
			}
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		h, err := hashFile(path)
		if err != nil {
			return err
		}
		computed[filepath.ToSlash(rel)] = h
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(pins) == 0 {
		return computed, nil
	}
	var fails []string
	for _, name := range sortedPinKeys(pins) {
		got, ok := computed[name]
		switch {
		case !ok:
			fails = append(fails, fmt.Sprintf("pinned file missing: %s", name))
		case got != pins[name]:
			fails = append(fails, fmt.Sprintf("content mismatch: %s (pinned %s, got %s)", name, pins[name], got))
		}
	}
	for _, name := range sortedPinKeys(computed) {
		if _, ok := pins[name]; !ok {
			fails = append(fails, fmt.Sprintf("unpinned file present: %s (declare it or it is changed content)", name))
		}
	}
	if len(fails) > 0 {
		return computed, fmt.Errorf("content pins failed (metis#25 — the remote data does not match the declared identity):\n  %s",
			strings.Join(fails, "\n  "))
	}
	return computed, nil
}

// pinBlock renders the computed hashes as a paste-ready `sha256:` YAML map, sorted.
func pinBlock(computed map[string]string) string {
	var b strings.Builder
	b.WriteString("sha256:\n")
	for _, name := range sortedPinKeys(computed) {
		fmt.Fprintf(&b, "  %s: %s\n", name, computed[name])
	}
	return b.String()
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func sortedPinKeys(m map[string]string) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}
