---
id: 000006
status: open
created: 2026-07-14
updated: 2026-07-14
estimate_hours:
github_issue:
---

# kaggle submit auto-description — shape + free-variable values + fingerprint from the promoted run

## Problem

`kaggle submit --run <id>` uploads with whatever description the operator types (or none). Last
session's submission description was hand-written ("rf max_depth=4 n_est=500 + ticket_survival —
honest generalizer (inner-CV 0.8395, cx 14.3)…"); today's (metis#35 honest-beat, public 0.77751)
had none. The factual half of that provenance already exists in the promoted run's `record.json`
(shape name, resolved free-variable values, code fingerprint) — Kaggle's submission list is the
one place provenance currently evaporates.
