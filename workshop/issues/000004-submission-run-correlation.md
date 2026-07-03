---
id: 000004
status: open
deps: []
github_issue:
created: 2026-07-02
updated: 2026-07-02
estimate_hours:
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

## Spec

Make a submission traceable back to the run that produced it, and let the operator
retrieve prior submissions out-of-band.

- **Run-id in the message (the correlation key we own).** Have `kaggle/submit`
  automatically stamp the run identity into the submit message: send
  `"<base-message>@<run-id>"` (e.g. `titanic-baseline@run-live`). The run-id is
  derivable in the step with **no metis change** — it's `basename(METIS_RUN_DIR)`;
  add it once as `stepio.Context.RunID` (`internal/stepio`) so any kaggle step can
  use it. This is competition- and experiment-agnostic (every experiment benefits;
  no per-experiment `with` burden). `ARCH-DRY`.
- **A fetch command (out-of-band retrieval).** Add `cmd/kaggle-submissions` (a thin
  main over `CLI.Submissions` + `ParseSubmissions`) that lists a competition's
  submissions as a table (date, description, status, public/private score). This is
  the re-fetch path when the in-run poll times out, and the operator's way to read
  scores for prior submissions. (An operator-facing `bin/` wrapper — sibling of
  kbench's `bin/krun` — can follow; the command itself is the kaggle deliverable.)
- **Capture Kaggle's submission ref IF present (hard correlation).** The authored
  parser has no ID column because the fixture was authored from docs. **Empirical
  gate:** run `kaggle competitions submissions -c titanic --csv` against the real
  2.x CLI and inspect the header. If it exposes a submission id/ref column, add it
  to `Submission` + the parser (header-driven, so additive) so correlation can key
  off a stable ID instead of the description convention. If it does NOT, the
  message-stamp convention is the correlation mechanism and this item is dropped.
  (Same live capture should refresh the fixture's status vocab — the real value is
  `SubmissionStatus.COMPLETE`, not the authored `complete`; see kaggle#3 close notes
  / atlas honesty caveat.)

## Done when

- `kaggle/submit` sends `"<message>@<run-id>"`; a live (or fake) submission's
  `description` in the list carries the run-id, so two runs of the same experiment
  are distinguishable. `stepio.Context.RunID` exists + is unit-tested.
- `cmd/kaggle-submissions <competition>` prints the parsed submission list; covered
  by a test against `fake-kaggle` (process-level, per the deterministic-shell rule).
- The real `submissions --csv` header has been captured; either the submission
  ref/id is parsed into `Submission` (with a test) **or** the issue records that the
  CLI exposes no id and the message convention stands. Fixture status vocab
  reconciled to the real value.
- `go build ./... && go test ./...` green.

## Plan

- [ ] `internal/stepio`: add `Context.RunID` = `basename(METIS_RUN_DIR)`; unit test.
- [ ] `cmd/kaggle-submit`: append `@<RunID>` to the submit message (guard empty run-id / empty base message); update `main_test.go`.
- [ ] `cmd/kaggle-submissions`: new thin command over `CLI.Submissions` + `ParseSubmissions`; table output; fake-driven test.
- [ ] Empirical: capture real `kaggle competitions submissions -c titanic --csv` header. If an id/ref column exists → add to `Submission` + parser (+ test) + refresh the fixture; else record "no id, convention stands". Reconcile fixture status vocab (`SubmissionStatus.COMPLETE`).
- [ ] atlas/kaggle-layer.md: document `Context.RunID`, the message-stamp convention, `cmd/kaggle-submissions`; clear the fixture honesty caveat once reconciled.
- [ ] `go build ./... && go test ./...` green.

## Log

### 2026-07-02
- Filed from the post-live-run design discussion (kbench#1 walked, public_score 0.76794). Scoped to **kaggle** (not kbench): the submit step + parser + fetch command are kaggle-owned, and run-id is derivable from `METIS_RUN_DIR` in-step, so no kbench/metis change is required (metis could later add a `METIS_RUN_ID` env as a nicety, but `basename(RUN_DIR)` suffices).
- Depends empirically on the real `submissions --csv` schema (id column? status vocab?) — the "capture Kaggle's ref" item is conditional on that capture; the run-id-in-message + fetch-command core does not depend on it.
- Correlation heuristic today (main.go:98) is safe **within** a single synchronous run (newest-first + filename); this issue makes it durable **across** runs.
