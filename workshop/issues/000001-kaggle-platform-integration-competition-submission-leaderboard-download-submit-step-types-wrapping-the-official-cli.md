---
id: 000001
status: working
deps: [metis#1]
github_issue:
created: 2026-07-01
updated: 2026-07-01
estimate_hours:
started: 2026-07-01T22:27:10-07:00
---

# kaggle platform integration: Competition/Submission/Leaderboard + download/submit step-types wrapping the official CLI

## Problem

kaggle is an empty scaffold atop metis. The `kaggle-ml-base-layer` project needs the Kaggle **platform-integration** layer: typed records for the state of Kaggle interaction (Competition / Submission / Leaderboard + credentials) and the `kaggle/download` + `kaggle/submit` **step-types** that the metis step-runner invokes. "Platform-specific" test: *does it touch the Kaggle API/CLI?* — if yes it lives here, not in metis.

## Spec

Design from the 2026-07-01 brainstorm. **Scope Go to the STATE, ride the official client for the TRANSPORT.**

- Model the typed **state** in Go (the durable, git-legible records): **Competition** (id, metric, deadline, data manifest — thin), **Submission** (file + score + status + timestamp), **Leaderboard** (public-score snapshot — thin), **Credentials** (kaggle auth via `~/.kaggle/kaggle.json` or env).
- Ride the **official `kaggle` CLI** for the bytes-over-wire (auth, dataset download, submission upload) rather than reimplementing multipart upload + auth in Go. Later, swap internals for a Go REST client *behind the same Go type surface* (out of scope now; consumers don't change).
- **Step-types contributed to the metis runner** (via `uses: kaggle/download` / `kaggle/submit`): `download` (auth + pull competition data into the gitignored data dir — the *download half* of an Adapter), `submit` (submit `submission.csv`, return a typed Submission + status), and read the public leaderboard score.
- **Process-level fake** — model the Kaggle CLI/API with a process-level fake for the e2e test (per the "model external services" rule); function-call mocks miss interaction bugs, and CI must not depend on live Kaggle.

## Done when

- `kaggle/download` and `kaggle/submit` run under `metis run`, authenticating + pulling Titanic data and submitting a file, returning a typed Submission + public score.
- A **process-level fake** Kaggle service backs the e2e test (no live Kaggle needed in CI).
- Exercised end-to-end by kbench#1's Titanic thread (data downloaded + submission submitted + score read).

## Plan

- [ ] M1 — Competition/Submission/Leaderboard + credentials; `kaggle/download` + `kaggle/submit` step-types wrapping the official CLI; process-level fake + e2e

## Log

### 2026-07-01

Created from the `kaggle-ml-base-layer` project brainstorm (brain `data/project/kaggle-ml-base-layer.md`). Depends on metis#1 (the step-runner + step-type contract + Dataset envelope). Layer: `kbench → kaggle → metis → ariadne`.
