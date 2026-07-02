---
id: 000001
status: working
deps: [metis#1]
github_issue:
created: 2026-07-01
updated: 2026-07-01
estimate_hours: 3.5
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

- [x] M1 — kaggle library: `Competition`/`Submission` records + pure CLI-output parsers (`parseSubmissions`/`latestScored`) + pure `credentialSource`; `internal/kagglecli` injectable `${KAGGLE_CLI}` client; process-level fake `kaggle` (zip download + async pending→scored submit); client-vs-fake integration test
- [ ] M2 — integration: Go step-side contract reader (`internal/stepio`); `kaggle/download` + `kaggle/submit` step-types over the metis contract; e2e (`download → make-submission → submit`) under `metis run` against the fake; atlas + whole-issue close

Durable plan (per-file/per-test detail, Core concepts, open decisions): [`workshop/plans/000001-kaggle-platform-integration-plan.md`](../plans/000001-kaggle-platform-integration-plan.md). M1 detailed; M2 sketched (re-run `sdlc start-plan` before detailing it).

## Estimate

```estimate
model: estimate-logic-v3.1
familiarity: 1.0
item: greenfield-go-module   design=0.4 impl=0.4
item: api-integration        design=0.4 impl=0.6
item: real-api-discovery     design=0.2 impl=0.2
item: smaller-go-module      design=0.3 impl=0.4
item: milestone-review       design=0.0 impl=0.2
item: milestone-review       design=0.0 impl=0.2
design-buffer: 0.15
total: 3.5
```

Derivation (AI-paired ship-wall-clock, v3.1): **M1** = `greenfield-go-module` (pure `pkg/kaggle` records + parsers) + `api-integration` (the `kagglecli` CLI wrapper + process-level fake + async poll/retry) + `real-api-discovery` (budget to match the Kaggle CLI's `download`/`submit`/`submissions` surface + CSV shape from docs — live access deferred, so a small discovery cost, not a full one). **M2** = `smaller-go-module` (the two step-types + `internal/stepio`, extending the established metis step contract). Plus two `milestone-review` boundaries (M1, M2). recomputed = Σdesign(1.3)×1.15 + Σimpl(2.0) = 3.495 ≈ **3.5**. Provisional — calibration source flagged stale; sanity-checked against metis#1 (est 6 / actual 3.83, similar Go+fake+wiring surface).

## Log

### 2026-07-01

Created from the `kaggle-ml-base-layer` project brainstorm (brain `data/project/kaggle-ml-base-layer.md`). Depends on metis#1 (the step-runner + step-type contract + Dataset envelope). Layer: `kbench → kaggle → metis → ariadne`.

Claimed + planned (durable plan written, fresh-eyes reviewed). Design: **Go owns STATE, official CLI owns TRANSPORT**; pure `pkg/kaggle` (records + parsers), thin injectable `internal/kagglecli`, Go step-types honoring the metis contract, process-level fake for a hermetic e2e. Contract-fidelity check against `metis/cmd/metis/exec.go` came back clean.

**M1 shipped (pending boundary review + close).** The kaggle library: a **pure** `pkg/kaggle` (`ARCH-PURE`, table-tested, zero IO) — `Competition`/`Submission` records + the single CLI-text↔state boundary `ParseSubmissions`/`LatestScored`/`FormatSubmissionsCSV` (header-driven, order-independent) + the pure `CredentialSource` decision. A **thin IO seam** `internal/kagglecli` shelling an injectable `${KAGGLE_CLI:-kaggle}` (no parsing; auth decision deferred to the pure fn; precheck skipped only on explicit `KAGGLE_FAKE=1`, not a binary-name match). A **process-level fake** `cmd/fake-kaggle` that emits a real-shaped `.zip` on download and models the **async scoring transition** (`pending` for the first `KAGGLE_FAKE_SCORE_AFTER` polls, then `complete`+scored). `FormatSubmissionsCSV` gives fake+parser **one** schema (`ARCH-DRY`) — with the honest residual gap that neither is validated against real Kaggle (the fixture is *authored*, not captured; validate on first live run). Verification: `go test ./...` green; the `TestClientAgainstFake` integration test proves the submit→poll→scored flow **iterates through pending** (scored on poll #2). `Leaderboard` deferred (YAGNI). Delegated to a full-context implementation fork.

## Revisions

### 2026-07-01 — plan split M1→M1/M2 + review-driven design fixes
- **Split the single M1 into two review boundaries** (M1 = the kaggle library; M2 = the step-types + e2e integration) — two genuinely separate close points, matching the SDLC milestone convention.
- **`Leaderboard` deferred (YAGNI):** the walking skeleton reads the public score off `Submission.PublicScore` (via `parseSubmissions`); nothing consumes a full leaderboard snapshot, so building the struct now is a Simplicity-First smell. The Spec's leaderboard-*score* purpose is met. Deferred until a `kaggle/leaderboard` pull step needs it.
- **Fidelity caveat (honesty, not a claim):** this machine has no `kaggle` CLI and no credentials, so the *verified* deliverable is feature + faithful process-level fake with a green e2e; the **live-Kaggle path is code-complete but NOT live-verified** until run with credentials (operator, or kbench#1). The `submissions --csv` fixture is *authored* (Kaggle-CLI-docs provenance), not captured — validate its columns/status vocabulary on the first live run.
- Plan-review fixes folded in: `credentialSource` made pure (env/stat IO pushed to `kagglecli`); the fake models the async pending→scored *transition* (so the submit poll loop actually iterates) + emits a real-shaped `.zip`; e2e data-flow corrected (a `make-submission` producer step writes the `submission.csv` upstream artifact — a fixture is not an `UpstreamPath` artifact); submit poll-timeout contract pinned (exit non-zero on exhaustion). Step-contract constants: **Decision A2** (kaggle-local now, promote to `metis/pkg/stepcontract` when kbench is the 3rd Go consumer).
