package kaggle

import (
	"encoding/csv"
	"fmt"
	"strconv"
	"strings"
)

// Column names of `kaggle competitions submissions --csv`, VALIDATED against the
// first live capture (kaggle#8; see testdata/submissions.csv and the dated archive
// in workshop/captures/). Both ParseSubmissions (the consumer) and
// FormatSubmissionsCSV (the fake's producer) derive from THIS one set, so they
// cannot drift from each other; the live-conformance test in internal/kagglecli
// checks this set against the real CLI so they cannot silently drift from KAGGLE
// either — the blind spot that made the pre-capture schema wrong in three ways.
const (
	colRef          = "ref"
	colFileName     = "fileName"
	colDate         = "date"
	colDescription  = "description"
	colStatus       = "status"
	colPublicScore  = "publicScore"
	colPrivateScore = "privateScore"
)

var submissionsCSVHeader = []string{colRef, colFileName, colDate, colDescription, colStatus, colPublicScore, colPrivateScore}

// SubmissionsCSVHeader returns the expected `--csv` column set. Exported for the
// live-conformance test in internal/kagglecli, which must assert against the ONE
// schema definition rather than restating the column list (ARCH-DRY).
func SubmissionsCSVHeader() []string {
	out := make([]string, len(submissionsCSVHeader))
	copy(out, submissionsCSVHeader)
	return out
}

// ParseSubmissions turns `kaggle competitions submissions --csv` stdout into typed
// Submissions. Header-driven: columns are looked up by NAME, not index, so a CLI
// that reorders columns still parses. An empty OR non-numeric publicScore cell →
// nil (unscored): a single odd score (e.g. "-"/"None" on an errored or pending
// row) must NOT discard every valid row — LatestScored skips unscored rows and
// finds the newest validly-scored one. Pure — no IO. Competition is left empty (the
// CLI output is already scoped by `-c <slug>`; the submit step fills it).
//
// It also NORMALIZES the status here — this is the single CLI-text↔typed-state
// boundary, so it is the right and only place to reconcile Kaggle's wire
// vocabulary (`SubmissionStatus.COMPLETE`) with the constants consumers compare
// against. Status gets the normalized value, StatusRaw the wire string. Doing it
// here is what keeps every downstream comparison correct without a call-site sweep,
// and keeps submission.json's cross-repo shape stable (kaggle#8).
//
// The production parser is fixture-agnostic (no comment-line handling); the test
// strips the fixture's provenance header before calling this.
func ParseSubmissions(out string) ([]Submission, error) {
	if strings.TrimSpace(out) == "" {
		return nil, nil
	}
	r := csv.NewReader(strings.NewReader(out))
	r.FieldsPerRecord = -1 // tolerate ragged rows (a trailing empty score column)
	rows, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("kaggle: parse submissions csv: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	idx := map[string]int{}
	for i, name := range rows[0] {
		idx[strings.TrimSpace(name)] = i
	}
	if _, ok := idx[colFileName]; !ok {
		return nil, fmt.Errorf("kaggle: submissions csv missing %q column (header: %v)", colFileName, rows[0])
	}
	get := func(row []string, col string) string {
		if i, ok := idx[col]; ok && i < len(row) {
			return strings.TrimSpace(row[i])
		}
		return ""
	}
	var subs []Submission
	for _, row := range rows[1:] {
		if len(row) == 0 {
			continue
		}
		raw := get(row, colStatus)
		s := Submission{
			Ref:         get(row, colRef), // absent in pre-kaggle#8 captures -> ""
			File:        get(row, colFileName),
			SubmittedAt: get(row, colDate),
			Message:     get(row, colDescription),
			Status:      NormalizeStatus(raw), // the CLI↔state boundary is where the wire
			StatusRaw:   raw,                  // vocabulary is reconciled — see submission.go
		}
		if ps := get(row, colPublicScore); ps != "" {
			if v, err := strconv.ParseFloat(ps, 64); err == nil {
				s.PublicScore = &v // else leave unscored (nil); don't fail the whole parse
			}
		}
		subs = append(subs, s)
	}
	return subs, nil
}

// LatestScored returns the newest Submission carrying a public score. Kaggle lists
// submissions newest-first, so the first scored row is the newest scored; ok is
// false when none are scored yet. Pure.
//
// NOTE: this is a "best current score across ALL submissions" query. It is NOT the
// right selector for correlating a JUST-uploaded submission to its score — that
// must key off the newest row (subs[0]) matched to the uploaded file, else a
// competition with prior scored submissions reports an older one's score. See
// cmd/kaggle-submit/main.go pollScore.
func LatestScored(subs []Submission) (Submission, bool) {
	for _, s := range subs {
		if s.Scored() {
			return s, true
		}
	}
	return Submission{}, false
}

// FormatSubmissionsCSV renders Submissions in the `--csv` schema (submissionsCSVHeader).
// The process-level fake uses this so its output and ParseSubmissions share ONE
// schema definition (ARCH-DRY). Newest-first ordering is the caller's job.
func FormatSubmissionsCSV(subs []Submission) string {
	var b strings.Builder
	w := csv.NewWriter(&b)
	_ = w.Write(submissionsCSVHeader)
	for _, s := range subs {
		score := ""
		if s.PublicScore != nil {
			score = strconv.FormatFloat(*s.PublicScore, 'f', -1, 64)
		}
		// Wire fidelity is the DEFAULT, not an opt-in: a Submission built in code (the
		// fake, tests) carries no StatusRaw, and falling back to the normalized word
		// would render `complete` — a spelling Kaggle never emits. Derive it instead.
		status := s.StatusRaw
		if status == "" {
			status = WireStatus(s.Status)
		}
		_ = w.Write([]string{s.Ref, s.File, s.SubmittedAt, s.Message, status, score, ""})
	}
	w.Flush()
	return b.String()
}
