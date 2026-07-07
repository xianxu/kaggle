# `kaggle submit` CLI — Implementation Plan

> **For agentic workers:** Consult AGENTS.md Section 3 (Subagent Strategy) to determine the appropriate execution approach: use superpowers-subagent-driven-development (if subagents are suitable per AGENTS.md) or superpowers-executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A thin `kaggle submit --run <id>` CLI that submits a run's `submission.csv` to Kaggle and returns its `public_score`, reusing the **same** submit+poll+auth path the `kaggle/submit` step uses — no pipeline edit for the ad-hoc "submit the promoted sweep winner" case.

**Architecture:** The submit→poll core (`pollScore` + the submit-then-poll orchestration + the `KAGGLE_SUBMIT_*` env helpers) currently lives **unexported in `cmd/kaggle-submit`'s `package main`**. Extract it into a new **`internal/submit`** package (one path, ARCH-DRY), have the step **refactor to call it**, and build a new **`cmd/kaggle`** binary whose `submit` subcommand also calls it. Auth is free — both go through `kagglecli.New()` (= `${KAGGLE_CLI:-kaggle}`), so the fake-kaggle test harness works identically. kaggle stays a **zero-metis-dependency** module: the CLI reads the competition slug from the run's `record.json` with a local minimal struct (or `-c`), consistent with `internal/stepio`'s "declare contract strings locally" posture.

**Tech Stack:** Go 1.26; `internal/kagglecli` (`New`/`Submit`/`Submissions`), `pkg/kaggle` (`ParseSubmissions`/`Submission`/`Competition`); `internal/kaggletest.WireFake` + `cmd/fake-kaggle` for hermetic tests; stdlib `flag`/`encoding/json`.

---

## Core concepts

### The reuse seam (what's already there vs. what moves)

- **Reuse as-is:** `kagglecli.{New,Submit,Submissions}` + `kaggle.{ParseSubmissions,Submission,Competition}`. Auth is delegated to the wrapped CLI (`KAGGLE_CLI` override = the only knob), so the fake works for free.
- **Extract (don't copy):** `pollScore` (`cmd/kaggle-submit/main.go:108`) + `envInt`/`envDuration` (`:149`/`:163`) are unexported in `package main`. They move to `internal/submit`. The issue's Done-when — "Reuses the `kaggle/submit` step's submit+poll logic (shared helper, not a copy) — one submit/auth path" — is satisfied **only** if the step is refactored to consume the extracted helper (not left with its own copy).

### Pure entities

| Name | Lives in | Status |
|------|----------|--------|
| `pollScore` | `internal/submit/submit.go` | moved (from `cmd/kaggle-submit`) |
| `SubmitAndPoll` | `internal/submit/submit.go` | new |
| `Submitter` (interface) | `internal/submit/submit.go` | new |
| `envInt` / `envDuration` | `internal/submit/env.go` | moved |
| `slugFromRecordJSON` | `cmd/kaggle/runref.go` | new |

- **pollScore** — the blocking newest-submission-correlated poll loop; moved verbatim (already pure: `submissionsFn func() (string,error)` + injected `sleep func(int)`). Its `TestPollScore_*` unit tests move with it.
  - **DRY rationale:** one poll path for the step and the CLI. Its subtle correctness (correlate to `subs[0]`/`wantFile`, not `LatestScored`; fast-fail on `status=error`; time-out safe) must exist once.

- **SubmitAndPoll** — new orchestration: `cli.Submit(slug, csv, msg)` then `pollScore(func(){cli.Submissions(slug)}, base(csv), maxAttempts, sleep)`; returns `(kaggle.Submission, scored bool, error)`. Callers decide what to do with the result (step writes `submission.json`+metrics; CLI prints the score).
  - **Relationships:** takes a `Submitter` (so it's unit-testable + reused by both callers); N callers : 1 helper.
  - **Injected into:** `cmd/kaggle-submit`'s `run()` and `cmd/kaggle`'s `submit`. The real `time.Sleep(delay)` is wrapped here; `pollScore`'s `sleep` stays injectable for tests.

- **Submitter** — `interface { Submit(slug, file, msg string) error; Submissions(slug string) (string, error) }`. `kagglecli.CLI` satisfies it structurally.
  - **DRY rationale:** decouples the orchestration from the concrete subprocess CLI (ARCH-PURE — the poll logic never touches `os/exec`).

- **slugFromRecordJSON** — pure `([]byte) (slug string, ok bool)`: parse a run's `record.json`, return the first `steps[].with.competition.slug`. Local minimal struct (no metis import — kaggle's zero-dep posture).
  - **Future extensions:** also fall back to the experiment `.md` frontmatter if a run ever lacks a kaggle step; deferred (YAGNI — `-c` covers it).

### Integration points

| Name | Lives in | Status | Wraps |
|------|----------|--------|-------|
| `cmd/kaggle` (submit subcommand) | `cmd/kaggle/main.go` | new | flags + FS + `kagglecli` + `submit` |
| `cmd/kaggle-submit` `run()` | `cmd/kaggle-submit/main.go` | modified | now calls `submit.SubmitAndPoll` |

- **cmd/kaggle** — new `bin/kaggle` (auto-built by Makefile.workflow from `cmd/kaggle/main.go` — no Makefile edit). `main()` dispatches `switch args[0]` (mirroring `fake-kaggle/main.go:47`); the first subcommand is `submit`. `submit` flags: `--run <id>`, `-f <file>` (explicit path, overrides `--run`), `-c <slug>` (competition, overrides record.json), `-m <message>`. Resolution: file = `-f` else `runs/<run>/submission/submission.csv` (cwd-relative); slug = `-c` else `slugFromRecordJSON(runs/<run>/record.json)` else error naming `-c`. Then `submit.SubmitAndPoll(...)`, print `public_score` to stdout; non-scored → non-zero exit + a stderr note (mirrors the step's failed-run semantics). Poll tuning via the same `KAGGLE_SUBMIT_MAX_ATTEMPTS`/`KAGGLE_SUBMIT_DELAY` envs (from the shared `internal/submit` helpers).
  - **Injected into:** n/a (top-level). Consumes `submit.SubmitAndPoll` + `slugFromRecordJSON`.

- **cmd/kaggle-submit `run()`** — refactored: keep `stepio` with.json reading + `writeSubmission`/`WriteMetrics` (step-specific), but replace the inline `cli.Submit`+`pollScore` block with `submit.SubmitAndPoll(cli, w.Competition.Slug, csvPath, w.Message, maxAttempts, delay)`. All existing `cmd/kaggle-submit` tests must still pass (the refactor is behavior-preserving).

### Test surface

- `internal/submit/submit_test.go` — the moved `TestPollScore_*` pure tests + a `SubmitAndPoll` test against a fake `Submitter` (or the real fake-kaggle via `kaggletest.WireFake`) asserting the scored path + the own-score-vs-prior-scored correlation.
- `cmd/kaggle/main_test.go` — hermetic `kaggle submit` against `cmd/fake-kaggle`: lay down `runs/<id>/submission/submission.csv` (+ a `record.json` with `steps[].with.competition.slug` for the auto-slug case), `WireFake`, `KAGGLE_FAKE_SCORE_AFTER=1` + `KAGGLE_SUBMIT_MAX_ATTEMPTS=5`/`DELAY=0`, run `submit`, assert the printed `public_score` (0.775) — the Done-when. Plus: `-c` override, `-f` override, and slug-missing → error. `slugFromRecordJSON` gets a pure unit test.
- `cmd/kaggle-submit/main_test.go` — unchanged, must stay green (proves the refactor is behavior-preserving).

---

## Task 1: Extract `internal/submit` (pollScore + env helpers), refactor the step

**Files:**
- Create: `internal/submit/submit.go`, `internal/submit/env.go`, `internal/submit/submit_test.go`
- Modify: `cmd/kaggle-submit/main.go` (remove `pollScore`/`envInt`/`envDuration`; call `submit.SubmitAndPoll`), `cmd/kaggle-submit/main_test.go` (move `TestPollScore_*` out to `internal/submit`)

- [ ] **Step 1: Move `pollScore` + `TestPollScore_*` into `internal/submit`.** Copy `pollScore` verbatim into `internal/submit/submit.go` (package `submit`), exported name kept lowercase only if internal-tested — but the CLI doesn't call `pollScore` directly (it calls `SubmitAndPoll`), so keep `pollScore` unexported in `internal/submit` and test it in-package. Move the `TestPollScore_*` funcs (`cmd/kaggle-submit/main_test.go:16-111`) into `internal/submit/submit_test.go` (package `submit`), adjusting the referenced `kaggle.Submission`/`ParseSubmissions` imports. Move `envInt`/`envDuration` into `internal/submit/env.go` (exported: `EnvInt`/`EnvDuration`, since both `package main`s call them).

- [ ] **Step 2: Add `Submitter` + `SubmitAndPoll` to `internal/submit/submit.go`:**

```go
package submit

import (
	"path/filepath"
	"time"

	"github.com/xianxu/kaggle/pkg/kaggle"
)

// Submitter is the kagglecli seam SubmitAndPoll needs (kagglecli.CLI satisfies it).
type Submitter interface {
	Submit(slug, file, msg string) error
	Submissions(slug string) (string, error)
}

// SubmitAndPoll uploads csvPath to slug, then blocking-polls for THIS upload's
// public score (newest-submission-correlated — see pollScore). One path shared by
// the kaggle/submit step and the `kaggle submit` CLI. Returns the submission
// (scored on success, newest-own/pending on timeout) + whether it scored.
func SubmitAndPoll(cli Submitter, slug, csvPath, message string, maxAttempts int, delay time.Duration) (kaggle.Submission, bool, error) {
	if err := cli.Submit(slug, csvPath, message); err != nil {
		return kaggle.Submission{}, false, err
	}
	return pollScore(
		func() (string, error) { return cli.Submissions(slug) },
		filepath.Base(csvPath),
		maxAttempts,
		func(int) { time.Sleep(delay) },
	)
}
```

- [ ] **Step 3: Run the moved tests** — `go test ./internal/submit/ -v` → the `TestPollScore_*` pass (RED first if you delete the impl to confirm they bind).

- [ ] **Step 4: Refactor `cmd/kaggle-submit/main.go`'s `run()`** — replace the block **`main.go:62-78`** (the `cli := kagglecli.New()` at line **62** through the `pollScore(...)` call — `cli` is used only inside this block, so line 62 MUST be deleted too or it orphans as `cli declared and not used`) with:

```go
maxAttempts := submit.EnvInt("KAGGLE_SUBMIT_MAX_ATTEMPTS", 30)
delay := submit.EnvDuration("KAGGLE_SUBMIT_DELAY", 5*time.Second)
sub, scored, err := submit.SubmitAndPoll(kagglecli.New(), w.Competition.Slug, csvPath, w.Message, maxAttempts, delay)
if err != nil {
	return err
}
sub.Competition = w.Competition.Slug
```

Delete the now-moved `pollScore`/`envInt`/`envDuration` from `cmd/kaggle-submit/main.go`. **Import-cleanup checklist** (grep each for a remaining user — the moved code was the sole user of two): drop **`"path/filepath"`** (only use was `filepath.Base(csvPath)` at line 71, which moves into `SubmitAndPoll`) and **`"strconv"`** (only used by the moved `envInt`/`envDuration`). **Keep `"time"`** (still referenced via `5*time.Second`). Add `"github.com/xianxu/kaggle/internal/submit"`. Run `go build ./cmd/kaggle-submit/` to confirm no orphaned imports/locals.

- [ ] **Step 5: Verify the step is behavior-preserving** — `go test ./cmd/kaggle-submit/ ./internal/submit/ -v` → all green (the step's `TestRun_*` integration tests unchanged and passing). Then `go build ./... && go vet ./...`.

- [ ] **Step 6: Commit** — `#5: extract internal/submit (pollScore + SubmitAndPoll); step reuses it`.

---

## Task 2: `cmd/kaggle` binary with the `submit` subcommand

**Files:**
- Create: `cmd/kaggle/main.go`, `cmd/kaggle/runref.go`, `cmd/kaggle/main_test.go`, `cmd/kaggle/runref_test.go`

- [ ] **Step 1: `slugFromRecordJSON` (pure) + test.** In `cmd/kaggle/runref.go`:

```go
package main

import "encoding/json"

// slugFromRecordJSON extracts the competition slug from a run's record.json:
// the first steps[].with.competition.slug (a kaggle/download or /submit step
// carries it). Local minimal struct — kaggle imports no metis package.
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
```

`runref_test.go`: assert extraction from a record.json byte fixture (a `download` step with `with.competition.slug`), and `("",false)` for no-kaggle-step / malformed JSON.

- [ ] **Step 2: `cmd/kaggle/main.go` — dispatch + `submit`.** RED: write `main_test.go` first (Step 3), then implement:

```go
// Command kaggle is the thin user-facing Kaggle CLI (kaggle#5): ad-hoc verbs over
// the same kagglecli/submit path the kaggle/* steps use. First verb: submit.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/xianxu/kaggle/internal/kagglecli"
	"github.com/xianxu/kaggle/internal/submit"
)

const usage = "usage: kaggle submit [--run <id> | -f <file>] [-c <slug>] [-m <msg>]"

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "kaggle:", err)
		os.Exit(1)
	}
}

// run threads stdout (ARCH-PURE — tests assert on a bytes.Buffer, mirroring
// cmd/fake-kaggle's run(args, stdout) rather than swapping os.Stdout).
func run(args []string, stdout io.Writer) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		fmt.Fprintln(stdout, usage)
		if len(args) == 0 {
			return fmt.Errorf("no subcommand")
		}
		return nil
	}
	switch args[0] {
	case "submit":
		return cmdSubmit(args[1:], stdout)
	default:
		return fmt.Errorf("unknown subcommand %q (want: submit)", args[0])
	}
}

func cmdSubmit(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("submit", flag.ContinueOnError)
	runID := fs.String("run", "", "run id → runs/<id>/submission/submission.csv")
	file := fs.String("f", "", "explicit submission.csv path (overrides --run)")
	slug := fs.String("c", "", "competition slug (overrides the run record)")
	msg := fs.String("m", "", "submission message")
	if err := fs.Parse(args); err != nil {
		return err
	}

	csvPath := *file
	if csvPath == "" {
		if *runID == "" {
			return fmt.Errorf("need --run <id> or -f <file>")
		}
		csvPath = filepath.Join("runs", *runID, "submission", "submission.csv")
	}
	if _, err := os.Stat(csvPath); err != nil {
		return fmt.Errorf("submission csv %s: %w", csvPath, err)
	}

	comp := *slug
	if comp == "" && *runID != "" {
		if b, err := os.ReadFile(filepath.Join("runs", *runID, "record.json")); err == nil {
			if s, ok := slugFromRecordJSON(b); ok {
				comp = s
			}
		}
	}
	if comp == "" {
		return fmt.Errorf("competition slug not found in the run record; pass -c <slug>")
	}

	maxAttempts := submit.EnvInt("KAGGLE_SUBMIT_MAX_ATTEMPTS", 30)
	delay := submit.EnvDuration("KAGGLE_SUBMIT_DELAY", 5*time.Second)
	sub, scored, err := submit.SubmitAndPoll(kagglecli.New(), comp, csvPath, *msg, maxAttempts, delay)
	if err != nil {
		return err
	}
	if !scored {
		return fmt.Errorf("%q not scored after %d attempts (status=%s)", comp, maxAttempts, sub.Status)
	}
	fmt.Fprintf(stdout, "public_score: %g\n", *sub.PublicScore)
	return nil
}
```

(`main()` dispatch is `kaggle <verb>` — a flat grammar, so `switch args[0]`; this is unlike `cmd/fake-kaggle/main.go:52` which switches on `args[1]` because its first token is the `competitions` group prefix.)

- [ ] **Step 3: Hermetic `main_test.go` (the Done-when).** Mirror the step's integration test but drive the CLI's `run()`:

```go
func TestSubmit_RunResolvesAndScores(t *testing.T) {
	kaggletest.WireFake(t, t.TempDir()) // MUST precede t.Chdir — it go-builds the fake
	t.Setenv("KAGGLE_FAKE_SCORE_AFTER", "1")
	t.Setenv("KAGGLE_SUBMIT_MAX_ATTEMPTS", "5")
	t.Setenv("KAGGLE_SUBMIT_DELAY", "0")

	ws := t.TempDir()
	writeFile(t, filepath.Join(ws, "runs", "winner", "submission", "submission.csv"), "PassengerId,Survived\n892,0\n")
	writeFile(t, filepath.Join(ws, "runs", "winner", "record.json"),
		`{"steps":[{"with":{"competition":{"slug":"titanic"}}}]}`)
	t.Chdir(ws) // Go 1.26: run from the workspace so runs/<id>/... resolves (auto-restored)

	var out bytes.Buffer
	if err := run([]string{"submit", "--run", "winner"}, &out); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(out.String(), "public_score: 0.775") {
		t.Fatalf("want public_score 0.775 in output; got %q", out.String())
	}
}
```

Plus `-c` override (no record.json needed), `-f` override (explicit path, no `runs/` layout), and slug-missing → error. **Only helper needed:** `writeFile(t, path, content)` = `os.MkdirAll(filepath.Dir(path), 0o755)` then `os.WriteFile` (must create the parent dir). No stdout-swap hack — `run` takes an `io.Writer`, so assert on a `bytes.Buffer`. Order matters: **`WireFake` before `t.Chdir`** (WireFake `go build`s the fake, which fails from a go.mod-less temp cwd).

- [ ] **Step 4: Run** — `go test ./cmd/kaggle/ -v` → PASS; `go build ./... && go vet ./...` clean; confirm `bin/kaggle` builds (`go build -o bin/kaggle ./cmd/kaggle`).

- [ ] **Step 5: Commit** — `#5: kaggle submit CLI — resolve run → submission.csv + slug, submit, return public_score`.

---

## Task 3: Full-suite verification, atlas, close

- [ ] **Step 1: Full suite** — `go test ./...` all green (step tests unchanged, new submit + CLI tests pass).
- [ ] **Step 2: Manual smoke against the fake** — build `bin/kaggle` + `bin/fake-kaggle`; from a temp workspace with `runs/winner/submission/submission.csv` + `record.json`, run `KAGGLE_CLI=<fake> KAGGLE_FAKE_STATE=… bin/kaggle submit --run winner` → prints `public_score: 0.775`. Record in `## Log`.
- [ ] **Step 3: Atlas** — add a `kaggle submit` CLI entry (the thin ad-hoc verb; one submit/auth path shared with the `kaggle/submit` step via `internal/submit`). Keep `atlas/index.md` linking it.
- [ ] **Step 4: Close** — `sdlc close --issue 5 --verified '<fake-run output + go test ./... pass>'`; `--actual` measured (or `--no-actual` with reason if contaminated).

---

## Decisions

- **Extract to `internal/submit`, not `internal/kagglecli`.** `kagglecli` is the thin subprocess wrapper (no business logic); the poll loop + submit orchestration is a distinct concern. Separating keeps `kagglecli` pure-wrapper and the poll correctness in one testable place (ARCH-PURE).
- **Zero metis dependency — parse `record.json` locally.** Consistent with `internal/stepio`'s "declare metis-contract strings locally (rule-of-two)" posture. A `metis/pkg/record` import would be a new cross-module coupling for one nested field; a 6-line local struct is thinner. `-c` is the always-works override.
- **Print the score; don't mutate the run record.** "returns the public_score" = print to stdout (parseable). Writing back into metis's `record.json` would couple layers + fight #13 immutability. Recording is an "and/or" in the issue — deferred as a thin follow-up if wanted.
- **New `cmd/kaggle` binary (not a flag on `kaggle-submit`).** The layering model names a thin user-facing `kaggle` CLI (submit now, competition-lookup later); `cmd/kaggle-submit` is a step entrypoint (reads `with.json`/METIS_* env), a different contract. Dispatch is ready for more verbs.
