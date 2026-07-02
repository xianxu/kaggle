package stepio

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// setContractEnv sets the full METIS_* contract env for a step launched from
// stepDir/runDir; individual tests then blank one out to exercise the drift-guard.
func setContractEnv(t *testing.T, stepDir, runDir, stepID string) {
	t.Helper()
	t.Setenv(envStepDir, stepDir)
	t.Setenv(envRunDir, runDir)
	t.Setenv(envStepID, stepID)
	t.Setenv(envExpDir, filepath.Join(runDir, "..")) // arbitrary; best-effort
	t.Setenv(envSeed, "42")
}

func TestNew_ResolvesContract(t *testing.T) {
	stepDir, runDir := t.TempDir(), t.TempDir()
	setContractEnv(t, stepDir, runDir, "download")

	ctx, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if ctx.StepDir != stepDir || ctx.RunDir != runDir || ctx.StepID != "download" {
		t.Fatalf("resolved wrong: %+v", ctx)
	}
	if ctx.Seed != 42 {
		t.Errorf("Seed = %d, want 42", ctx.Seed)
	}
}

// TestNew_RequiresVars is the local half of the drift-guard: a missing (or
// renamed → absent) required METIS_* var is a hard error naming the var, never a
// silent cwd fallback. The e2e supplies the other half (real metis emits the names).
func TestNew_RequiresVars(t *testing.T) {
	for _, missing := range []string{envStepDir, envRunDir, envStepID} {
		t.Run(missing, func(t *testing.T) {
			setContractEnv(t, t.TempDir(), t.TempDir(), "s")
			t.Setenv(missing, "") // blank == unset for os.Getenv
			_, err := New()
			if err == nil {
				t.Fatalf("New with %s unset: want error, got nil", missing)
			}
			if !contains(err.Error(), missing) {
				t.Errorf("error %q should name the missing var %q", err, missing)
			}
		})
	}
}

func TestReadWith(t *testing.T) {
	stepDir, runDir := t.TempDir(), t.TempDir()
	setContractEnv(t, stepDir, runDir, "download")
	writeFile(t, filepath.Join(stepDir, withFile), `{"competition":{"slug":"titanic","metric":"accuracy"}}`)

	ctx, err := New()
	if err != nil {
		t.Fatal(err)
	}
	var w struct {
		Competition struct {
			Slug   string `json:"slug"`
			Metric string `json:"metric"`
		} `json:"competition"`
	}
	if err := ctx.ReadWith(&w); err != nil {
		t.Fatalf("ReadWith: %v", err)
	}
	if w.Competition.Slug != "titanic" || w.Competition.Metric != "accuracy" {
		t.Errorf("parsed wrong: %+v", w)
	}
}

func TestWriteMetrics(t *testing.T) {
	stepDir, runDir := t.TempDir(), t.TempDir()
	setContractEnv(t, stepDir, runDir, "submit")
	ctx, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if err := ctx.WriteMetrics(map[string]float64{"public_score": 0.775}); err != nil {
		t.Fatalf("WriteMetrics: %v", err)
	}
	// Round-trip into exactly the runner's target type (map[string]float64).
	b, err := os.ReadFile(filepath.Join(stepDir, metricsFile))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]float64
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("metrics.json not a flat {name:number}: %v", err)
	}
	if m["public_score"] != 0.775 {
		t.Errorf("public_score = %v, want 0.775", m["public_score"])
	}
}

func TestUpstreamAndOutPath(t *testing.T) {
	stepDir, runDir := t.TempDir(), t.TempDir()
	setContractEnv(t, stepDir, runDir, "submit")
	ctx, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := ctx.UpstreamPath("make-submission", "submission.csv"), filepath.Join(runDir, "make-submission", "submission.csv"); got != want {
		t.Errorf("UpstreamPath = %q, want %q", got, want)
	}
	if got, want := ctx.OutPath("submission.json"), filepath.Join(stepDir, "submission.json"); got != want {
		t.Errorf("OutPath = %q, want %q", got, want)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
