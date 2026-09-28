---
id: '000005'
status: done
started: 2026-07-06T23:13:17-07:00
created: 2026-07-06
updated: 2026-07-07
estimate_hours: 1.35
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
