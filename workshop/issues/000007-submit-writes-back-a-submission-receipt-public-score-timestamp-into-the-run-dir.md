---
id: 000007
status: open
deps: []
github_issue:
created: 2026-07-14
updated: 2026-07-14
estimate_hours:
---

# submit writes back a submission receipt — public_score + timestamp into the run dir

## Problem

`kaggle submit --run <id>` prints `public_score: 0.78229` to the terminal and forgets it. Whether
a run was EVER submitted, when, and what it scored lives nowhere machine-readable — today it
survives only in chat logs and hand-written issue Logs (metis#35/#41 honest-beat sessions: three
public scores, all recorded manually). The run dir is the natural home: content-addressed,
joinable back to the ledger row (via kaggle#6's description the chain goes ledger→Kaggle;
this closes the loop Kaggle→run).

## Spec

- On a successful submit (score polled), write `runs/<id>/submission/receipt.json`:
  `{submitted_at, competition, public_score, description, message}` — append-style (a JSON array
  or one file per submission `receipt-<n>.json`) since a run CAN be re-submitted.
- Print unchanged; the receipt is a side effect of the same poll that prints the score.
- Downstream (not this issue): `metis select --point`/board rendering could surface "submitted:
  public 0.78229" next to the honest estimate — the operator's LB-vs-estimate comparison
  (RUNBOOK §5) becomes greppable. Also feeds metis#36's ticket-estimand experiment (public
  samples per config accumulate in-tree).

## Done when

- A live (or fake-kaggle) submit leaves a receipt.json with the polled score + timestamp; a
  re-submit appends rather than overwrites.
- Unit/e2e via the existing fake-kaggle (process-level fake) — no live creds in tests.

## Plan

- [ ] Small: serialize the already-polled result next to the submission.csv it scored.

## Log

### 2026-07-14
- Filed by operator right after the point-rf-3daa6310 submit (public 0.78229) — the third
  hand-recorded score of the day. Sibling: kaggle#6 (auto-description = ledger→Kaggle);
  this is Kaggle→run.
