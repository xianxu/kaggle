---
id: 000005
status: open
deps: []
github_issue:
created: 2026-07-06
updated: 2026-07-06
estimate_hours:
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

## Plan

- [ ] `kaggle submit` command: resolve the run's submission.csv + competition, submit via kagglecli, poll for public_score.
- [ ] Reuse the `kaggle/submit` step's submit+poll logic (shared helper, not a copy).
- [ ] Hermetic test (fake-kaggle) + a `--help` line.

## Log

### 2026-07-06
- Filed from the layering discussion (operator): submit is a Kaggle concern (a step + a thin CLI),
  not a run verb. Closes the awkward offline-sweep → promote-winner → submit-that-one-file flow
  (metis-v1 kbench#4's operator step) without editing the experiment.
