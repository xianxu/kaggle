# Boundary Review — kaggle#1 (milestone M2)

| field | value |
|-------|-------|
| issue | 1 — kaggle platform integration: Competition/Submission/Leaderboard + download/submit step-types wrapping the official CLI |
| repo | kaggle |
| issue file | workshop/issues/000001-kaggle-platform-integration-competition-submission-leaderboard-download-submit-step-types-wrapping-the-official-cli.md |
| boundary | milestone M2 |
| milestone | M2 |
| window | 4df2b36ddf7e621e432dbefbc2281694fa526cff..HEAD |
| command | sdlc milestone-close --issue 1 --milestone M2 |
| reviewer | claude |
| timestamp | 2026-07-02T09:37:36-07:00 |
| verdict | FIX-THEN-SHIP |

## Review

[0m[31mIgnoring 6 permissions.allow entries from .claude/settings.json: this workspace has not been trusted. Run Claude Code interactively here once and accept the trust dialog, or set projects["/Users/xianxu/workspace/kaggle"].hasTrustDialogAccepted: true in /Users/xianxu/.claude.json.[0m
All tests green, `go vet` clean, and I've verified the metis-side contract facts the plan leans on. Here's my review.

```verdict
verdict: FIX-THEN-SHIP
confidence: high
```

M2 delivers exactly what the Plan's Chunk 2 committed: `internal/stepio` (the Go step-side metis-contract reader, Decision A2 local consts), the `kaggle/download` + `kaggle/submit` step-types, committed `steps/kaggle/*` go-run wrappers, shared `internal/kaggletest` helpers, and a hermetic e2e that drives the **real** metis binary against the process-level fake. I independently confirmed the contract the code hardcodes — `METIS_STEP_DIR/RUN_DIR/STEP_ID/EXP_DIR/SEED` (metis `cmd/metis/exec.go:60-64`), `run.json` status `"ok"`/`"failed"` (metis `pkg/experiment/run.go:65,71`), the flat metrics merge, and `metis run --run <id>` + `METIS_STEP_PATH` (metis `cmd/metis/main.go:38,58-60`) — all match. Architecture is sound: `pollScore` is genuinely pure (injected clock, unit-tested with a scripted `subFn` and no-op sleep), IO sits at the boundary, and the drift-guard chain is real (stepio requires the vars from env; real metis emits them). Nothing blocks the boundary — the verified (fake + e2e) path is fully green and every Plan item landed. The findings below are non-blocking, the chief one being a robustness gap on the explicitly-deferred live path that the first live consumer (kbench#1) will hit.

### 1. Strengths
- **Drift-guard is genuine, not a self-echo** (`internal/stepio/stepio.go:46-71` + `e2e/e2e_test.go`): `New()` requires the three consumed vars from env with no cwd fallback, and the e2e runs the actual metis binary — a rename in metis structurally turns the run RED. `TestNew_RequiresVars` encodes the local half. This directly resolves the M1-review open item.
- **`pollScore` ARCH-PURE seam** (`cmd/kaggle-submit/main.go`): the fragile async-poll loop is a pure function with an injected `sleep`, unit-tested for both the pending→scored iteration and sleep-only-between-attempts (`slept==1` for 2 attempts) — zero wall-clock in tests. Exactly the right place to put the risk.
- **No nil-deref on the score path**: `*sub.PublicScore` is only dereferenced when `scored==true`, which `LatestScored` guarantees (`Scored()` ⇒ non-nil). Timeout path returns before any deref.
- **ARCH-DRY consolidation** (`internal/kaggletest/kaggletest.go`): `BuildBin`/`WireStep`/`WireFake` replace the duplicated `buildFake` in `integration_test.go` and unify the metis/fake build path across step, integration, and e2e tests.
- **Honest scope accounting**: atlas + plan Revisions clearly separate the fake-verified path from the code-complete-but-not-live-verified path, naming the single unvalidated schema point (`testdata/submissions.csv`).

### 2. Critical findings
None.

### 3. Important findings
- **`kaggle/submit` doesn't fast-fail on a terminal `error` submission status** (`cmd/kaggle-submit/main.go`, `pollScore` — the `LatestScored`/`last = subs[0]` region, ~line 113-120). A submission Kaggle rejects (bad format) gets `Status="error"` permanently with no score. `LatestScored` skips it (nil score), so the loop polls the *entire* budget (default 30 × 5s = 150s) before exiting non-zero, and the message misattributes it (`"not scored after 30 attempts (status=error)"` when it was terminal after attempt 1). The plan's "timeout contract" only modeled pending→scored→exhaustion and overlooked the terminal-error case; `StatusError` exists in the model but nothing consumes it. Fix sketch: in `pollScore`, after parsing, if the newest row's `Status == kaggle.StatusError`, return it immediately (`scored=false`) so `run()` writes the errored record and exits with a distinct terminal message. **Non-blocking at this gate** (on the deferred live path; the fake never emits `error`; the final verdict — a failed run — is still correct), but cheap and worth doing before kbench#1's first live run.

### 4. Minor findings
- `envInt`/`envDuration` (`cmd/kaggle-submit/main.go:131-149`) silently fall back to the default on a malformed value (`KAGGLE_SUBMIT_DELAY=5x` → 5s, no warning) — a misconfiguration hides as default behavior.
- `internal/stepio/stepio_test.go:125` hand-rolls `contains()` instead of `strings.Contains`.
- `unzip`/`extractOne` (`cmd/kaggle-download/main.go`) ignore zip-entry file modes (`os.Create` default perms) — fine for CSV data, noting for completeness.
- `steps/kaggle/*` `go run` recompiles the entrypoint on every step invocation — a documented tradeoff mirroring metis's `uv run`; not hot-path.

### 5. Test coverage notes
- **Zip-slip guard is untested** (`cmd/kaggle-download/main.go:83-85`). The guard is correct (`filepath.Join` cleans + `HasPrefix(cleanDest+sep)` catches `../` escapes), but no test feeds a malicious entry. Input is trusted Kaggle data, so low risk — a one-line adversarial-zip unit test would pin it.
- **Untested error paths**: submit's missing-upstream-artifact (`os.Stat` fail), download's "produced no .zip" branch, and the error-status poll behavior (ties to the Important finding) have no coverage. Each is a small unit test.
- The core contracts (async iteration, timeout→pending+non-zero, loose-file download, scored round-trip, run.json ok + public_score) are all well covered, including the subprocess-integration and full-e2e levels.

### 6. Architectural notes for upcoming work
- **ARCH-DRY: PASS.** `kaggletest` is the shared build/wire seam; `FormatSubmissionsCSV`↔`ParseSubmissions` remain the single schema; the `submission: <upstream-id>` → `submission.csv` convention reuses metis's `folds: split` pattern rather than inventing a path scheme. The `submissionFile` const lives once in submit; the bash stub restating it is a test fixture standing in for kbench's real step, not production duplication.
- **ARCH-PURE: PASS.** Pure poll/parse core, thin IO shell (`stepio`, `unzip`, step `main`s). No business logic buried in IO.
- **ARCH-PURPOSE: PASS (with the note above).** Shadow-sweep of the single-source claim (Decision A2): consumers of the METIS_* contract are the metis runner (source) and kaggle's `stepio` (derived by value, e2e-guarded); promotion to `metis/pkg/stepcontract` is a documented rule-of-three deferral, not a hand-maintained restatement left as the point of the issue. Done-when #3 (kbench live thread) is a genuinely separable cross-repo consumer, honestly deferred — not the deferred point of *this* issue. The one place the live path under-serves the purpose is the errored-submission fast-fail (Important finding).
- For M2→kbench handoff: the `submission.csv` filename coupling between `submit` and its upstream producer is the contract kbench's submission step must honor; it's captured in the atlas `with`-contract table.

### 7. Plan revision recommendations
- None required for table-vs-code fidelity — the Core-concepts entries (`stepio`, both step-types) exist at their stated paths with the stated PURE/INTEGRATION character, and the two documented deviations (no cross-repo go.mod; wrappers not a build system) are already recorded in the 2026-07-02 Revisions.
- Optional: add a one-line note to the Task-3 "Timeout contract" bullet acknowledging the **terminal `error` status** case (currently unmodeled) so the fast-fail fix has a home in the plan rather than surfacing only here.
