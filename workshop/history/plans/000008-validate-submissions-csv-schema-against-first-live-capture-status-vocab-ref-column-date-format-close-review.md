# Boundary Review — kaggle#8 (whole-issue close)

| field | value |
|-------|-------|
| issue | 8 — Validate submissions --csv schema against first live capture (status vocab, ref column, date format) |
| repo | kaggle |
| issue file | workshop/issues/000008-validate-submissions-csv-schema-against-first-live-capture-status-vocab-ref-column-date-format.md |
| boundary | whole-issue close |
| milestone | — |
| window | 5f6c8d18e25cf62e8daf7ee3346886548fc18109..HEAD |
| command | sdlc close --issue 8 |
| reviewer | claude |
| timestamp | 2026-07-28T11:08:42-07:00 |
| verdict | FIX-THEN-SHIP |

## Review

Ignoring 6 permissions.allow entries from .claude/settings.json: this workspace has not been trusted. Run Claude Code interactively here once and accept the trust dialog, or set projects["/Users/xianxu/workspace/kaggle"].hasTrustDialogAccepted: true in /Users/xianxu/.claude.json.
```verdict
verdict: FIX-THEN-SHIP
confidence: medium
```

The core of this boundary is right and genuinely delivered: normalizing at `ParseSubmissions` fixes both dead live comparisons (`internal/submit/submit.go:75`, `cmd/kaggle-submit/main.go:78`) without touching either call site, the fixture is a real capture with an immutable dated archive behind it, and the conformance check was actually *run* against live Kaggle rather than merely written. Nothing here is a correctness bug and nothing blocks the boundary. What holds it back from SHIP is that the issue's own thesis — *one source for the wire schema* — was only half applied: the column set got exported and derived, but the `SubmissionStatus.` prefix (the divergence that actually broke production) is now hand-restated in three Go files, and `FormatSubmissionsCSV`'s silent fallback to the normalized spelling means "the fake speaks wire vocabulary" is a per-call-site convention rather than a property of the formatter. Compounding that, the two new fake behaviors this issue added (ref minting, wire rendering) are unasserted — delete them and the suite stays green. **Caveat on my confidence: the Bash tool was unavailable for this entire session (harness `EPERM` on every invocation), so I could not run `go test`, `go vet`, or any `git` command. All findings below are from reading the tree at HEAD; the Log's green-suite claim is unverified by me.**

## 1. Strengths

- **`pkg/kaggle/parse.go:86-93` — the design's central call is correct.** Normalizing at the single CLI-text↔typed-state boundary, with `StatusRaw` additive and `omitempty`, is what let 21 call sites go unedited *and* kept `submission.json`'s cross-repo shape stable. The Log's "the only test that failed was the fixture-count assertion" is the right proof to have looked for.
- **`pkg/kaggle/submission.go:30-36` — `NormalizeStatus` is total, pure, and honest.** Preserving `SubmissionStatus.WEIRD` → `weird` instead of rejecting or coercing is exactly the right lesson to draw from having guessed a vocabulary once, and `TestNormalizeStatus` (`parse_test.go:117`) pins it including the empty and already-normalized cases.
- **`internal/kagglecli/conformance_live_test.go` placement.** IO-bearing check in the IO package, not in IO-free `pkg/kaggle`; driving `New()` (not `CLI{}`) so it exercises the shipped `${KAGGLE_CLI:-kaggle}` resolution; refusing to run when `KAGGLE_CLI` looks like the fake. The Log's note that the first draft used `CLI{}` and silently skipped — caught by actually running it — is the kind of thing that only surfaces when you run the check.
- **`pkg/kaggle/parse.go:32-36` + `parse_test.go:239` — `SubmissionsCSVHeader()` returns a defensive copy and there's a test that mutates the result to prove it.** Small, but it's a newly-exported surface and it was hardened on the way out.
- **`workshop/captures/` as an immutable dated archive with the working fixture citing it.** The canonical-copy rule is stated in the fixture header itself, where a future re-capturer will actually read it.

## 2. Critical findings

None.

## 3. Important findings

**(a) ARCH-DRY — the wire prefix is restated in three places; only the column half of the schema was single-sourced.** `pkg/kaggle/submission.go:21` (`statusPrefix`), `cmd/fake-kaggle/main.go:223` (`"SubmissionStatus." + strings.ToUpper(...)`), `internal/kagglecli/conformance_live_test.go:39` (`^SubmissionStatus\.[A-Z]+$`). The column set was exported *specifically* so the conformance test wouldn't restate it — the exact same argument applies to the prefix, and the prefix is the part that was actually wrong before this issue. Fix: export the inverse next to `NormalizeStatus` in `pkg/kaggle/submission.go`:
```go
// WireStatus renders a normalized status the way the CLI prints it (the inverse of
// NormalizeStatus). Both derive from statusPrefix, so the wire spelling has ONE source.
func WireStatus(normalized string) string { return statusPrefix + strings.ToUpper(normalized) }
// StatusWirePrefix reports the enum-repr prefix, for conformance assertions.
func StatusWirePrefix() string { return statusPrefix }
```
then delete `cmd/fake-kaggle.wireStatus` in favor of `kaggle.WireStatus`, and build `statusWireShape` from `regexp.QuoteMeta(kaggle.StatusWirePrefix())`.

**(b) ARCH-MOCK — `FormatSubmissionsCSV`'s fallback makes wire fidelity opt-in per call site.** `pkg/kaggle/parse.go:135-138` falls back to the *normalized* value when `StatusRaw` is empty, so the guarantee "the fake teaches real shapes" holds only where a caller remembered to set `StatusRaw`. Six live call sites already don't: `internal/submit/submit_test.go:18,44,67,72,94,130` all construct `Submission{Status: kaggle.StatusPending}` with no raw, so they render bare `pending`/`complete`/`error` — precisely the shape this issue proved Kaggle never emits. Fix (composes with (a)):
```go
status := s.StatusRaw
if status == "" && s.Status != "" {
    status = WireStatus(s.Status) // the formatter IS the inverse — don't let callers opt out
}
```
That also lets `cmd/fake-kaggle` drop its `StatusRaw:` lines entirely.

**(c) Test coverage — the fake's two new behaviors are unpinned.** Nothing asserts the minted `ref` reaches a consumer through the process seam, and nothing asserts the fake's stdout carries the wire spelling. Delete `Ref: strconv.Itoa(st.Ref)` (`cmd/fake-kaggle/main.go:184`) or both `StatusRaw: wireStatus(...)` lines (`:189,194`) and `cmd/fake-kaggle/main_test.go`, `internal/kagglecli/integration_test.go`, and `e2e/e2e_test.go` all stay green — the status assertions compare against the *normalized* constants, which the fallback in (b) satisfies either way. Done-when line 141 justifies ref-minting as "otherwise the field this issue adds is never exercised through the process-level seam"; the exercise landed but nothing would notice its removal. Fix: in `TestFakeSubmitThenAsyncScoring` (`cmd/fake-kaggle/main_test.go:38`), assert `strings.Contains(out.String(), "SubmissionStatus.")` *before* parsing, assert `subs[0].Ref != ""`, and add a resubmit round asserting the second ref is strictly greater.

**(d) Test coverage — the curated fixture dropped every quoted CSV cell.** 8 of the 18 rows in `workshop/captures/submissions-live-2026-07-28.csv` have quoted, comma-bearing `description` values (lines 6, 9-12, 14, 18); all five rows kept in `pkg/kaggle/testdata/submissions.csv` are unquoted, and no other test in the suite parses a quoted cell. `encoding/csv` handles it today, but the whole suite would stay green through a parser rewritten on `strings.Split` — and then break on the first live poll. Fix: swap one fixture row for a quoted one from the archive (e.g. capture line 6, `54996757`) and assert the description round-trips with its embedded commas intact.

**(e) Requirements traceability — a checked Plan item is not delivered.** Plan line 184 is `[x]` and ends "`cmd/kaggle/main.go:96` prints `StatusRaw` in the timeout message (the wire value is the useful diagnostic)". `cmd/kaggle/main.go:96` still prints `sub.Status`, and the file is absent from the diff. Note there's a real reason to hesitate: on budget exhaustion where our row never appeared, `pollScore` returns the synthetic `last := Submission{Status: StatusPending}` (`internal/submit/submit.go:56`) which has no `StatusRaw`, so a naive swap prints empty. Either implement it with a fallback (`status=%s` on `cmp.Or(sub.StatusRaw, sub.Status)`) or uncheck the clause and record the reversal in `## Revisions`. The Log's "Deviations from plan, both simplifications" enumerates two deviations and this isn't one of them.

**(f) Doc drift in the file this issue is about.** `cmd/fake-kaggle/main.go:10-13` still reads *"SHARED BLIND SPOT: … neither is validated against real Kaggle's wire format. That gap closes only on the first live capture."* That gap closed in this very commit. Separately, Done-when line 141 requires the fake's doc comment to name **complete-without-score** as a state it does not model — `prior.csv` is named (inline at `:205-208`), but complete-without-score appears nowhere in `cmd/fake-kaggle/main.go`; only the atlas (`kaggle-layer.md:22`) records it. A reader who opens the fake — the audience the Done-when targeted — gets a stale warning and a missing one.

**(g) Docs gate — new env surface missing from the atlas tables.** `KAGGLE_LIVE_CONFORMANCE` and `KAGGLE_CONFORMANCE_SLUG` are things an operator types. `atlas/kaggle-layer.md` has two env tables (`:26-31`, `:75-78`) that exist to enumerate exactly this; neither was updated, and only `KAGGLE_LIVE_CONFORMANCE` appears in prose at `:33`. `KAGGLE_CONFORMANCE_SLUG` is documented only inside the test file's own comment. Also, `workshop/captures/` is a new `workshop/` subdirectory and is not registered in CLAUDE.md's "Directory Structure" list alongside `issues/ plans/ targets/ history/ parley/ pensive/`. (No README.md exists in this repo, so the README half of the gate is N/A.)

**(h) ARCH-MOCK — the conformance check has no cadence, and CI never even compiles it.** `atlas/kaggle-layer.md:33` and Done-when line 142 call this "the drift-detection cadence," but nothing schedules it, and because of `//go:build live` the file is excluded from every normal `go build`/`go vet`/`go test` — so it can rot into a compile error silently and the drift check will simply be broken the next time someone reaches for it. Cheapest real fix: add `go vet -tags live ./...` to the merge checks. Then either wire a scheduled run or downgrade the atlas wording from "cadence" to "operator-run, manual" so the doc doesn't over-claim.

## 4. Minor findings

- `pkg/kaggle/testdata/submissions.csv:19` — the synthetic pending row `55999999` carries the **newest** timestamp (`16:00:00`) yet sits **last**, contradicting the newest-first ordering that `pollScore`'s `subs[0]`-is-ours logic depends on. A live pending row would lead. The header flags the row as shape-inferred but not its impossible position.
- `pkg/kaggle/parse.go:16` says the pre-capture schema was "wrong in three ways"; the issue, atlas, and `conformance_live_test.go:8` all say four. Pick one.
- `TestSubmissionJSONStatusIsNormalized` (`internal/submit/submit_test.go:191`) touches nothing in `internal/submit` — it exercises `kaggle.ParseSubmissions` + `json.Marshal`. It belongs in `pkg/kaggle/submission_test.go`.
- `pkg/kaggle/parse_test.go:176-177` and `:199` still use bare `complete`/`error`; `pkg/kaggle/submission_test.go:24` still uses RFC3339 `2026-07-01T12:00:00Z`. Tolerated by design, but they're leftover instances of the shapes this issue disproved.
- `conformance_live_test.go:70` takes the first output line as the header. A CLI warning banner ahead of the CSV would produce a confusing set-mismatch failure rather than a clean skip — worth a comment or a "find the line starting with `ref,`" scan.
- `atlas/index.md:13-14` still reads "live-Kaggle deferred to the operator / kbench#1". Accurate as scoped to the ad-hoc CLI, but it's the sixth restatement of the honesty claim and wasn't in the five the issue enumerated.

## 5. Test coverage notes

Strong where it counts: `TestPollScore_WireErrorFastFails` asserts the *behavior* (one poll, zero sleeps) rather than the literal, and its comment correctly refuses to claim `ERROR` is validated. `TestLatestScoredSkipsLeadingPending` preserves the pending-leads coverage the fixture swap would otherwise have lost — that was called out in Done-when and actually delivered. The complete-without-score pin looks up row `54846753` by ref instead of by index, so it survives fixture reordering.

Three gaps, in priority order: the fake's ref/wire behavior (3c), quoted CSV cells (3d), and — noted rather than demanded — nothing exercises `ParseSubmissions` on the **full 18-row archive**. A `TestParseSubmissions_FullArchive` reading `../../workshop/captures/*.csv` and asserting all 18 rows parse with non-empty refs would cost three lines and would have caught (3d) for free. Whether the test tree should reach into `workshop/` is a judgment call; copying the full capture into `testdata/` as a second fixture is the alternative.

## 6. Architectural notes for upcoming work

- **ARCH-DRY — FLAG.** The column half is exemplary (`SubmissionsCSVHeader()`, defensive copy, asserted). The status half is not: prefix literal ×3 (finding 3a), plus the formatter fallback that lets callers bypass the wire spelling (3b). The shadow sweep over consumers of the schema: `ParseSubmissions` ✅ derives, `FormatSubmissionsCSV` ✅ for columns / ❌ for status, `cmd/fake-kaggle.wireStatus` ❌ restates, `conformance_live_test.statusWireShape` ❌ restates, testdata fixture ✅ captured, `construct/local/base/SKILL.md:64,82` and the atlas ✅ (documentation, correct, acceptable as restatement).
- **ARCH-PURE — PASS.** `NormalizeStatus` is deterministic and unit-tested with no IO; `pkg/kaggle` stays IO-free; the conformance test was deliberately kept out of it and into the seam package; `pollScore`'s injected clock is untouched. No mocks are needed to run any pure test in this diff.
- **ARCH-PURPOSE — PASS with one deferral.** The stated purpose — kill the authored schema, prove it live — is genuinely fulfilled, not reduced to the cheap win; the check was executed against real Kaggle, which is the part most implementations would have skipped. The under-delivery is narrow: Plan line 184's `StatusRaw` diagnostic (3e) and the fake's doc comment (3f). Deferring kaggle#9 is a legitimate separable extension, correctly filed with `deps: [kaggle#8]` and with `prior.csv` left in place as its documented RED entry point — that reasoning is sound and I'd keep it.
- **ARCH-MOCK — FLAG.** The process-level fake is at the right seam and production and test flow share it. But its newly-modeled behavior is unpinned (3c), the wire rendering is bypassable (3b), and the live conformance check — the thing ARCH-MOCK actually asks for — has no schedule and is excluded from compilation (3h). Right now the fake's fidelity rests on the reviewer having read it, not on a test.
- **For kaggle#9:** the `Ref` field it needs now exists and the fake mints monotonic ids, so the correlation fix is unblocked as designed. When #9 flips `prior.csv` → `submission.csv`, findings (3b) and (3c) become blocking there — a fake whose distinguishing field is untested cannot serve as #9's RED test. Fixing them now is the cheaper ordering.
- **Per AGENTS.md §4**, `workshop/lessons.md:6` (the lesson that produced this issue) has now been discharged and is worth a follow-on entry: *"A shared formatter that falls back to normalized values when the raw is unset re-opens the fidelity hole it was built to close — make the formatter derive the wire form, don't ask every call site to supply it."*

## 7. Plan revision recommendations

Add a `## Revisions` section to `workshop/issues/000008-...md` with:

1. **Plan line 184 — partially reversed.** The `cmd/kaggle/main.go:96` clause ("prints `StatusRaw` in the timeout message") is checked but not implemented, and the file is not in the commit range. Either implement with a fallback (the synthetic timeout `Submission` has no `StatusRaw`) or record the reversal and its reason.
2. **Done-when line 141 — partially delivered.** The fake's doc comment names `prior.csv` (inline at `main.go:205`) but not complete-without-score, and `main.go:10-13` still carries the superseded "SHARED BLIND SPOT / gap closes only on the first live capture" paragraph. Narrow the claim or finish the comment.
3. **Fixture-curation consequence.** Record that curating 18 rows → 5 dropped every quoted/comma-bearing `description`, removing CSV-quoting coverage the archive demonstrably contains — with whichever remedy is chosen (swap a row, or add a full-archive parse test).
4. **Done-when line 142 — "cadence" over-claims.** The conformance test is opt-in and manual, with no scheduler and no CI compile. State the actual trigger, or add the scheduling/`-tags live` vet step that makes the word true.
5. **Scope note on the single-source deliverable.** `SubmissionsCSVHeader()` was exported to give the column set one source; the `SubmissionStatus.` prefix — the divergence that actually broke production — was left restated in three files. If finding (3a) is deferred rather than fixed, record it here as an explicit known deferral rather than letting it read as complete.
