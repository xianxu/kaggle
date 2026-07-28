package kaggle

import "strings"

// Submission status values — the NORMALIZED vocabulary (see NormalizeStatus).
//
// The wire format is a Python enum repr (`SubmissionStatus.COMPLETE`), VALIDATED against the first
// live capture (kaggle#8, testdata/submissions.csv). ParseSubmissions normalizes at the CLI↔state
// boundary, so consumers compare against these constants while Submission.StatusRaw keeps the wire
// value. Do NOT compare raw CLI output against these.
//
// StatusError is SHAPE-INFERRED, never observed: no captured submission has errored, so `error` is
// what `SubmissionStatus.ERROR` *would* normalize to given the observed prefix shape. Treat a match
// as unconfirmed until a real errored submission is captured.
const (
	StatusPending  = "pending"
	StatusComplete = "complete"
	StatusError    = "error"
)

// StatusWirePrefix is the enum-repr prefix real Kaggle emits (`SubmissionStatus.COMPLETE`).
// Exported as the SINGLE source of the wire shape: NormalizeStatus strips it, WireStatus adds
// it, and the live-conformance check builds its assertion from it. The column set is
// single-sourced by SubmissionsCSVHeader for the same reason — and the prefix is the half that
// was actually wrong before kaggle#8, so restating it anywhere is the bug's own shape.
const StatusWirePrefix = "SubmissionStatus."

// NormalizeStatus maps a raw CLI status onto the normalized vocabulary: it strips the
// `SubmissionStatus.` enum-repr prefix and folds case, so `SubmissionStatus.COMPLETE` → `complete`.
//
// It is deliberately TOTAL: an unrecognized status is normalized and PRESERVED
// (`SubmissionStatus.WEIRD` → `weird`), never rejected or coerced. This package guessed the
// vocabulary once (kaggle#8) and shipped three constants that matched no live value; tolerating the
// unknown is the other half of that fix. Pure.
func NormalizeStatus(raw string) string {
	s := strings.TrimSpace(raw)
	if len(s) >= len(StatusWirePrefix) && strings.EqualFold(s[:len(StatusWirePrefix)], StatusWirePrefix) {
		s = s[len(StatusWirePrefix):]
	}
	return strings.ToLower(s)
}

// WireStatus renders a normalized status the way the CLI prints it — the inverse of
// NormalizeStatus, e.g. "complete" -> "SubmissionStatus.COMPLETE". The fake uses it so its
// output teaches the real shape rather than our internal vocabulary.
func WireStatus(normalized string) string {
	if normalized == "" {
		return ""
	}
	return StatusWirePrefix + strings.ToUpper(normalized)
}

// Submission is one upload's durable record, serialized as submission.json (a step artifact).
// PublicScore is a *float64 because Kaggle scores asynchronously: it is nil until a later
// `submissions` poll reports the score.
//
// Status carries the NORMALIZED value and is a cross-repo contract (kbench's e2e asserts
// submission.json's `status` == "complete"); StatusRaw carries the wire string verbatim and is
// additive.
//
// NOTE: Scored() — not Status — is the scored/unscored test. Real Kaggle reports COMPLETE rows with
// an EMPTY publicScore (captured: submission 54846753), so "complete" does not imply scored.
type Submission struct {
	Competition string   `json:"competition,omitempty"` // set by the submit step (not in CLI output)
	Ref         string   `json:"ref,omitempty"`         // Kaggle's submission id — the stable handle
	File        string   `json:"file"`
	Message     string   `json:"message,omitempty"`
	SubmittedAt string   `json:"submitted_at,omitempty"` // wire shape: "2026-07-28 15:17:26.513000" (no zone)
	Status      string   `json:"status"`                 // normalized; see the constants above
	StatusRaw   string   `json:"status_raw,omitempty"`   // exactly as the CLI printed it
	PublicScore *float64 `json:"public_score,omitempty"`
}

// Scored reports whether Kaggle has assigned a public score yet.
func (s Submission) Scored() bool { return s.PublicScore != nil }
