package main

import "encoding/json"

// slugFromRecordJSON extracts the competition slug from a run's record.json: the
// first steps[].with.competition.slug (a kaggle/download or /submit step carries
// it, even when the run has no submit step). A local minimal struct — kaggle
// imports no metis package (internal/stepio's declare-contract-strings-locally
// posture); the CLI reads only this one nested field.
func slugFromRecordJSON(b []byte) (string, bool) {
	var doc struct {
		Steps []struct {
			With map[string]any `json:"with"`
		} `json:"steps"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return "", false
	}
	for _, s := range doc.Steps {
		comp, ok := s.With["competition"].(map[string]any)
		if !ok {
			continue
		}
		if slug, ok := comp["slug"].(string); ok && slug != "" {
			return slug, true
		}
	}
	return "", false
}
