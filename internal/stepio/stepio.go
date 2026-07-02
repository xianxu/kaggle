// Package stepio is the Go step-side reader of the metis step contract: the
// METIS_* env vars the runner sets, the with.json input, and the metrics.json +
// artifact outputs. metis encodes this contract on the runner side
// (metis/cmd/metis/exec.go) and the Python step side (metis/metis/io.py); kaggle
// is the first GO step-author, so it reads the contract here.
//
// Decision A2 (workshop/plans/000001-kaggle-platform-integration-plan.md): the
// contract strings are declared LOCALLY rather than imported from a metis package.
// With only two Go consumers today (the metis runner + kaggle) that is a
// rule-of-two, and importing would fold a peer-repo edit under kaggle's issue
// number; promote to metis/pkg/stepcontract when kbench becomes the 3rd Go
// step-author. The authoritative prose is metis atlas/experiment.md
// "### Step-executable contract". DRIFT is caught by the M2 e2e: it drives the
// REAL metis binary (which emits these exact METIS_* names) against steps whose
// New() REQUIRES them from env — a renamed var makes the step exit non-zero and
// the run fail RED. These consts are never echoed back to stepio itself, so the
// guard is genuine.
package stepio

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

// Contract strings (provenance: metis atlas/experiment.md "### Step-executable
// contract"). Kept in sync by the M2 e2e, not by a shared import — see package doc.
const (
	envStepDir = "METIS_STEP_DIR" // absolute step dir (== cwd); with.json + outputs land here
	envRunDir  = "METIS_RUN_DIR"  // absolute run dir; upstream outputs at <RunDir>/<id>/
	envStepID  = "METIS_STEP_ID"  // this step's id
	envExpDir  = "METIS_EXP_DIR"  // absolute experiment dir (anchor for exp-relative inputs)
	envSeed    = "METIS_SEED"     // experiment seed, for reproducibility

	withFile    = "with.json"    // runner-written step input (the `with` config)
	metricsFile = "metrics.json" // step-written flat {name: number}; a reserved channel
)

// Context is the resolved step contract for one step invocation.
type Context struct {
	StepDir string // absolute; == cwd; where with.json lands + outputs go
	RunDir  string // absolute; upstream steps' outputs live at <RunDir>/<id>/
	StepID  string // this step's id
	ExpDir  string // absolute experiment dir (best-effort; kaggle steps don't consume it)
	Seed    int    // experiment seed (best-effort)
}

// New reads the METIS_* env the runner sets. It REQUIRES the three vars kaggle
// steps consume (StepDir/RunDir/StepID); an empty one is an error naming it — the
// drift-guard: a renamed metis var surfaces here as a hard failure, never a silent
// cwd fallback. ExpDir/Seed are read best-effort (kaggle steps don't need them, so
// coupling to them would be dishonest surface).
func New() (Context, error) {
	sd, err := requireEnv(envStepDir)
	if err != nil {
		return Context{}, err
	}
	rd, err := requireEnv(envRunDir)
	if err != nil {
		return Context{}, err
	}
	id, err := requireEnv(envStepID)
	if err != nil {
		return Context{}, err
	}
	seed, _ := strconv.Atoi(os.Getenv(envSeed)) // best-effort; 0 if unset/non-numeric
	return Context{StepDir: sd, RunDir: rd, StepID: id, ExpDir: os.Getenv(envExpDir), Seed: seed}, nil
}

func requireEnv(name string) (string, error) {
	v := os.Getenv(name)
	if v == "" {
		return "", fmt.Errorf("stepio: %s not set — a step must be launched by `metis run`", name)
	}
	return v, nil
}

// ReadWith unmarshals the step's with.json (the runner-written `with` config) into v.
func (c Context) ReadWith(v any) error {
	b, err := os.ReadFile(filepath.Join(c.StepDir, withFile))
	if err != nil {
		return fmt.Errorf("stepio: read %s: %w", withFile, err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		return fmt.Errorf("stepio: parse %s: %w", withFile, err)
	}
	return nil
}

// WriteMetrics writes the step's metrics.json (a flat {name: number} object the
// runner merges into Run.metrics). map[string]float64 mirrors the runner's
// readMetrics target exactly, so a non-numeric value can't slip through.
func (c Context) WriteMetrics(m map[string]float64) error {
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(c.StepDir, metricsFile), append(b, '\n'), 0o644)
}

// UpstreamPath is the path to a file an upstream step wrote: <RunDir>/<stepID>/<file>.
// The metis upstream-artifact convention: a downstream step names the upstream
// step's id; the filename is a convention of the step-type pair.
func (c Context) UpstreamPath(stepID, file string) string {
	return filepath.Join(c.RunDir, stepID, file)
}

// OutPath is the path for an output file this step writes into its own step dir.
func (c Context) OutPath(file string) string {
	return filepath.Join(c.StepDir, file)
}
