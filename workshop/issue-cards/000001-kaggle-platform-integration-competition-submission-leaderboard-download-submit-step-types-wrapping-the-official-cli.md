---
id: '000001'
status: done
started: 2026-07-01T22:27:10-07:00
created: 2026-07-01
updated: 2026-07-02
estimate_hours: 3.5
actual_hours: N/A
---

# kaggle platform integration: Competition/Submission/Leaderboard + download/submit step-types wrapping the official CLI

## Problem

kaggle is an empty scaffold atop metis. The `kaggle-ml-base-layer` project needs the Kaggle **platform-integration** layer: typed records for the state of Kaggle interaction (Competition / Submission / Leaderboard + credentials) and the `kaggle/download` + `kaggle/submit` **step-types** that the metis step-runner invokes. "Platform-specific" test: *does it touch the Kaggle API/CLI?* — if yes it lives here, not in metis.
