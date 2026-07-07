# Boundary Review — kaggle#5 (whole-issue close)

| field | value |
|-------|-------|
| issue | 5 — kaggle submit CLI — a thin command to submit a run's submission.csv + return the public_score (ad-hoc, no pipeline edit) |
| repo | kaggle |
| issue file | workshop/issues/000005-kaggle-submit-cli-a-thin-command-to-submit-a-run-s-submission-csv-return-the-public-score-ad-hoc-no-pipeline-edit.md |
| boundary | whole-issue close |
| milestone | — |
| window | 9ff12bec7b626fd895139631ff3ea499b530ce96..HEAD |
| command | sdlc close --issue 5 |
| reviewer | claude |
| timestamp | 2026-07-06T23:49:14-07:00 |
| verdict | FIX-THEN-SHIP |

## Review

Ignoring 6 permissions.allow entries from .claude/settings.json: this workspace has not been trusted. Run Claude Code interactively here once and accept the trust dialog, or set projects["/Users/xianxu/workspace/kaggle"].hasTrustDialogAccepted: true in /Users/xianxu/.claude.json.
```verdict
verdict: FIX-THEN-SHIP
confidence: high
```

The boundary delivers the issue's purpose cleanly: the submit→poll→score core was genuinely **extracted** to `internal/submit` (not copied), and **both** the `kaggle/submit` step and the new `cmd/kaggle` CLI consume it — the step's `TestRun_*` integration tests are unchanged and green, proving the refactor is behavior-preserving. `go build ./...`, `go vet ./...`, and `go test ./...` all pass. The Done-when (`kaggle submit --run winner` → real `public_score`, no `-c`, no pipeline edit) is proven hermetically, and I verified the auto-slug path works against the *real* metis `RunRecord` shape (`steps[].with.competition.slug`), so the headline flow isn't a fake-only mirage. Nothing blocks SHIP; the findings below are cheap, non-blocking cleanups.

### 1. Strengths
- **ARCH-DRY fully honored.** `pollScore`/`SubmitAndPoll`/`EnvInt`/`EnvDuration` live once in `internal/submit`; the step (`cmd/kaggle-submit/main.go:64-66`) and CLI (`cmd/kaggle/main.go:81-83`) both delegate. The subtle newest-submission-correlation (`subs[0]`/`wantFile`, not `LatestScored`) now exists in exactly one place with its regression test moved alongside it (`internal/submit/submit_test.go:57`).
- **ARCH-PURE clean.** `pollScore` takes injected `submissionsFn` + `sleep`; the real `time.Sleep` is wrapped only in the thin `SubmitAndPoll` glue (`submit.go:37`). `slugFromRecordJSON` is pure. `run(args, stdout io.Writer)` threads the writer so tests assert on a `bytes.Buffer` — no `os.Stdout` swap.
- **Zero-metis-dependency preserved.** The 6-line local `slugFromRecordJSON` struct matches the real record format without importing `metis/pkg/record`, consistent with `internal/stepio`'s declare-locally posture, and `-c` is the always-works override.
- **Nil-deref safe:** `*sub.PublicScore` is dereferenced only after the `if !scored` guard, and `scored=true` is returned only when `PublicScore != nil`.
- **Atlas updated** for both new surfaces (`internal/submit`, `cmd/kaggle`) in the same range; the `bin/kaggle` "auto-built by `cmd/*` scan" claim checks out against `Makefile.workflow:739`.

### 2. Critical findings
None.

### 3. Important findings
- **`internal/submit/env.go` — `EnvInt`/`EnvDuration` are now exported shared API with zero unit tests.** They carry non-trivial branches (empty→default, malformed→warn-to-stderr+default, and `EnvDuration`'s bare-integer→seconds path at `env.go:37`) consumed by two callers, yet no test exercises them. Fix: add a small table test in `internal/submit` covering the seconds-fallback and malformed-warn paths — cheap, and it pins the one genuinely subtle branch (`"5"` → `5s`). Non-blocking (moved verbatim, behavior-preserving), but it's exactly the newly-introduced-package-surface the gate asks to stabilize.

### 4. Minor findings
- `cmd/kaggle/main.go:31` — top-level `--help`/`-h`/`help` prints usage cleanly, but `kaggle submit --help` reaches `fs.Parse` and returns `flag.ErrHelp`, so it exits 1 with `kaggle: flag: help requested` on stderr instead of clean usage + exit 0. Minor UX wart.
- `cmd/kaggle/main.go:88` — the CLI's timeout error says `not scored after %d attempts (status=%s)` even for a terminal `status=error` rejection, which actually fast-fails on attempt 1 (the "after N attempts" wording is slightly off). The step distinguishes this case (`cmd/kaggle-submit/main.go:78`, "submission rejected"); the CLI collapses it. Cosmetic error-message inconsistency, not a correctness issue.
- `usage` string mixes `--run` with `-f/-c/-m` (double vs single dash); Go's `flag` treats them identically, so purely stylistic.

### 5. Test coverage notes
- Strong where it counts: `pollScore` (4 cases incl. prior-scored regression + terminal-error fast-fail), `SubmitAndPoll` (fake `Submitter`), `slugFromRecordJSON` (6 cases incl. malformed/empty-slug), CLI submit (run-resolve, `-c`, `-f`, slug-missing, needs-run-or-file, help/unknown), and the step's own integration tests retained. The `-f` arbitrary-basename correlation is exercised (`TestSubmit_FFlagExplicitPath`) and the fake records `filepath.Base` (`fake-kaggle/main.go:141`), matching real Kaggle's reported filename.
- Only gap: the `EnvInt`/`EnvDuration` helpers (see Important). No behavioral regressions are un-covered.

### 6. Architectural notes for upcoming work
- ARCH-PURPOSE: "print and/or record the score" — the print half is delivered; write-back into `record.json` is legitimately deferred (metis `#13` immutability). If a future issue picks up recording, it should go through a metis-owned mutation path, not have the kaggle CLI write metis's record — keep the zero-dep, immutable-record posture intact.
- `Submitter` is a well-scoped 2-method seam; if a future verb needs `Download`, extend via a separate interface rather than widening this one, to keep the poll path's dependency minimal.

### 7. Plan revision recommendations
None — the plan's Core-concepts table matches the code exactly (every listed entity exists at its stated path with the stated status: `pollScore`/`SubmitAndPoll`/`Submitter`/`EnvInt`/`EnvDuration` in `internal/submit`, `slugFromRecordJSON` in `cmd/kaggle/runref.go`; step `run()` modified as described). No `## Revisions` entry needed.

---

## Re-review — 2026-07-06T23:55:38-07:00 (FIX-THEN-SHIP)

| field | value |
|-------|-------|
| issue | 5 — kaggle submit CLI — a thin command to submit a run's submission.csv + return the public_score (ad-hoc, no pipeline edit) |
| repo | kaggle |
| issue file | workshop/issues/000005-kaggle-submit-cli-a-thin-command-to-submit-a-run-s-submission-csv-return-the-public-score-ad-hoc-no-pipeline-edit.md |
| boundary | whole-issue close |
| milestone | — |
| window | 9ff12bec7b626fd895139631ff3ea499b530ce96..HEAD |
| command | sdlc close --issue 5 |
| reviewer | claude |
| timestamp | 2026-07-06T23:55:38-07:00 |
| verdict | FIX-THEN-SHIP |

## Review

Ignoring 6 permissions.allow entries from .claude/settings.json: this workspace has not been trusted. Run Claude Code interactively here once and accept the trust dialog, or set projects["/Users/xianxu/workspace/kaggle"].hasTrustDialogAccepted: true in /Users/xianxu/.claude.json.
I have everything I need. Confirmed: `bin/kaggle` is auto-built by the generic `cmd/*/` scanner (`Makefile.workflow:739`), the metis run-record shape (`steps[].with` map, `record.json` at `runDir/record.json`, step outputs at `runDir/<step.ID>/`) matches `slugFromRecordJSON` and the `runs/<id>/submission/submission.csv` path, and `*sub.PublicScore` is nil-safe. One concrete finding: `internal/submit/env_test.go` is not gofmt-clean.

```verdict
verdict: FIX-THEN-SHIP
confidence: high
```

This boundary delivers the issue's purpose cleanly and honestly. The submit→poll→score core was **genuinely extracted** (not copied) into `internal/submit`, and I verified **both** consumers derive from it — the step (`cmd/kaggle-submit/main.go:64-66`) and the new CLI (`cmd/kaggle/main.go:86-88`), with the old `pollScore`/`envInt`/`envDuration` fully deleted from the step's `package main` (no orphaned duplicate remains). `go build ./...`, `go vet ./...`, and `go test ./...` all pass; the step's `TestRun_*` integration tests are unchanged and green, proving the refactor is behavior-preserving. I independently confirmed against the *real* metis `RunRecord` (`metis/pkg/record/record.go:56-81`) that the auto-slug path (`steps[].with.competition.slug`) and the on-disk paths (`runDir/record.json`, `runDir/<step.ID>/`) are real, not a fake-only mirage — so the headline "`kaggle submit --run winner`, no `-c`" flow is sound. The prior close-review's one Important (untested `EnvInt`/`EnvDuration`) is now addressed by `internal/submit/env_test.go`, and its Minor `submit --help` wart is fixed (`main.go:48,54-57`). Nothing blocks SHIP; the sole actionable fix is a gofmt nit, everything else is optional polish.

### 1. Strengths
- **ARCH-DRY fully honored — no residual duplicate.** The subtle newest-submission correlation (`subs[0]`/`wantFile`, *not* `LatestScored`) now lives in exactly one place (`internal/submit/submit.go:55`) with its 4-case regression test moved alongside (`internal/submit/submit_test.go`). I confirmed the step file no longer contains any copy.
- **ARCH-PURE clean.** `pollScore` takes injected `submissionsFn` + `sleep`; real `time.Sleep` is wrapped only in the thin `SubmitAndPoll` glue (`submit.go:37`). `slugFromRecordJSON` is pure and injected into a thin caller. `run(args, stdout io.Writer)` threads the writer so tests assert on a `bytes.Buffer` — no `os.Stdout` swap.
- **Zero-metis-dependency preserved and correct.** The 6-line local `slugFromRecordJSON` struct matches metis's `StepRecord.With map[string]any` / `RunRecord.Steps` shape exactly without importing `metis/pkg/record` — consistent with `internal/stepio`'s declare-locally posture, and `-c` is the always-works override.
- **Nil-deref safe.** `Scored()` is `PublicScore != nil` (`pkg/kaggle/submission.go:25`), and `pollScore` returns `scored=true` only via `newest.Scored()`, so `*sub.PublicScore` (`main.go:95`) can never deref nil.
- **Atlas updated** for both new surfaces (`internal/submit`, `cmd/kaggle`) in the same window; the `bin/kaggle` "auto-built by the `cmd/*` scan" claim checks out against `Makefile.workflow:739`.

### 2. Critical findings
None.

### 3. Important findings
None. (The prior review's Important — exported `EnvInt`/`EnvDuration` untested — is resolved by `internal/submit/env_test.go`, which covers the empty→default, malformed→warn, and bare-integer→seconds branches.)

### 4. Minor findings
- **`internal/submit/env_test.go` is not gofmt-clean** — the `EnvDuration` table's trailing comments are misaligned; `gofmt -d` shows a diff (every other file in the diff is clean). Fix: `gofmt -w internal/submit/env_test.go`. Mechanical and certain — worth doing before crossing the boundary since it's the one hygiene regression in the tree.
- `cmd/kaggle/main.go:93` — the `!scored` error reads `not scored after %d attempts (status=%s)` even for a terminal `status=error` rejection, which actually fast-fails on attempt 1 (the "after N attempts" wording is inaccurate for that case). The step distinguishes it (`cmd/kaggle-submit/main.go:78-79`, "submission rejected"); the CLI collapses it. Cosmetic error-message inconsistency, not a correctness issue.
- `cmd/kaggle/main.go:83` — the slug-missing error says `not found in the run record; pass -c` even in the pure `-f` case where no run record was consulted; the `pass -c` half is still actionable, but the "in the run record" clause is misleading there.
- `cmd/kaggle/main.go:95` — `%g` can render scientific notation for extreme scores (e.g. a large RMSE → `1.23e+06`). Display-only; the step side persists the canonical float to `metrics.json`, so this is purely the human-facing print.

### 5. Test coverage notes
- Strong where it counts: `pollScore` (4 cases incl. prior-scored regression + terminal-error fast-fail), `SubmitAndPoll` (fake `Submitter`), `slugFromRecordJSON` (6 cases incl. malformed/empty-slug), CLI submit (run-resolve, `-c`, `-f`, slug-missing, needs-run-or-file, help/unknown), `EnvInt`/`EnvDuration`, and the step's integration tests retained. The `-f` arbitrary-basename correlation is exercised end-to-end through the real fake (`TestSubmit_FFlagExplicitPath` → fake stores `filepath.Base`, `SubmitAndPoll` matches it).
- Untested-but-trivial passthroughs (not blocking): `SubmitAndPoll`'s `cli.Submit`-returns-error branch (`submit.go:30`) and the CLI's `!scored` timeout-error branch (`main.go:92-93`) — both one-line passthroughs whose underlying logic (`pollScore` timeout) is covered in `internal/submit`.

### 6. Architectural notes for upcoming work
- **`-f` correlation vs real Kaggle (live-verify backlog).** The poll correlates on `filepath.Base(csvPath) == subs[0].File`. The step always submits a fixed `submission.csv`, but the CLI's `-f` widens exposure to arbitrary basenames — if real Kaggle's `submissions --csv` echoes a filename that differs from the local basename, the poll would exhaust and report a false timeout despite a successful score. This is the documented fake/real blind spot (`cmd/fake-kaggle/main.go:9-13`), acceptable under the M2 live-Kaggle-deferred posture (kbench#1), but it's the first thing to pin on the first live capture.
- **ARCH-PURPOSE:** "print and/or record the score" — print is delivered; write-back into `record.json` is legitimately deferred (metis `#13` immutability). If a future issue picks up recording, route it through a metis-owned mutation path rather than having the kaggle CLI write metis's record — keep the zero-dep, immutable-record posture intact.
- `Submitter` is a well-scoped 2-method seam; if a future verb needs `Download`, extend via a separate interface rather than widening this one, to keep the poll path's dependency minimal.

### 7. Plan revision recommendations
None. The plan's Core-concepts table matches the code exactly: `pollScore`/`SubmitAndPoll`/`Submitter`/`EnvInt`/`EnvDuration` in `internal/submit`, `slugFromRecordJSON` in `cmd/kaggle/runref.go`, step `run()` modified as described — every entity exists at its stated path with the stated status. No `## Revisions` entry needed.

---

## Re-review — 2026-07-07T00:00:00-07:00 (SHIP)

| field | value |
|-------|-------|
| issue | 5 — kaggle submit CLI — a thin command to submit a run's submission.csv + return the public_score (ad-hoc, no pipeline edit) |
| repo | kaggle |
| issue file | workshop/issues/000005-kaggle-submit-cli-a-thin-command-to-submit-a-run-s-submission-csv-return-the-public-score-ad-hoc-no-pipeline-edit.md |
| boundary | whole-issue close |
| milestone | — |
| window | 9ff12bec7b626fd895139631ff3ea499b530ce96..HEAD |
| command | sdlc close --issue 5 |
| reviewer | claude |
| timestamp | 2026-07-07T00:00:00-07:00 |
| verdict | SHIP |

## Review

Ignoring 6 permissions.allow entries from .claude/settings.json: this workspace has not been trusted. Run Claude Code interactively here once and accept the trust dialog, or set projects["/Users/xianxu/workspace/kaggle"].hasTrustDialogAccepted: true in /Users/xianxu/.claude.json.
I have everything I need. Independently verified: build/gofmt/vet clean, all tests green, the extraction is genuine (no leftover `pollScore`/`envInt`/`envDuration` in the step), `kagglecli.CLI` structurally satisfies `Submitter`, `Scored()` guarantees nil-safe deref, the metis `RunRecord.Steps[].With` shape matches `slugFromRecordJSON` exactly (headline `--run` auto-slug is real, not fake-only), and the prior close-review's findings (untested Env helpers, `submit --help` wart, gofmt on env_test) are all resolved in the window's later commits.

```verdict
verdict: SHIP
confidence: high
```

This whole-issue boundary delivers kaggle#5's purpose cleanly and honestly. The submit→poll→score core was **genuinely extracted** into `internal/submit` (the step's copy of `pollScore`/`envInt`/`envDuration` is fully deleted — `grep` finds no residue in `cmd/kaggle-submit`), and **both** consumers derive from it: the step (`cmd/kaggle-submit/main.go:64-66`) and the new CLI (`cmd/kaggle/main.go:86-88`). `go build ./...`, `go vet`, `gofmt -l`, and `go test ./...` are all clean/green; the step's `TestRun_*` integration tests are unchanged, proving the refactor is behavior-preserving. I confirmed the Done-when flow (`kaggle submit --run winner` → real `public_score`, no `-c`, no pipeline edit) against the *real* metis `RunRecord` shape, so it isn't a fake-only mirage. The two prior FIX-THEN-SHIP passes recorded in the close-review artifact were addressed by commits `a74030b`/`3b11729` (env tests added, `submit --help` fixed, gofmt cleaned) — nothing Important or Critical survives at HEAD. The remaining items are cosmetic.

### 1. Strengths
- **ARCH-DRY fully honored, no residual duplicate.** The subtle newest-submission correlation (`subs[0]`/`wantFile`, *not* `kaggle.LatestScored`) lives in exactly one place (`internal/submit/submit.go:55`) with its 4-case regression test (`submit_test.go`). Verified the step no longer contains any copy.
- **ARCH-PURE clean.** `pollScore` takes injected `submissionsFn` + `sleep`; real `time.Sleep` is wrapped only in the thin `SubmitAndPoll` glue (`submit.go:37`). `slugFromRecordJSON` is pure; `run(args, stdout io.Writer)` threads the writer so tests assert on a `bytes.Buffer` — no `os.Stdout` swap.
- **Zero-metis-dependency preserved and correct.** The 6-line local `slugFromRecordJSON` struct matches metis's `StepRecord.With map[string]any` / `RunRecord.Steps` exactly (verified against `../metis/pkg/record/record.go`) without importing `metis/pkg/record`, and `-c` is the always-works override.
- **Nil-deref safe.** `Scored()` is `PublicScore != nil` (`pkg/kaggle/submission.go:25`); `pollScore` returns `scored=true` only via `newest.Scored()`, so `*sub.PublicScore` (`main.go:97`) can never deref nil.
- **`Submitter` is a well-scoped 2-method seam** that `kagglecli.CLI` satisfies structurally — the poll path never touches `os/exec`.
- **Atlas updated** for both new surfaces (`internal/submit`, `cmd/kaggle`); the `bin/kaggle` auto-build claim checks out.

### 2. Critical findings
None.

### 3. Important findings
None. (Prior review's Important — untested exported `EnvInt`/`EnvDuration` — is resolved by `internal/submit/env_test.go`, covering empty→default, malformed→warn, and the bare-integer→seconds branch.)

### 4. Minor findings
- `cmd/kaggle/main.go:97` — `%g` renders scientific notation for extreme scores (a large-RMSE competition → `1.23e+06`). Display-only and the CLI has no `metrics.json` fallback, so it's the sole human-facing surface, but still parseable/correct. Consider `strconv.FormatFloat(f, 'f', -1, 64)` if RMSE-style scores ever matter. Cosmetic.
- `cmd/kaggle/main.go:95` — the `!scored` message collapses timeout vs terminal `status=error`; the `status=%s` clause disambiguates and `polled up to %d attempts` is now phrased as a budget (accurate), so this is mild — the step still gives a dedicated "submission rejected" message (`cmd/kaggle-submit/main.go:79`) the CLI doesn't. Optional parity nit.
- `usage` string mixes `--run` with `-f/-c/-m` (double vs single dash); Go's `flag` treats them identically — purely stylistic.

### 5. Test coverage notes
- Strong where it counts: `pollScore` (4 cases incl. prior-scored regression + terminal-error fast-fail), `SubmitAndPoll` (fake `Submitter`), `slugFromRecordJSON` (6 cases incl. malformed/empty-slug), CLI submit (run-resolve auto-slug, `-c`, `-f`, slug-missing, needs-run-or-file, help/unknown), `EnvInt`/`EnvDuration`, and the step's integration tests retained.
- Only un-covered branches are trivial one-line passthroughs: `SubmitAndPoll`'s `cli.Submit`-error return (`submit.go:30`) and the CLI's `!scored` timeout→error branch (`main.go:92-95`) — the underlying `pollScore` timeout is covered in `internal/submit`. Not blocking.

### 6. Architectural notes for upcoming work
- **`-f` correlation vs real Kaggle (live-verify backlog).** The poll correlates on `filepath.Base(csvPath) == subs[0].File`. The step always submits a fixed `submission.csv`, but the CLI's `-f` widens exposure to arbitrary basenames. The fake stores `filepath.Base(*file)` (`cmd/fake-kaggle/main.go:141`) so tests are self-consistent, but this is the documented SHARED BLIND SPOT (`fake-kaggle/main.go:10-13`): if real Kaggle's `submissions --csv` echoes a different filename, the poll would falsely time out despite a successful score. Acceptable under the M2 live-Kaggle-deferred posture (kbench#1) — first thing to pin on the first live capture.
- **ARCH-PURPOSE:** "print and/or record the score" — print is delivered; write-back into `record.json` is legitimately deferred (metis #13 immutability, and the Spec is an explicit "and/or"). If a future issue picks up recording, route it through a metis-owned mutation path rather than the kaggle CLI writing metis's record — keep the zero-dep, immutable-record posture intact.
- If a future verb needs `Download`, extend `Submitter` via a separate interface rather than widening it, to keep the poll path's dependency minimal.

### 7. Plan revision recommendations
None. The plan's Core-concepts table matches the code exactly — `pollScore`/`SubmitAndPoll`/`Submitter`/`EnvInt`/`EnvDuration` in `internal/submit`, `slugFromRecordJSON` in `cmd/kaggle/runref.go`, step `run()` modified as described; every entity exists at its stated path with the stated status. No `## Revisions` entry needed.
