package main

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
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

// downloadFiles: empty dir arg (env unset or "") → the back-compat stub. The
// kaggle-layer's own e2e relies on this exact shape, so it must not flip to error.
func TestDownloadFilesStubWhenUnset(t *testing.T) {
	files, err := downloadFiles("")
	if err != nil {
		t.Fatal(err)
	}
	if got := files["train.csv"]; got != "PassengerId,Survived\n1,0\n2,1\n" {
		t.Errorf("stub train.csv = %q", got)
	}
	if _, ok := files["test.csv"]; !ok {
		t.Errorf("stub missing test.csv")
	}
}

// KAGGLE_FAKE_DATA_DIR → serve every top-level regular file byte-for-byte
// (competition-agnostic real column shapes).
func TestDownloadFilesServesFixtureDir(t *testing.T) {
	dir := t.TempDir()
	want := map[string]string{
		"train.csv": "PassengerId,Survived,Pclass,Sex,Age,SibSp,Parch,Fare\n1,0,3,male,22,1,0,7.25\n",
		"test.csv":  "PassengerId,Pclass,Sex,Age,SibSp,Parch,Fare\n3,3,female,26,0,0,7.92\n",
	}
	for name, content := range want {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0o755); err != nil { // must be skipped, not error
		t.Fatal(err)
	}
	got, err := downloadFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(want) {
		t.Fatalf("got %d files, want %d (subdir must be skipped): %v", len(got), len(want), got)
	}
	for name, content := range want {
		if got[name] != content {
			t.Errorf("%s = %q, want %q", name, got[name], content)
		}
	}
}

func TestDownloadFilesMissingDirErrors(t *testing.T) {
	if _, err := downloadFiles(filepath.Join(t.TempDir(), "does-not-exist")); err == nil {
		t.Fatal("a set-but-missing KAGGLE_FAKE_DATA_DIR path must error, not a silent empty zip")
	}
}

func TestDownloadFilesEmptyDirErrors(t *testing.T) {
	if _, err := downloadFiles(t.TempDir()); err == nil {
		t.Fatal("KAGGLE_FAKE_DATA_DIR with no regular files must error, not a silent empty zip")
	}
}

// End-to-end via doDownload: env → downloadFiles → a real zip whose UNZIPPED bytes
// equal the fixture (the point — real column shapes reach the consumer's unzip path).
func TestFakeDownloadServesFixtureBytesEndToEnd(t *testing.T) {
	data := t.TempDir()
	train := "PassengerId,Survived,Pclass,Sex,Age,SibSp,Parch,Fare\n1,0,3,male,22,1,0,7.25\n"
	if err := os.WriteFile(filepath.Join(data, "train.csv"), []byte(train), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KAGGLE_FAKE_DATA_DIR", data)
	t.Setenv("KAGGLE_FAKE_STATE", t.TempDir())
	dest := t.TempDir()
	var out bytes.Buffer
	if err := run([]string{"competitions", "download", "-c", "titanic", "-p", dest}, &out); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.OpenReader(filepath.Join(dest, "titanic.zip"))
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	found := ""
	for _, f := range zr.File {
		if f.Name == "train.csv" {
			rc, err := f.Open()
			if err != nil {
				t.Fatal(err)
			}
			b, _ := io.ReadAll(rc)
			rc.Close()
			found = string(b)
		}
	}
	if found != train {
		t.Errorf("unzipped train.csv = %q, want fixture bytes %q", found, train)
	}
}
