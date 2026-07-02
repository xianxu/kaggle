package kaggle

import (
	"encoding/csv"
	"fmt"
	"strconv"
	"strings"
)

// Column names of `kaggle competitions submissions --csv`. AUTHORED from the CLI
// docs (see testdata/submissions.csv). Both ParseSubmissions (the consumer) and
// FormatSubmissionsCSV (the fake's producer) derive from THIS one set, so they
// cannot drift from EACH OTHER — but note the shared blind spot: because fake and
// parser co-derive from this unvalidated schema, the fake structurally cannot
// catch a divergence from real Kaggle's ACTUAL columns/status vocabulary. That gap
// closes only by validating against the first live capture.
const (
	colFileName     = "fileName"
	colDate         = "date"
	colDescription  = "description"
	colStatus       = "status"
	colPublicScore  = "publicScore"
	colPrivateScore = "privateScore"
)

var submissionsCSVHeader = []string{colFileName, colDate, colDescription, colStatus, colPublicScore, colPrivateScore}

// ParseSubmissions turns `kaggle competitions submissions --csv` stdout into typed
// Submissions. Header-driven: columns are looked up by NAME, not index, so a CLI
// that reorders columns still parses. An empty OR non-numeric publicScore cell →
// nil (unscored): a single odd score (e.g. "-"/"None" on an errored or pending
// row) must NOT discard every valid row — LatestScored skips unscored rows and
// finds the newest validly-scored one. Pure — no IO. Competition is left empty (the
// CLI output is already scoped by `-c <slug>`; the submit step fills it).
//
// The production parser is fixture-agnostic (no comment-line handling); the test
// strips the authored fixture's provenance header before calling this.
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
		s := Submission{
			File:        get(row, colFileName),
			SubmittedAt: get(row, colDate),
			Message:     get(row, colDescription),
			Status:      get(row, colStatus),
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
		_ = w.Write([]string{s.File, s.SubmittedAt, s.Message, s.Status, score, ""})
	}
	w.Flush()
	return b.String()
}
