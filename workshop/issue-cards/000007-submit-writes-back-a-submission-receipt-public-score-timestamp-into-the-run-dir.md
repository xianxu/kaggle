---
id: 000007
status: open
created: 2026-07-14
updated: 2026-07-14
estimate_hours:
github_issue:
---

# submit writes back a submission receipt — public_score + timestamp into the run dir

## Problem

`kaggle submit --run <id>` prints `public_score: 0.78229` to the terminal and forgets it. Whether
a run was EVER submitted, when, and what it scored lives nowhere machine-readable — today it
survives only in chat logs and hand-written issue Logs (metis#35/#41 honest-beat sessions: three
public scores, all recorded manually). The run dir is the natural home: content-addressed,
joinable back to the ledger row (via kaggle#6's description the chain goes ledger→Kaggle;
this closes the loop Kaggle→run).
