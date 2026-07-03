---
id: 000003
status: codecomplete
deps: []
github_issue:
created: 2026-07-02
updated: 2026-07-02
estimate_hours: 0.52
started: 2026-07-02T20:22:04-07:00
actual_hours: 0.17
---

# kaggle CLI wrapper: delegate auth to the CLI (support access_token + OAuth), drop the stale credential precheck

## Problem

Surfaced during the kbench Titanic **operator live-run** (kbench#1). The operator
installed the current Kaggle CLI (2.2.3) and authenticated the modern way —
`~/.kaggle/access_token` (the new default token file). `bin/krun` failed at the
`get-data` step:

```
kaggle/download: kaggle: no credentials — set KAGGLE_USERNAME + KAGGLE_KEY, or install ~/.kaggle/kaggle.json
```

That error is **ours**, not the CLI's. `internal/kagglecli.checkCredentials` runs a
pre-flight guard *before* shelling to the CLI, and it only recognizes two **legacy**
mechanisms: the `KAGGLE_USERNAME`+`KAGGLE_KEY` env pair, or `~/.kaggle/kaggle.json`.
The new CLI supports **four** auth methods (per its docs): OAuth (`kaggle auth login`),
`KAGGLE_API_TOKEN` env var, `~/.kaggle/access_token`, and legacy `kaggle.json`. Our
guard false-negatives a valid `access_token` setup (and would also block an OAuth
login, which leaves no file and no env var), blocking the real CLI before it can run.

Root cause: the guard is a **stale, partial mirror** of the wrapped CLI's own auth
logic (DRY violation against an external source of truth). It has already drifted
once (kaggle.json era → access_token era) and will drift again. We *wrap* the CLI;
the CLI is the single source of truth for its own auth, and already emits a clear
error on missing creds — which our `wrap()` surfaces via stderr. `ARCH-DRY`.

## Spec

Delete the pre-flight credential mirror; let the wrapped CLI own the auth decision.
This fixes `access_token`, OAuth, and `KAGGLE_API_TOKEN` in one stroke, and every
future CLI auth method, with no ongoing maintenance. `KAGGLE_FAKE=1` existed *only*
to skip this guard (the fake binary itself never reads it — it keys off
`KAGGLE_FAKE_STATE`/`KAGGLE_FAKE_DATA_DIR`), so it becomes vestigial and is removed
too. The pure `kaggle.CredentialSource` + `ErrNoCredentials` have no other consumer
and are deleted as dead code. `ARCH-DRY`, Simplicity First (the guard has no
one-sentence justification once the CLI owns auth).

## Done when

- `internal/kagglecli` no longer runs a credential precheck; Download/Submit/
  Submissions shell straight to the CLI, whose own auth error surfaces on failure.
- The pure `CredentialSource`/`ErrNoCredentials` and the vestigial `KAGGLE_FAKE=1`
  signal are gone (code + tests + atlas).
- `go build ./... && go test ./...` green.
- A missing-credential path is still covered: a test asserts the CLI's own auth
  failure propagates through the wrapper (via a stub that exits non-zero).
- atlas/kaggle-layer.md reflects "auth delegated to the wrapped CLI" (the 4 methods).

## Estimate

```estimate
model: estimate-logic-v3.1
familiarity: 1.0
design-buffer: 0.15
item: smaller-go-module      design=0.1 impl=0.30
item: atlas-docs             design=0.0 impl=0.10
total: 0.52
```

Sizing: net a **deletion** in a now-familiar module (fam 1.0) — remove the
precheck + dead pure fn + vestigial env signal across cli.go/2 test files/1
helper, delete 2 files, add one auth-failure-propagation test (`smaller-go-module`,
smaller than kaggle#2's 0.58 feature add since most is removal), plus the atlas
edit. Design is ~nil (diagnosis done in the claim window; single obvious approach).

## Plan

- [x] Remove `checkCredentials` + its 3 call sites + the `KAGGLE_FAKE=1` skip from `internal/kagglecli/cli.go`; fix imports + the package/auth doc comment.
- [x] Delete `pkg/kaggle/credentials.go` + `credentials_test.go` (dead: no other consumer).
- [x] Tests: drop `TestCheckCredentials`; remove `KAGGLE_FAKE=1` from **all 4** consumer sites — `cli_test.go` (:31, :65), `integration_test.go` (:23), `kaggletest.WireFake` (:56), and `e2e/e2e_test.go` (:56, keep the sibling `KAGGLE_FAKE_STATE`/`SCORE_AFTER`); add a test that a CLI auth failure propagates through the wrapper (non-zero-exit stub) — `TestCLIError_Propagates`.
- [x] atlas/kaggle-layer.md: rewrite **all 3** spots — the line-13 `CredentialSource` bullet, the line-18 "runs `checkCredentials()` first / skipped iff `KAGGLE_FAKE=1`" clause, and the line-30 `KAGGLE_FAKE=1` table row; state auth is delegated to the CLI (OAuth / access_token / KAGGLE_API_TOKEN / kaggle.json).
- [x] `go build ./... && go test ./...` green; the fix unblocks the live-run via the working-tree `go run`.

## Log

### 2026-07-02
- 2026-07-02: closed — Re-close to re-anchor HEAD for the v0 merge. Delta since the prior close = the FIX-THEN-SHIP fixes the close review itself requested: output() exec-path unification (Submissions now shares runs wrap) + TestCLIError_Propagates covering both Download and Submissions error branches + lessons.md supersession of the removed constructs; plus the new #4 issue file (docs). go vet ./... clean; go test ./... green (all packages). No new external surface.; review verdict: SHIP
- 2026-07-02: closed — go vet ./... clean; go test ./... all packages OK — fake-kaggle, kaggle-download, kaggle-submit, e2e, internal/kagglecli (incl. new TestCLIError_Propagates pinning that a CLI auth failure surfaces through the wrapper), pkg/kaggle. Grep confirms zero refs to removed checkCredentials/CredentialSource/ErrNoCredentials. Fake+e2e path verified. HONEST GAP (verify-before-claim): real ~/.kaggle/access_token auth against live Kaggle is NOT proven here — no CLI/creds/network in this env; that proof is the operator live-run (kbench#1), now unblocked since the download step go-runs this working tree. atlas/kaggle-layer.md updated (auth delegated to CLI).; review verdict: FIX-THEN-SHIP
- Diagnosed from the live-run failure: our precheck only knew legacy creds; new CLI 2.2.3 uses `~/.kaggle/access_token`. Chose delegate-to-CLI (remove) over broaden (add access_token) because broaden still false-negatives OAuth and keeps mirroring evolving external auth. `ARCH-DRY`.
- change-code plan-quality judge (fresh context) caught a real `ARCH-PURPOSE` shadow-sweep miss: `KAGGLE_FAKE=1` has **5** sites, and `e2e/e2e_test.go:56` was omitted from my first enumeration (tests stay green with the dead ref in place, so the "green" gate wouldn't catch it). Also flagged atlas:18. Folded both into the Plan/Done-when before implementing.
- Implemented as planned. `go vet ./...` clean; `go test ./...` **all packages ok** (fake-kaggle, kaggle-download, kaggle-submit, e2e, internal/kagglecli incl. new `TestCLIError_Propagates`, pkg/kaggle). Grep confirms zero refs to `checkCredentials`/`CredentialSource`/`ErrNoCredentials`; remaining `KAGGLE_FAKE` hits are the unrelated `KAGGLE_FAKE_PRIOR_SCORE`.
- **Honesty caveat (verify-before-claim):** build+full-suite green proves the wrapper no longer prechecks and that a CLI error propagates. It does **not** prove real `access_token` auth against live Kaggle — no CLI/network/creds here. That proof is the operator live-run (kbench#1), now unblocked (download step `go run`s this working tree).
- Also corrected `kbench/competition/titanic/README.md` step 2 to the modern CLI flow (access_token / `kaggle auth login`), since the operator was following the stale legacy-only instructions when this surfaced.
