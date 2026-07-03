# Boundary Review — kaggle#1 (whole-issue close)

| field | value |
|-------|-------|
| issue | 1 — kaggle platform integration: Competition/Submission/Leaderboard + download/submit step-types wrapping the official CLI |
| repo | kaggle |
| issue file | workshop/issues/000001-kaggle-platform-integration-competition-submission-leaderboard-download-submit-step-types-wrapping-the-official-cli.md |
| boundary | whole-issue close |
| milestone | — |
| window | bde5f00e583c5bc5c86a043d1a2c8c8d01562e3c..HEAD |
| command | sdlc close --issue 1 |
| reviewer | claude |
| timestamp | 2026-07-02T09:46:52-07:00 |
| verdict | FIX-THEN-SHIP |

## Review

[0m[31mIgnoring 6 permissions.allow entries from .claude/settings.json: this workspace has not been trusted. Run Claude Code interactively here once and accept the trust dialog, or set projects["/Users/xianxu/workspace/kaggle"].hasTrustDialogAccepted: true in /Users/xianxu/.claude.json.[0m
Confirmed. `LatestScored` (pkg/kaggle/parse.go:88) returns the first scored row scanning newest-first, with **no correlation to the submission just uploaded** — and `cmd/kaggle-submit/main.go:109` is its only production consumer. `parse_test.go:43-49` literally encodes the failure scenario: given `[newest=pending, submission_v2=scored 0.78229, …]`, `LatestScored` returns the *older* `submission_v2`. Right after `submit` uploads a new (pending) submission to a real competition that already has prior scored submissions, poll #1 would immediately return a **previous** submission's score. Let me finalize the review.

```verdict
verdict: FIX-THEN-SHIP
confidence: high
```

The whole-issue boundary (M1+M2) delivers its **verified** scope cleanly: `go test ./...` is fully green (including the e2e that drives the *real* built `metis` binary against the process-level fake — I ran it uncached: PASS in 1.39s), `go vet` is clean, the pure/IO seam is drawn correctly (ARCH-PURE real, not claimed), the fake+parser share one CSV schema (ARCH-DRY), and both prior milestone findings are genuinely fixed in the tree. Every Plan checklist item landed. What keeps this from a clean SHIP is one correctness bug on the explicitly-deferred **live** path that neither milestone review caught: `submit` selects the score to report via `LatestScored` (any scored row, newest-first), which is uncorrelated to the file it just uploaded — so against a real competition with prior scored submissions it will silently report the *wrong* submission's score. It is structurally unreachable by the fake (single submission per competition), so it does **not** block this fake-verified close, but it defeats `submit`'s stated purpose ("return a typed Submission + public score" for the file submitted) and must be fixed before kbench#1's first live run.

### 1. Strengths

- **The e2e is a genuine end-to-end proof, not a stub.** `e2e/e2e_test.go` builds and runs the actual `metis` binary from the sibling module and asserts `run.json.status==ok` + `public_score>0` + a scored `submission.json` + loose (non-zip) download data. I reran it uncached and watched metis execute all three steps. This is the strongest form of the Done-when evidence available without live Kaggle.
- **ARCH-PURE is real and tested at the seam.** `pollScore` (`cmd/kaggle-submit/main.go:99`) is a pure loop with an injected `sleep`, unit-tested for iteration, timeout, and terminal-error with zero wall-clock; `pkg/kaggle` functions take/return values and `credentials_test.go` deliberately avoids `t.Setenv`/temp-HOME. The IO (`os.Getenv`+`os.Stat`) lives in `kagglecli` and is tested *there*.
- **Drift-guard is genuine, not a self-echo** (`internal/stepio/stepio.go:46` + the real-metis e2e): `New()` requires the `METIS_*` vars from env with no cwd fallback, and the Log records that drifting a const was empirically verified to turn the e2e RED.
- **Both prior-review fixes verifiably landed:** `ParseSubmissions` degrades a bad `publicScore` cell to unscored instead of failing the whole list (`parse.go` + `TestParseSubmissionsBadScoreDegrades`); `pollScore` fast-fails on terminal `error` (`TestPollScore_TerminalErrorFastFails`, 1 poll, no sleep); zip-slip guard tested (`TestUnzip_RejectsZipSlip`); `envInt/envDuration` now warn on malformed input.
- **Honest scope accounting.** The authored-fixture caveat (schema unvalidated until first live capture) is stated consistently across fixture header, atlas, plan, and code — the unverified point is named, not hidden behind the green e2e.

### 2. Critical findings

- **`cmd/kaggle-submit/main.go:109` — `submit` reports the score of the wrong submission on the live path (silent wrong output).** `pollScore` uses `kaggle.LatestScored(subs)`, which returns the newest *scored* row in the newest-first list with no correlation to the file just uploaded. Failure scenario: a real competition already has prior scored submissions (the normal case). `submit` uploads `submission.csv` (newest row, `pending`); poll #1's `Submissions` returns `[newest=pending(mine), older=complete 0.78, …]`; `LatestScored` returns the *older* 0.78 row → `submit` writes that previous submission's `file`/`date`/`publicScore` into `submission.json` + `metrics.json{public_score}` and exits 0, as if the just-uploaded file scored 0.78. `parse_test.go:43-49` demonstrates exactly this selection. This is behavior drift from the Done-when ("submitting a file, returning a typed Submission + public score" — of *that* file).
  - *Blocking status:* **Non-blocking at THIS gate** — the committed/verified deliverable is the fake path, which is structurally single-submission and cannot reach this (the e2e passes correctly). By the issue's own honestly-declared scope (live path deferred to operator/kbench#1), this is a must-fix-before-live, not a must-fix-before-this-close — hence the verdict stays FIX-THEN-SHIP, same disposition the M2 review gave its analogous live-only finding. But its severity is Critical: it silently emits a wrong score for the step's core purpose.
  - *Fix sketch:* `submit` must identify *its* submission, not "any scored row." Since Kaggle lists newest-first and `submit` just uploaded, the just-submitted row is `subs[0]` — poll until **`subs[0].Scored()`** (the newest row is scored), not "some row is scored." Optionally also match `filepath.Base(csvPath)` against `subs[0].File` for robustness against a racing concurrent submit. Leave `LatestScored` for a future "best current score" query where any-scored is the right semantics; introduce e.g. `NewestSubmission(subs)`/`subs[0]`-based selection for the submit poll. ARCH-PURPOSE lens: the current code takes the easy subset (any score) rather than the purpose (the score of the file submitted).

### 3. Important findings

- **Missing coverage is exactly why this shipped: the fake cannot model a multi-submission competition.** `cmd/fake-kaggle` stores one `fakeState` per slug and `doSubmissions` emits exactly one row, so no test in the suite ever presents `LatestScored`/`pollScore` with a prior scored submission alongside a new pending one. Fix alongside the Critical: have the fake retain an append-only list per competition (or seed a pre-existing scored submission) and add an e2e/integration case that submits into a competition with a prior scored row, asserting `submit` reports the *new* file's score — that test would have caught the bug and would pin the fix.

### 4. Minor findings

- `cmd/kaggle-submit/main_test.go:120` — **gofmt violation** (comment-alignment on the `t.Setenv("KAGGLE_FAKE_SCORE_AFTER"…)` block); `gofmt -l` flags this file. The M1 review body claimed "gofmt clean"; a `gofmt -w` restores it. Would fail a CI `gofmt -l` gate.
- `internal/kagglecli/cli.go` — `Submissions` and `run` duplicate the `exec.Command(c.bin, …).Output()` + `wrap(c.bin, err)` shape (ARCH-DRY, carried over from the M1 review, still unaddressed). A single `output(args...) ([]byte, error)` helper that `run` calls-and-discards would consolidate.
- `cmd/kaggle-download/main.go` — `unzip`/`extractOne` use `os.Create` default perms, ignoring zip-entry file modes; fine for CSV data, noted for completeness.
- `steps/kaggle/*` `go run` recompiles the entrypoint on every step invocation — documented tradeoff mirroring metis's `uv run`; not hot-path.

### 5. Test coverage notes

- Well covered: async pending→scored iteration, timeout→pending+non-zero, terminal-error fast-fail, loose-file download + zip removal, zip-slip rejection, missing-upstream-artifact, header-reorder independence, bad-score degradation, missing-`fileName`-column error, JSON/CSV round-trips, credential-source table, and the real-metis e2e.
- **Gap (ties to Critical/Important):** no test exercises `submit` against a competition with a *prior* scored submission — the one scenario that distinguishes "return my score" from "return any score." The fake's single-submission model is the structural reason the gap exists.

### 6. Architectural notes for upcoming work

- **ARCH-DRY: PASS.** `kaggletest` is the shared build/wire seam; `FormatSubmissionsCSV`↔`ParseSubmissions` is the single schema; `submission: <upstream-id>` reuses metis's `folds: split` id-naming convention rather than a bespoke path scheme.
- **ARCH-PURE: PASS.** Pure poll/parse/credential core; thin IO shell (`stepio`, `kagglecli`, `unzip`, step `main`s). No business logic buried in IO.
- **ARCH-PURPOSE: FLAG (the Critical).** Shadow-sweep of the Decision-A2 single-source (`METIS_*` contract): consumers = metis runner (source) + kaggle `stepio` (derived, e2e-guarded); promotion to `metis/pkg/stepcontract` is an honest rule-of-three deferral, and Done-when #3 (kbench live thread) is a genuinely separable cross-repo consumer — those deferrals are legitimate. The *under-delivery* is inside `submit` itself: it returns "a score" rather than "the score of what it submitted." For the kbench#1 handoff, this and the schema-validation-on-first-live-capture are the two things standing between "fake-verified" and "live-correct."

### 7. Plan revision recommendations

- **Add a `## Revisions` entry recording the submission-identity gap.** The plan's Task-3 "Behavior" and the Core-concepts `latestScored` bullet describe polling "until scored" via `latestScored` with no mention of correlating the scored row to the uploaded file. Add a revision noting: `LatestScored` (any-scored, newest-first) is the wrong selector for `submit`; submit must key off the newest row (`subs[0]`)/filename match, and the fake must model multiple submissions to cover it. This keeps the plan from continuing to claim `latestScored` is the correct submit-poll primitive.
- No table-vs-code fidelity revision needed — the exported-name and M2-deviation drifts are already recorded in the 2026-07-01 / 2026-07-02 Revisions, and every Core-concepts entity exists at its stated path with the stated PURE/INTEGRATION character.
