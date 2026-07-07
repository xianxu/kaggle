---
id: 000005
status: codecomplete
deps: []
github_issue:
created: 2026-07-06
updated: 2026-07-06
estimate_hours: 1.35
started: 2026-07-06T23:13:17-07:00
actual_hours: 0.60
---

# kaggle submit CLI — a thin command to submit a run's submission.csv + return the public_score (ad-hoc, no pipeline edit)

## Problem

Submitting a run's output to Kaggle is currently awkward. The `kaggle/submit` **step** exists (used
inside a pipeline, e.g. the titanic-baseline thread), but for the **ad-hoc** case — "I ran an offline
sweep (no submit step), promoted a winner, now submit that ONE run's `submission.csv` and tell me the
score" — the operator must either drop to the raw `kaggle competitions submit` CLI (bypasses the
workbench + doesn't record the score) or hand-edit the winner experiment to add a `submit` step and
re-run (clunky). Submit is a **Kaggle concern** (metis stays domain-agnostic), so it belongs as a
thin kaggle-layer CLI, not a metis/run verb.

## Spec

- A thin `kaggle` CLI command — e.g. **`kaggle submit --run <run-id>`** (resolving
  `<...>/runs/<run-id>/submission/submission.csv`), or `kaggle submit -c <slug> -f <file>` — that:
  - reads the competition slug from the run's record/experiment (or `-c`),
  - submits the `submission.csv` via the same official-`kaggle`-CLI path the `kaggle/submit` step
    uses (`internal/kagglecli` — reuse it, don't re-implement auth), and
  - **polls for and returns the `public_score`** (the step already does this blocking-poll), printing
    it and/or recording it (e.g. into the run's record).
- Thin wrapper around the real `kaggle` CLI + the existing submit-step logic — no new auth,
  no new submission mechanism. Reuses `KAGGLE_API_TOKEN` / `~/.kaggle/access_token` / legacy
  `kaggle.json` (the `kagglecli` resolution order).

## Done when

- `kaggle submit --run winner` submits `runs/winner/submission/submission.csv` and returns the
  real `public_score` — no pipeline edit, no raw-CLI hunting.
- Reuses `internal/kagglecli` (shared with the `kaggle/submit` step) — one submit/auth path.
- Hermetic test against the fake-kaggle (like the submit step's test).

## Estimate

*Produced via `brain/data/life/42shots/velocity/estimate-logic-v3.1.md` against `baseline-v3.1.md`. Method A only.*

```estimate
model: estimate-logic-v3.1
familiarity: 1.0
item: smaller-go-module      design=0.15  impl=0.3
item: smaller-go-module      design=0.15  impl=0.35
item: atlas-docs             design=0.05  impl=0.1
item: milestone-review       design=0.0   impl=0.2
design-buffer: 0.15
total: 1.35
```

Two small modules: (1) extract `internal/submit` (pollScore+SubmitAndPoll) + refactor the
step to reuse it; (2) new thin `cmd/kaggle` binary with the `submit` subcommand + slug/file
resolution + hermetic fake-kaggle test. Auth/submit seam (`internal/kagglecli`) is reused as-is.

## Plan

Durable plan: `workshop/plans/000005-kaggle-submit-cli-plan.md` (reviewed).
Single-boundary (plain checkboxes, one `sdlc close`).

- [x] T1: extract `internal/submit` (`pollScore`+`SubmitAndPoll`+`EnvInt`/`EnvDuration`); refactor the `kaggle/submit` step to consume it (shared helper, not a copy).
- [x] T2: new `cmd/kaggle` binary — `submit` subcommand: resolve `--run`→submission.csv, slug from `-c`/`record.json`, `SubmitAndPoll`, print `public_score`; hermetic fake-kaggle test.
- [x] T3: full-suite verify + manual fake smoke + atlas.

## Log

### 2026-07-06
- 2026-07-06: closed — Re-close after addressing the FIX-THEN-SHIP review. Fixed the Important finding: added internal/submit/env_test.go covering EnvInt/EnvDuration (empty→default, valid, malformed-warn, Go-duration, bare-int→seconds). Fixed a Minor: kaggle submit --help now prints clean usage + exit 0. go test ./... all green. No behavior change to the shipped submit/CLI path — added test + a help-UX fix only.; review verdict: FIX-THEN-SHIP
- 2026-07-06: closed — Thin kaggle submit CLI reusing the SAME submit+poll+auth path as the kaggle/submit step. Extracted internal/submit (SubmitAndPoll+pollScore+Env helpers) from cmd/kaggle-submit package main; step refactored to consume it (shared helper, not a copy) — its TestRun_* integration tests unchanged + green (behavior-preserving). New cmd/kaggle: submit --run resolves runs/<id>/submission/submission.csv + slug from record.json (local parse, no metis import) or -c/-f. PROOF: built-binary smoke `KAGGLE_CLI=<fake> bin/kaggle submit --run winner` → public_score: 0.775 exit 0 (no -c, no pipeline edit); hermetic cmd/kaggle tests (--run auto-slug, -c, -f, slug-missing, help) + go test ./... all green. Both change-code judges INFO. actual 0.60 clean single-issue window.; review verdict: FIX-THEN-SHIP
- Filed from the layering discussion (operator): submit is a Kaggle concern (a step + a thin CLI),
  not a run verb. Closes the awkward offline-sweep → promote-winner → submit-that-one-file flow
  (metis-v1 kbench#4's operator step) without editing the experiment.

### 2026-07-07 — implemented (durable plan `workshop/plans/000005-*`, both change-code judges INFO)
- **Shared helper, not a copy:** the submit→poll→score core (`pollScore` + the submit orchestration
  + `KAGGLE_SUBMIT_*` env helpers) was unexported in `cmd/kaggle-submit`'s `package main`. Extracted
  to **`internal/submit`** (`SubmitAndPoll` over a `Submitter` seam, `pollScore`, `EnvInt`/`EnvDuration`);
  the `kaggle/submit` STEP refactored to consume it — its `TestRun_*` integration tests unchanged +
  green (behavior-preserving). One submit/auth path; `pollScore`'s newest-correlated poll (not
  `LatestScored`) lives once.
- **New thin `cmd/kaggle` CLI:** `kaggle submit [--run <id> | -f <file>] [-c <slug>] [-m <msg>]`.
  `--run` → `runs/<id>/submission/submission.csv`; slug from `-c` else best-effort
  `slugFromRecordJSON(record.json)` (local minimal parse — **no metis import**, zero-dep posture).
  Threads `io.Writer` (ARCH-PURE); prints `public_score`. `--help`/`-h`/`help` → usage.
- **Zero-metis-dependency kept.** Reads the run record's slug with a 6-line local struct rather than
  importing `metis/pkg/record` (consistent with `internal/stepio`'s declare-locally posture); `-c`
  is the always-works override.
- **Done-when PROVEN** — hermetic `cmd/kaggle` tests (`--run` auto-slug, `-c`, `-f`, slug-missing
  error, help) all green + a **built-binary smoke**: from a temp workspace with
  `runs/winner/{submission/submission.csv, record.json}`, `KAGGLE_CLI=<fake> … bin/kaggle submit
  --run winner` → `public_score: 0.775`, exit 0 (no `-c`, no pipeline edit). `go test ./...` all green.
- **Score not written back** into metis's `record.json` (keeps #13 immutability); recording is a
  deferred thin follow-up if wanted (the issue's "print and/or record" — print is the deliverable).
