// Command fake-kaggle is a PROCESS-LEVEL fake of the official `kaggle` CLI for
// hermetic e2e tests (per the "model external services" rule — a real subprocess
// speaking the CLI's surface, not a function mock). It implements only the
// subcommands the kaggle layer uses: competitions {download, submit, submissions}.
// State lives in $KAGGLE_FAKE_STATE so a submit→submissions sequence is coherent,
// and it models the ASYNC scoring TRANSITION: pending for the first
// $KAGGLE_FAKE_SCORE_AFTER (default 1) submissions polls, then complete+scored —
// so a consumer's poll loop actually iterates ≥1 time.
//
// SCHEMA PROVENANCE (kaggle#8, 2026-07-28): submissions output is produced via
// kaggle.FormatSubmissionsCSV — the same schema pkg/kaggle parses — so fake and parser
// can't drift from each other; and that schema is now VALIDATED against a live capture
// (pkg/kaggle/testdata/submissions.csv) plus a live-conformance check
// (internal/kagglecli/conformance_live_test.go). The former blind spot is closed.
//
// STATES THIS FAKE DOES NOT MODEL (deliberate, so nobody reads its behavior as the
// contract):
//   - complete-WITHOUT-score: real Kaggle reports SubmissionStatus.COMPLETE with an
//     empty publicScore (captured: submission 54846753). Here complete and scored flip
//     together in one transition, so Scored() and Status agree — on live they need not.
//   - the prior row's "prior.csv" filename: on live EVERY row is submission.csv, which
//     is why pollScore cannot correlate by name (kaggle#9). Kept unfaithful on purpose —
//     flipping it to submission.csv is that issue's RED test.
package main

import (
	"archive/zip"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/xianxu/kaggle/pkg/kaggle"
)

const (
	fakeScore = 0.775
	// Live wire shape (kaggle#8 capture): space-separated, microseconds, NO timezone.
	// Fixed value keeps tests deterministic.
	fakeSubmittedAt = "2026-07-01 00:00:00.000000"
	fakeRefBase     = 50000000 // refs are minted monotonically from here
)

type fakeState struct {
	File    string `json:"file"`
	Message string `json:"message"`
	Polls   int    `json:"polls"`
	// Ref is minted at submit and echoed at submissions — real Kaggle assigns an id per
	// submission, and it is the only field that distinguishes one from another (every
	// fileName is submission.csv on live; see kaggle#9). A fake that omitted it would
	// leave the parser's ref column exercised only by unit tests, never through the seam.
	Ref int `json:"ref"`
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "fake-kaggle:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	if len(args) < 2 || args[0] != "competitions" {
		return fmt.Errorf("unsupported command: %v", args)
	}
	rest := args[2:]
	switch args[1] {
	case "download":
		return doDownload(rest, stdout)
	case "submit":
		return doSubmit(rest, stdout)
	case "submissions":
		return doSubmissions(rest, stdout)
	default:
		return fmt.Errorf("unsupported subcommand: competitions %s", args[1])
	}
}

func doDownload(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("download", flag.ContinueOnError)
	slug := fs.String("c", "", "competition")
	dest := fs.String("p", "", "path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *slug == "" || *dest == "" {
		return fmt.Errorf("download requires -c and -p")
	}
	if err := os.MkdirAll(*dest, 0o755); err != nil {
		return err
	}
	files, err := downloadFiles(os.Getenv("KAGGLE_FAKE_DATA_DIR"))
	if err != nil {
		return err
	}
	// Real `kaggle competitions download` yields a .zip — mirror that shape.
	zpath := filepath.Join(*dest, *slug+".zip")
	if err := writeZip(zpath, files); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Downloading %s.zip to %s\n", *slug, *dest)
	return nil
}

// downloadFiles returns the files to pack into the download zip. When dir is
// non-empty (KAGGLE_FAKE_DATA_DIR), it serves every top-level regular file in
// that dir byte-for-byte — competition-agnostic, so a consumer points it at a
// committed fixture carrying the REAL column shapes for a full-thread e2e.
// Subdirectories are skipped (fixtures are flat). A dir that is missing or holds
// no regular files is an error, not a silent empty zip. When dir is "" (env
// unset or empty), it falls back to the minimal PassengerId,Survived stub —
// enough for the kaggle layer's own download→unzip e2e (back-compat).
func downloadFiles(dir string) (map[string]string, error) {
	if dir == "" {
		return map[string]string{
			"train.csv": "PassengerId,Survived\n1,0\n2,1\n",
			"test.csv":  "PassengerId\n3\n4\n",
		}, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("KAGGLE_FAKE_DATA_DIR %q: %w", dir, err)
	}
	files := map[string]string{}
	for _, e := range entries {
		if e.IsDir() {
			continue // flat fixtures — top-level files only
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		files[e.Name()] = string(b)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("KAGGLE_FAKE_DATA_DIR %q has no regular files", dir)
	}
	return files, nil
}

func doSubmit(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("submit", flag.ContinueOnError)
	slug := fs.String("c", "", "competition")
	file := fs.String("f", "", "file")
	msg := fs.String("m", "", "message")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *slug == "" || *file == "" {
		return fmt.Errorf("submit requires -c and -f")
	}
	dir, err := stateDir()
	if err != nil {
		return err
	}
	ref := fakeRefBase
	if prev, err := readState(dir, *slug); err == nil && prev.Ref >= fakeRefBase {
		ref = prev.Ref + 1 // monotonic: a resubmit to the same competition gets a new id
	}
	if err := writeState(dir, *slug, fakeState{File: filepath.Base(*file), Message: *msg, Ref: ref}); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Successfully submitted to %s\n", *slug)
	return nil
}

func doSubmissions(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("submissions", flag.ContinueOnError)
	slug := fs.String("c", "", "competition")
	fs.Bool("csv", false, "csv output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *slug == "" {
		return fmt.Errorf("submissions requires -c")
	}
	dir, err := stateDir()
	if err != nil {
		return err
	}
	st, err := readState(dir, *slug)
	if err != nil {
		return err
	}
	st.Polls++
	if err := writeState(dir, *slug, st); err != nil {
		return err
	}
	sub := kaggle.Submission{
		Ref:         strconv.Itoa(st.Ref),
		File:        st.File,
		Message:     st.Message,
		SubmittedAt: fakeSubmittedAt,
		Status:      kaggle.StatusPending,
		StatusRaw:   kaggle.WireStatus(kaggle.StatusPending), // emit the WIRE spelling, not ours
	}
	if st.Polls > scoreAfter() { // async: scored only after N polls
		score := fakeScore
		sub.Status = kaggle.StatusComplete
		sub.StatusRaw = kaggle.WireStatus(kaggle.StatusComplete)
		sub.PublicScore = &score
	}
	// Newest-first: our just-submitted row leads. KAGGLE_FAKE_PRIOR_SCORE models a
	// competition that ALREADY has an older scored submission — the case that
	// distinguishes "report OUR score" from "report any scored row" (a real bug the
	// single-submission fake couldn't reach).
	rows := []kaggle.Submission{sub}
	if prior, ok := priorScore(); ok {
		rows = append(rows, kaggle.Submission{
			Ref:     strconv.Itoa(st.Ref - 1),
			File:    "prior.csv",        // UNFAITHFUL: on live every row is submission.csv, so this
			Message: "prior submission", // filename can't occur. Left in deliberately —
			// internal/submit still correlates by filename, and kaggle#9 fixes that by
			// correlating on Ref; flipping this to submission.csv is #9's RED test.
			SubmittedAt: "2026-06-30 00:00:00.000000",
			Status:      kaggle.StatusComplete,
			StatusRaw:   kaggle.WireStatus(kaggle.StatusComplete),
			PublicScore: &prior,
		})
	}
	fmt.Fprint(stdout, kaggle.FormatSubmissionsCSV(rows))
	return nil
}

// priorScore reports a pre-existing older scored submission, if KAGGLE_FAKE_PRIOR_SCORE is set.
func priorScore() (float64, bool) {
	if v := os.Getenv("KAGGLE_FAKE_PRIOR_SCORE"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f, true
		}
	}
	return 0, false
}

func scoreAfter() int {
	if v := os.Getenv("KAGGLE_FAKE_SCORE_AFTER"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return 1
}

func stateDir() (string, error) {
	d := os.Getenv("KAGGLE_FAKE_STATE")
	if d == "" {
		return "", fmt.Errorf("KAGGLE_FAKE_STATE not set")
	}
	if err := os.MkdirAll(d, 0o755); err != nil {
		return "", err
	}
	return d, nil
}

func statePath(dir, slug string) string { return filepath.Join(dir, slug+".json") }

func writeState(dir, slug string, st fakeState) error {
	b, err := json.Marshal(st)
	if err != nil {
		return err
	}
	return os.WriteFile(statePath(dir, slug), b, 0o644)
}

func readState(dir, slug string) (fakeState, error) {
	b, err := os.ReadFile(statePath(dir, slug))
	if err != nil {
		return fakeState{}, fmt.Errorf("no submission for %q (submit first): %w", slug, err)
	}
	var st fakeState
	if err := json.Unmarshal(b, &st); err != nil {
		return fakeState{}, err
	}
	return st, nil
}

func writeZip(path string, files map[string]string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			return err
		}
		if _, err := io.WriteString(w, content); err != nil {
			return err
		}
	}
	return zw.Close()
}
