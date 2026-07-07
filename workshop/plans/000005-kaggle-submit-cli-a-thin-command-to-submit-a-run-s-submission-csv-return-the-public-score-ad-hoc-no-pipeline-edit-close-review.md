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
