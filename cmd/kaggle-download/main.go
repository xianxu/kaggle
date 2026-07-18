// Command kaggle-download is the `kaggle/download` step-type: the download half of
// an Adapter. It authenticates + pulls a competition's data via the official
// `kaggle` CLI (injectable — a fake stands in for tests), then UNZIPS the CLI's
// .zip into the step dir and drops the zip, so the artifacts metis records are the
// loose data files (train.csv/test.csv). kbench's `adapt` step consumes that loose
// shape. See workshop/plans/000001-* Chunk 2 Task 2.
package main

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/xianxu/kaggle/internal/kagglecli"
	"github.com/xianxu/kaggle/internal/stepio"
	"github.com/xianxu/kaggle/pkg/kaggle"
)

// downloadWith is this step's `with` config, read from with.json.
type downloadWith struct {
	Competition kaggle.Competition `json:"competition"`
	// Sha256 declares the expected content identity of the download, per extracted
	// file (metis#25, fixed-output-derivation): {slash-relative-name: sha256hex}.
	// Present → verified after unzip, mismatch/missing/extra FAILS the step loudly.
	// Absent → the step prints a paste-ready pin block (unpinned ingest is loud).
	// Because `with` is Kpre material in metis, editing a pin re-keys get-data and
	// everything downstream — content identity rides the existing cache channel.
	Sha256 map[string]string `json:"sha256"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "kaggle/download:", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, err := stepio.New()
	if err != nil {
		return err
	}
	var w downloadWith
	if err := ctx.ReadWith(&w); err != nil {
		return err
	}
	if err := w.Competition.Validate(); err != nil {
		return err
	}

	cli := kagglecli.New()
	if err := cli.Download(w.Competition.Slug, ctx.StepDir); err != nil {
		return err
	}

	// The real CLI (and the fake) yield <slug>.zip. Unzip into the step dir so
	// downstream/adapt sees loose files, then remove the zip — the extracted files
	// become this step's artifacts.
	zips, err := filepath.Glob(filepath.Join(ctx.StepDir, "*.zip"))
	if err != nil {
		return err
	}
	if len(zips) == 0 {
		return fmt.Errorf("download of %q produced no .zip", w.Competition.Slug)
	}
	for _, z := range zips {
		if err := unzip(z, ctx.StepDir); err != nil {
			return fmt.Errorf("unzip %s: %w", filepath.Base(z), err)
		}
		if err := os.Remove(z); err != nil {
			return err
		}
	}

	// metis#25: content identity. Pins declared → verify (any drift fails the step,
	// so a changed remote can never silently propagate downstream). No pins → loud
	// note + a paste-ready block, so declaring identity is one paste away.
	computed, err := verifyPins(ctx.StepDir, w.Sha256)
	if err != nil {
		return err
	}
	if len(w.Sha256) == 0 {
		fmt.Fprintf(os.Stderr,
			"kaggle/download: UNPINNED ingest — content identity not declared (metis#25); paste into this step's with:\n%s",
			pinBlock(computed))
	}
	return nil
}

// unzip extracts src into destDir (flat + nested entries), guarding against
// zip-slip (an entry escaping destDir via ../).
func unzip(src, destDir string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()
	cleanDest := filepath.Clean(destDir)
	for _, f := range r.File {
		target := filepath.Join(destDir, f.Name)
		if target != cleanDest && !strings.HasPrefix(target, cleanDest+string(os.PathSeparator)) {
			return fmt.Errorf("unsafe zip entry %q escapes %s", f.Name, destDir)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := extractOne(f, target); err != nil {
			return err
		}
	}
	return nil
}

func extractOne(f *zip.File, target string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.Create(target)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, rc)
	return err
}
