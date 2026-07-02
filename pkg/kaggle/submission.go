package kaggle

// Submission status values as reported by the Kaggle CLI's `competitions
// submissions` output. (Authored from the CLI's documented status vocabulary —
// VALIDATE against the first live capture, per testdata/submissions.csv.)
const (
	StatusPending  = "pending"
	StatusComplete = "complete"
	StatusError    = "error"
)

// Submission is one upload's durable record, serialized as submission.json (a step
// artifact). PublicScore is a *float64 because Kaggle scores asynchronously: it is
// nil until a later `submissions` poll reports the score.
type Submission struct {
	Competition string   `json:"competition,omitempty"` // set by the submit step (not in CLI output)
	File        string   `json:"file"`
	Message     string   `json:"message,omitempty"`
	SubmittedAt string   `json:"submitted_at,omitempty"`
	Status      string   `json:"status"`
	PublicScore *float64 `json:"public_score,omitempty"`
}

// Scored reports whether Kaggle has assigned a public score yet.
func (s Submission) Scored() bool { return s.PublicScore != nil }
