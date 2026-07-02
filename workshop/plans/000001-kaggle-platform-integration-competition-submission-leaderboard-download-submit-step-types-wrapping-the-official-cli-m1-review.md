# Boundary Review — kaggle#1 (milestone M1)

| field | value |
|-------|-------|
| issue | 1 — kaggle platform integration: Competition/Submission/Leaderboard + download/submit step-types wrapping the official CLI |
| repo | kaggle |
| issue file | workshop/issues/000001-kaggle-platform-integration-competition-submission-leaderboard-download-submit-step-types-wrapping-the-official-cli.md |
| boundary | milestone M1 |
| milestone | M1 |
| window | d8bf2583f6fd621ffc33f7ac6de3c02bb6d819be^..HEAD |
| command | sdlc milestone-close --issue 1 --milestone M1 |
| reviewer | claude |
| timestamp | 2026-07-01T23:11:27-07:00 |
| verdict | FIX-THEN-SHIP |

## Review

Ignoring 6 permissions.allow entries from .claude/settings.json: this workspace has not been trusted. Run Claude Code interactively here once and accept the trust dialog, or set projects["/Users/xianxu/workspace/kaggle"].hasTrustDialogAccepted: true in /Users/xianxu/.claude.json.
```verdict
verdict: FIX-THEN-SHIP
confidence: high
```

M1 delivers exactly its declared purpose — the pure `pkg/kaggle` library (records + parsers + credential decision), the injectable `internal/kagglecli` IO seam, and a process-level `cmd/fake-kaggle` — with a green test suite (`go test ./...`, `go vet`, `gofmt` all clean) and a genuine client-vs-fake integration test that proves the submit→poll→scored flow *iterates through pending* rather than short-circuiting to the terminal state. The architecture is clean: business logic is pure and table-tested with no IO, the exec boundary is thin, and fake+parser share one CSV schema. Nothing blocks the boundary. The one item worth addressing before M2 builds on it: `ParseSubmissions` aborts the *entire* list on a single malformed `publicScore` cell, and its error branches are untested — cheap to harden now at the "load-bearing fragile boundary" the plan itself names.

### 1. Strengths
- **ARCH-PURE is real, not claimed.** `pkg/kaggle` functions take/return values (`CredentialSource(username, key, fileExists)`, `ParseSubmissions(out string)`), and the tests confirm it — `credentials_test.go` explicitly avoids `t.Setenv`/temp-HOME, and the IO (env-read + `os.Stat`) lives in `kagglecli/cli.go:36-51` and is tested *there*. The pure/IO seam is drawn correctly.
- **Single CSV schema (ARCH-DRY).** `submissionsCSVHeader` in `parse.go:26` is the one source both `ParseSubmissions` and `FormatSubmissionsCSV` derive from; the fake produces via `FormatSubmissionsCSV`, so fake and parser structurally cannot drift from each other. `TestFormatSubmissionsRoundTrip` pins it.
- **The fake models the transition, not the end-state** (`main.go:150-155`) — and the integration test *enforces* iteration (`integration_test.go:60-62`: fails if scored before poll #2). This is the correct fidelity choice and it's guarded.
- **The credential-precheck skip gates on the explicit `KAGGLE_FAKE=1` signal, not a binary-name match** (`cli.go:37`), with a comment naming the exact failure mode it avoids (a real CLI at `.venv/bin/kaggle`). Directly implements the lessons.md rule.
- **Honest provenance.** The authored-fixture caveat is stated in the fixture header, atlas, plan, and code comments — the unvalidated schema point is named, not hidden behind the green e2e.

### 2. Critical findings
None.

### 3. Important findings

- **`pkg/kaggle/parse.go:73` — one malformed `publicScore` cell aborts the entire submissions parse.** A single non-numeric score (`strconv.ParseFloat` fails) makes `ParseSubmissions` return `nil, err` for the *whole* list, discarding every valid row. In M2 the submit step's poll loop calls this on every poll; if real Kaggle ever renders an errored/odd submission's score cell as non-empty-non-numeric (e.g. `-`, `None`), the step fails even though a valid scored row exists. This is on the explicitly-unverified live schema and is exactly the fragility the plan flags at this boundary.
  - *Fix sketch:* treat an unparseable `publicScore` as unscored (`nil`) for that row (optionally alongside a `status`-aware guard), rather than failing the whole parse; OR, if fail-loud is the deliberate choice, add a test that documents it. Either way, add coverage for the two currently-untested error branches: the bad-float path (`parse.go:73`) and the missing-`fileName`-column path (`parse.go:53`). Neither branch is exercised by any test today.

### 4. Minor findings
- `parse.go:39` — `r.Comment = '#'` exists **only** to let the authored fixture carry a provenance header; real `--csv` output has none. Production parser behavior is thus shaped by a test fixture (a fileName beginning with `#` would be silently dropped). Cleaner: strip comment lines when loading the fixture in the test, keeping the production parser fixture-agnostic. Documented in-code, so low priority.
- `cli.go:73` vs `cli.go:80-84` — the `exec.Command(c.bin, …).Output()` + `wrap(c.bin, err)` pattern is duplicated between `Submissions` and `run` (ARCH-DRY, small). Consider a single `output(args...) ([]byte, error)` helper that `run` calls and discards stdout from.
- `main.go:135-140` — `scoreAfter()` silently defaults to 1 on a non-integer `KAGGLE_FAKE_SCORE_AFTER`. Test-only fake, so harmless; noting for completeness.
- `FormatSubmissionsCSV` always emits an empty `privateScore` column even for scored rows (`parse.go:110`). No consumer reads it, so consistent with YAGNI — just flagging that the fake never exercises a populated privateScore.

### 5. Test coverage notes
- Happy paths and key edges are well covered: empty input, header-reorder independence, nil-score/unscored, JSON round-trip, format↔parse round-trip, async transition, submissions-before-submit error, argv construction, and all four credential-source combinations incl. partial-env.
- **Gap:** `ParseSubmissions`' two error branches (bad float, missing `fileName` column) have no test — see the Important finding. This is the class of bug most likely to surface on the first live capture.
- The `parse_test.go` fixture read is read-only testdata feeding a pure function — acceptable, not an ARCH-PURE violation (no mocks, no mutable fs).

### 6. Architectural notes for upcoming work (M2)
- ARCH-DRY / Decision A2: `internal/stepio` will restate metis's `METIS_*`/`with.json`/`metrics.json` contract as local constants. The plan's guard is "drift caught by the M2 e2e." Confirm the e2e actually fails on a renamed `METIS_*` var (i.e. it reads the real metis-emitted env, not stepio's own consts echoed back) — otherwise the drift-detection is illusory. Promote to `metis/pkg/stepcontract` at the 3rd Go consumer as planned.
- The M1 parser fragility above is the boundary M2's poll loop will lean on hardest — resolve the all-or-nothing failure semantics before wiring the retry loop, so bounded-retry behavior is defined against a parser that degrades gracefully.
- ARCH-PURPOSE at *this* boundary passes: M1's purpose is the library (the issue's step-types Done-when is explicitly M2), and the deferred `Leaderboard` is a documented YAGNI with the score-purpose met via `Submission.PublicScore`. No under-delivery of M1's committed scope.

### 7. Plan revision recommendations
- **Core concepts table naming drift (plan `## Core concepts`).** The table lists `credentialSource`, `parseSubmissions`, `latestScored` (unexported), but the code exports them (`CredentialSource`, `ParseSubmissions`, `LatestScored`) — necessarily, since `internal/kagglecli` and `cmd/fake-kaggle` consume them across package boundaries. Add a `## Revisions` entry updating the table (and the prose in "Integration points"/"Notes") to the exported names, so the plan stops claiming an unexported surface the code cannot use. (Content is otherwise faithful; the M1/M2 split, YAGNI-Leaderboard, and fidelity caveat are already recorded in Revisions.)
