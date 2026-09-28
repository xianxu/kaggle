---
id: 000004
status: open
created: 2026-07-02
updated: 2026-07-02
estimate_hours:
github_issue:
---

# submission↔run correlation: run-id in submit message + a fetch command (+ capture Kaggle's submission ref)

## Problem

Surfaced by the operator after the first live Titanic submission (kbench#1, public
score 0.76794). Two related gaps once you make **more than one** submission to a
competition:

1. **No durable correlation key.** `kaggle/submit` (`cmd/kaggle-submit`) correlates
   the fetched score to *our* upload **positionally**: `pollScore` assumes Kaggle
   lists newest-first (`subs[0]`) and optionally matches on `File == "submission.csv"`
   (`cmd/kaggle-submit/main.go:98`). But `fileName` is **not unique** (every run
   uploads `submission.csv`), the parsed schema (`fileName,date,description,status,
   publicScore,privateScore`) has **no submission ID**, and the submit **message** —
   the one field we control — is set statically per-experiment (`message:
   "titanic-baseline"` in the experiment `with`), so **every run of an experiment
   collides**. Within a single synchronous run the positional heuristic is safe
   (documented at main.go:98), but you cannot look at a competition's submission
   list later and tell which row came from which run.

2. **No out-of-band fetch.** The score poll is synchronous and bounded
   (`KAGGLE_SUBMIT_MAX_ATTEMPTS`×`KAGGLE_SUBMIT_DELAY`, default 30×5s). If Kaggle's
   async scoring outlasts that budget the run FAILS (writes a `pending`
   submission.json, exits non-zero) — and there is **no command to re-fetch the
   score afterward**. The capability exists (`CLI.Submissions` → `ParseSubmissions`)
   but is only wired into the poll loop, not exposed to the operator.
