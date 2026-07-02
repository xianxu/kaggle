// Package kaggle models the durable STATE of Kaggle interaction — pure records
// plus the pure parsers that turn the official `kaggle` CLI's output into typed
// values. The TRANSPORT (shelling the CLI) lives in internal/kagglecli; this
// package is IO-free and table-tested (ARCH-PURE). The one place CLI text becomes
// typed state is ParseSubmissions (parse.go) — the single, fragile boundary.
package kaggle

import "errors"

// Competition is the thin config identifying a competition. It is supplied by an
// experiment's `with` (kbench sets it), not fetched from Kaggle.
type Competition struct {
	Slug     string `json:"slug"`               // e.g. "titanic"
	Metric   string `json:"metric,omitempty"`   // e.g. "accuracy"
	Deadline string `json:"deadline,omitempty"` // optional ISO date
}

// Validate reports whether the competition is well-formed (non-empty slug).
func (c Competition) Validate() error {
	if c.Slug == "" {
		return errors.New("kaggle: competition slug is required")
	}
	return nil
}
