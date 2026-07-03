# Boundary Review — kaggle#3 (whole-issue close)

| field | value |
|-------|-------|
| issue | 3 — kaggle CLI wrapper: delegate auth to the CLI (support access_token + OAuth), drop the stale credential precheck |
| repo | kaggle |
| issue file | workshop/issues/000003-delegate-auth-to-cli.md |
| boundary | whole-issue close |
| milestone | — |
| window | d4a1796d84ea88698c6f1de3e625bd914e65e1c8..HEAD |
| command | sdlc close --issue 3 |
| reviewer | claude |
| timestamp | 2026-07-02T20:34:13-07:00 |
| verdict | FIX-THEN-SHIP |

## Review

Ignoring 6 permissions.allow entries from .claude/settings.json: this workspace has not been trusted. Run Claude Code interactively here once and accept the trust dialog, or set projects["/Users/xianxu/workspace/kaggle"].hasTrustDialogAccepted: true in /Users/xianxu/.claude.json.
All verifications complete. Build, vet, and the full test suite are green; no source references to the deleted symbols remain; the atlas is updated; there is no in-repo README to update.

```verdict
verdict: FIX-THEN-SHIP
confidence: high
```

This is a clean, well-executed deletion that removes a genuine ARCH-DRY violation: the stale `checkCredentials` mirror of the wrapped CLI's evolving auth rules is gone, and auth is fully delegated to the CLI, whose own error now surfaces through `wrap()`. The purpose is fully delivered — all five `KAGGLE_FAKE=1` sites and the dead `CredentialSource`/`ErrNoCredentials` pair are removed, a replacement propagation test is added, and `go build/vet/test ./...` are green. The one thing keeping this from a clean SHIP is doc-drift in a load-bearing self-improvement log (`workshop/lessons.md`), which now recommends the exact constructs this issue deleted — non-blocking but worth fixing before the boundary since AGENTS.md §4 has every agent read it at session start.

**1. Strengths**
- **ARCH-DRY delivered, not just claimed** (`internal/kagglecli/cli.go:6-14`): the root cause — a partial mirror of an external tool's auth that had already drifted once — is eliminated at the source rather than patched by adding `access_token` support. The rewritten package doc names the four CLI auth methods and the exact failure mode avoided.
- **Missing-cred path stays covered** (`cli_test.go:62-79`): `TestCLIError_Propagates` is a real replacement for the deleted precheck — a non-zero-exit stub with stderr, asserting the CLI's own actionable message carries through `wrap()`. This is the Done-when's required safety net, and it pins real behavior (stderr propagation), not a mock.
- **Complete shadow-sweep**: grep confirms zero source refs to `checkCredentials`/`CredentialSource`/`ErrNoCredentials`/bare `KAGGLE_FAKE`; the fake genuinely never read the bare signal (it keys off `KAGGLE_FAKE_STATE`/`_DATA_DIR`), so removing it is behavior-neutral.
- **Honest verification boundary** (issue Log:92): the implementor explicitly records that green tests prove the precheck is gone and errors propagate, but do *not* prove live `access_token` auth — correctly deferred to the operator live-run since no CLI/creds/network exist here.
- **atlas updated in-range** (`atlas/kaggle-layer.md:17`, table:26-31): all three flagged spots rewritten to "auth delegated to the wrapped CLI"; the `KAGGLE_FAKE=1` env row removed.

**2. Critical findings**
None.

**3. Important findings**
- **`workshop/lessons.md:11-12` now recommends the removed constructs.** Line 12 tells future agents to "Gate on an explicit signal (`KAGGLE_FAKE=1`)" — a signal this diff deletes — and line 11 prescribes keeping "a pure decision fn (`credentialSource`) in `pkg/kaggle`", the function this diff deletes as dead code. Per AGENTS.md §4 this file is read at every session start, so the stale guidance will actively mislead. *Fix sketch:* annotate both bullets as superseded by kaggle#3 (the precheck was removed entirely; the CLI owns auth), and — as §4 asks after a review — add the new lesson this issue teaches: *"Don't mirror an external tool's evolving rules (e.g. its auth methods); delegate and surface the tool's own error. A local mirror drifts and false-negatives valid setups — ARCH-DRY."* Cheap, and it converts a contradiction into the correct captured insight.

**4. Minor findings**
- `Submissions` (`cli.go:50-56`) inlines its own `exec.Command(...).Output()` + `wrap()` rather than sharing `run`'s path; the new error-propagation test exercises only `Download` (→ `run` → `wrap`). The two paths share `wrap`, so risk is low, but `Submissions`' error branch is untested. (The exec/wrap duplication itself is pre-existing, not introduced here — a `runOut` helper could unify them if this area is touched again.)

**5. Test coverage notes**
- The deleted `TestCheckCredentials` (which needed `t.Setenv`/temp-HOME — IO) is correctly replaced by a process-level stub test, keeping the missing-cred contract covered without the IO-in-a-"pure"-test smell.
- Full suite green including `e2e`, `internal/kagglecli` (incl. the new test), and `pkg/kaggle`. Coverage gap noted above (`Submissions` error branch) is minor.

**6. Architectural notes for upcoming work**
- **ARCH-DRY — pass (strongly).** This issue *is* an ARCH-DRY correction; no new duplication introduced.
- **ARCH-PURE — pass.** `internal/kagglecli` remains a thin IO seam that only shells out; no business logic buried in it. The prior pure/IO split (pure `CredentialSource` fed by IO) is obviated — there is no auth *decision* left to keep pure, so the removal is consistent with the principle, not a violation of it.
- **ARCH-PURPOSE — pass.** Shadow-sweep confirms every consumer of the removed surface derives correctly (all deleted); the deferred "live auth" proof is genuinely unverifiable in this repo and is honestly flagged rather than silently claimed. No easy-subset shortcut.

**7. Plan revision recommendations**
None — the Plan is fully `[x]` and matches the shipped code; no `## Revisions` entry needed. (The lessons.md fix above is a separate doc edit, not a plan contradiction.)
