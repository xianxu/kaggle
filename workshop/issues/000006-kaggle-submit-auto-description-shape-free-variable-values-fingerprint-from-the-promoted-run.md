---
id: 000006
status: open
deps: []
github_issue:
created: 2026-07-14
updated: 2026-07-14
estimate_hours:
---

# kaggle submit auto-description — shape + free-variable values + fingerprint from the promoted run

## Problem

`kaggle submit --run <id>` uploads with whatever description the operator types (or none). Last
session's submission description was hand-written ("rf max_depth=4 n_est=500 + ticket_survival —
honest generalizer (inner-CV 0.8395, cx 14.3)…"); today's (metis#35 honest-beat, public 0.77751)
had none. The factual half of that provenance already exists in the promoted run's `record.json`
(shape name, resolved free-variable values, code fingerprint) — Kaggle's submission list is the
one place provenance currently evaporates.

## Spec

- `kaggle submit --run <id>` auto-composes the submission description from the run record:
  `<shape> · <free-variable assignments> · fp <fingerprint-short> · run <id-short>` — e.g.
  `titanic-sweep · features=[title,family,age] rf md=4 n=500 · fp b7aee3de · best-rf-d3c532f8`.
- `-m "<note>"` APPENDS the operator's hypothesis note (never replaces the factual part) — the
  human writes the why, the machine writes the what.
- Respect Kaggle's description length limit (truncate free-vars first, never the fingerprint/run
  id — those are the join keys back to the ledger).
- Fields read via the existing record.json contract (shared `internal/submit`, kaggle#5) — no new
  metis surface.

## Done when

- A `--run` submit with no `-m` carries the auto-description (visible on the Kaggle submissions
  page / API response); with `-m` both parts appear.
- Unit test over a fixture record.json → exact description string (incl. the truncation rule).

## Plan

- [ ] Small: record.json → description composer (pure, tested) + wire into the submit call.

## Log

### 2026-07-14
- Filed by operator during the metis#35 honest-beat wrap-up. Siblings from the same session (in
  metis): #39 fingerprint visibility, #40 /metis-select skill, #41 select --point. The submission
  description is the LB-side end of the same provenance chain #39 surfaces on the ledger side.
