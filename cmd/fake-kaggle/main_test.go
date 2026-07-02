package main

import (
	"archive/zip"
	"bytes"
	"path/filepath"
	"testing"

	"github.com/xianxu/kaggle/pkg/kaggle"
)

func TestFakeDownloadEmitsZip(t *testing.T) {
	t.Setenv("KAGGLE_FAKE_STATE", t.TempDir())
	dest := t.TempDir()
	var out bytes.Buffer
	if err := run([]string{"competitions", "download", "-c", "titanic", "-p", dest}, &out); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.OpenReader(filepath.Join(dest, "titanic.zip"))
	if err != nil {
		t.Fatalf("expected titanic.zip: %v", err)
	}
	defer zr.Close()
	names := map[string]bool{}
	for _, f := range zr.File {
		names[f.Name] = true
	}
	if !names["train.csv"] || !names["test.csv"] {
		t.Errorf("zip missing train/test.csv, has: %v", names)
	}
}

// The fake models the async scoring TRANSITION: pending for the first
// KAGGLE_FAKE_SCORE_AFTER polls, then complete+scored — so a consumer's poll loop
// actually iterates.
func TestFakeSubmitThenAsyncScoring(t *testing.T) {
	t.Setenv("KAGGLE_FAKE_STATE", t.TempDir())
	t.Setenv("KAGGLE_FAKE_SCORE_AFTER", "1")

	var out bytes.Buffer
	if err := run([]string{"competitions", "submit", "-c", "titanic", "-f", "sub.csv", "-m", "msg"}, &out); err != nil {
		t.Fatalf("submit: %v", err)
	}

	// Poll 1 → pending, unscored.
	out.Reset()
	if err := run([]string{"competitions", "submissions", "-c", "titanic", "--csv"}, &out); err != nil {
		t.Fatalf("submissions#1: %v", err)
	}
	subs, err := kaggle.ParseSubmissions(out.String())
	if err != nil {
		t.Fatal(err)
	}
	if len(subs) != 1 || subs[0].Status != kaggle.StatusPending || subs[0].Scored() {
		t.Fatalf("poll#1 want pending+unscored, got %+v", subs)
	}

	// Poll 2 → complete, scored.
	out.Reset()
	if err := run([]string{"competitions", "submissions", "-c", "titanic", "--csv"}, &out); err != nil {
		t.Fatalf("submissions#2: %v", err)
	}
	subs, _ = kaggle.ParseSubmissions(out.String())
	if len(subs) != 1 || subs[0].Status != kaggle.StatusComplete || !subs[0].Scored() {
		t.Fatalf("poll#2 want complete+scored, got %+v", subs)
	}
	if subs[0].File != "sub.csv" {
		t.Errorf("file = %q, want sub.csv", subs[0].File)
	}
}

func TestFakeSubmissionsBeforeSubmit(t *testing.T) {
	t.Setenv("KAGGLE_FAKE_STATE", t.TempDir())
	var out bytes.Buffer
	if err := run([]string{"competitions", "submissions", "-c", "titanic", "--csv"}, &out); err == nil {
		t.Fatal("submissions before submit should error")
	}
}
