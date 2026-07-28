---
id: 000008
status: done
deps: []
github_issue:
created: 2026-07-28
updated: 2026-07-28
estimate_hours: 1.34
started: 2026-07-28T10:24:27-07:00
actual_hours: 2.0
---

# Validate submissions --csv schema against first live capture (status vocab, ref column, date format)

## Problem

`pkg/kaggle/testdata/submissions.csv` is an **authored** fixture, documented in-file and in
`atlas/kaggle-layer.md` as *"the one unverified schema point"* — fake and parser co-derive from it,
so the fake structurally cannot catch a divergence from real Kaggle. That gap closes only against a
live capture.

**The live capture now exists**: `workshop/captures/submissions-live-2026-07-28.csv`, 18 rows from
`kaggle competitions submissions rogii-wellbore-geology-prediction --csv` (official Python CLI,
authenticated, live competition) during the rogii-v2 investigation in kbench — exactly the command
`internal/kagglecli.CLI.Submissions()` shells.

```
ref,fileName,date,description,status,publicScore,privateScore
55058575,submission.csv,2026-07-28 15:17:26.513000,p-dup-surface diagnosis: ...,SubmissionStatus.COMPLETE,9.662,
54846753,submission.csv,2026-07-20 06:28:33.097000,rogii baseline (kbench#18): ...,SubmissionStatus.COMPLETE,,
```

`SubmissionStatus.PENDING` was observed **in the default table output** while polling this session;
the archived `--csv` capture was taken after everything scored, so **no PENDING row exists in the
archive**. Consequence for the canonical-copy rule below: the fixture's pending row is marked
**shape-inferred** (status spelling observed, `--csv` row shape inferred from the scored rows), and
the next capture taken mid-poll supersedes it.

### Four divergences

**The first two are LIVE BEHAVIOR BUGS, not just fixture drift** (this is the finding that resizes
the issue — the original filing said "nothing crashes," which is true only of the *parser*):

1. **Status vocabulary is wrong → the rejected-submission fast-fail never fires.** Live emits
   `SubmissionStatus.COMPLETE` / `SubmissionStatus.PENDING` (Python enum reprs); our constants are
   `"complete"` / `"pending"` / `"error"`. Two production comparisons are therefore
   **always-false against real Kaggle while passing every fake-backed test**:
   - `internal/submit/submit.go:75` — `newest.Status == kaggle.StatusError` is the terminal fast-fail
     for a Kaggle-rejected submission. Dead on live ⇒ a rejected submission burns the **entire poll
     budget** (`maxAttempts` × backoff) before reporting the generic "not scored".
   - `cmd/kaggle-submit/main.go:78` — the "submission rejected by kaggle (status=error)" diagnostic
     never fires; the operator gets the misleading timeout message instead.
2. **A leading `ref` column exists and is dropped.** Live's first column is the numeric submission id
   (`55058575`). Header-driven lookup ignores it harmlessly, but `Submission` has no field for it and
   `FormatSubmissionsCSV` omits it, so the id — the only stable handle for citing a submission (used
   constantly by hand in rogii-v2's anchors table) — is unrepresentable in our state.
3. **Date format differs.** Live `2026-07-28 15:17:26.513000` (space, microseconds, no zone) vs the
   fixture's RFC3339 `2026-07-01T15:00:00Z`. `SubmittedAt` is a string so nothing fails, but the fake
   teaches the wrong shape to any consumer that later parses it.
4. **`COMPLETE` does NOT imply scored.** Capture row 19 (`54846753`) is
   `SubmissionStatus.COMPLETE` with an **empty** `publicScore`. `Scored()` correctly reports false,
   but `cmd/fake-kaggle/main.go:176-180` models complete-and-scored as one atomic transition — so the
   fake teaches a pairing real Kaggle does not honor, and no test pins the real one.

### Declared follow-ups (found in the same capture, deliberately NOT folded in)

5. **`pollScore`'s file-name correlation is a no-op on live → `kaggle#9`** (filed). Every one of the
   18 captured rows is `fileName=submission.csv`, so `internal/submit/submit.go:69`'s "is this row
   ours?" guard can never discriminate; a poll landing before our row registers returns a *previous
   run's score*. Same failure class as divergences 1–2 and invisible to the fake (its prior row uses
   the unreachable filename `prior.csv`). Deferred because the fix is a poll-loop redesign whose
   correlation key (`Ref`) is a **deliverable of this issue** — #9 `deps: [kaggle#8]`. This issue
   therefore leaves the fake's `prior.csv` in place, documented as unfaithful, so the resulting test
   failure lands in #9 as its TDD entry point rather than blocking a schema change here.
6. `internal/kagglecli` models only the file-submit flow; code competitions need
   `submit <slug> -k <owner/kernel> -v <version> -f <output> -m <msg>` (the `-c <slug>` form and
   omitting `-f` both return a bare `400`). File when a code-competition step is actually wanted.

## Spec

Reconcile the schema against the capture, and fix the always-false comparisons **at the root** rather
than by retargeting literals.

- **Normalize AT THE PARSE BOUNDARY (revised after review round 3 — see the cross-repo finding).**
  `ParseSubmissions` is already documented as *"the single CLI-text↔typed-state boundary"*; that is
  exactly where a wire-vocabulary change belongs. So: `NormalizeStatus(raw)` (strip a
  `SubmissionStatus.` prefix, fold case, trim) is applied **during parse**, `Submission.Status` holds
  the **normalized** value, and a new `StatusRaw` preserves the wire string for fidelity/debugging.
  Consequences, all good:
  - the exported constants keep their current values and **every existing comparison starts working
    on live with no call-site edit** — the two dead production comparisons (divergence 1) are fixed
    at the root, not by chasing 21 references;
  - **the `submission.json` artifact contract is unchanged**, which matters because it crosses a repo
    boundary (below);
  - an unrecognized status is normalized-but-preserved (`SubmissionStatus.WEIRD` → `weird`), never
    rejected — the tolerance this issue exists to protect;
  - no `StatusIs` helper is needed. Dropped (Simplicity-First): with `Status` normalized, `==`
    against the constants is correct and obvious.
- **Enumerated consumer that the earlier sweep missed — `submission.json` crosses into kbench.**
  `kbench/e2e/thread_test.py:75` asserts `submit["status"] == "complete"` against the serialized
  record. Storing the raw wire value in `Status` (the round-2 design) would have **broken a peer
  repo's e2e**. Normalizing at parse keeps that contract intact by construction; the Done-when pins
  it, and `StatusRaw` is additive (`omitempty`) so the JSON shape only grows.
- **No code path may reject an unrecognized status.** This issue exists because we guessed a
  vocabulary once; the parser must stay tolerant.
- **The `ERROR` spelling stays shape-inferred, in code and in tests.** We have never observed the
  error state. Tests exercise the *behavior* — "a `SubmissionStatus.<anything>` value normalizes, so
  the existing `== StatusError` terminal path fires" — and carry a comment saying the literal is
  inferred from the observed `SubmissionStatus.` prefix shape, never captured. No test may read as
  though `ERROR` were validated.
- `Submission` gains `Ref`; `ParseSubmissions` reads the column when present (absent ⇒ empty, so
  older captures still parse); `FormatSubmissionsCSV` emits it, keeping fake and parser co-derived
  from one schema.
- `SubmittedAt` stays a string (Simplicity-First — no consumer needs `time.Time`); the observed format
  is recorded in the fixture header.
- **Canonical-copy rule (this issue's own drift class):** `workshop/captures/*.csv` is the immutable
  dated archive; `pkg/kaggle/testdata/submissions.csv` is the working fixture and its header **cites
  the archive path**. A future re-capture adds a new dated archive file, then updates the fixture.
- **Conformance cadence (ARCH-MOCK), placed correctly (revised after review round 3).** The live
  conformance test lives in **`internal/kagglecli`** — not `pkg/kaggle`, which is IO-free
  (ARCH-PURE) — and drives the **production seam** `CLI.Submissions()` rather than re-implementing
  binary resolution and exec (ARCH-DRY). It must be impossible to pass against the fake: it requires
  an explicit `KAGGLE_LIVE_CONFORMANCE=1` opt-in **and** refuses to run when the resolved
  `KAGGLE_CLI` is our fake binary. It asserts the parsed header set equals `submissionsCSVHeader`
  (sourced from the shared constant — which means `pkg/kaggle` must EXPORT it, e.g.
  `SubmissionsCSVHeader() []string`, since `internal/kagglecli` cannot see the unexported var; that
  tiny API addition is part of item 3, not a freebie) and
  that every non-empty status matches `^SubmissionStatus\.[A-Z]+$`. Skips — never fails — when the
  opt-in, credentials, competition slug, or rows are absent.

## Done when

- [x] `testdata/submissions.csv` is the live capture; header states captured-not-authored, names the source command + date, and cites the `workshop/captures/` archive path
- [x] `NormalizeStatus` applied inside `ParseSubmissions`; `Status` normalized, `StatusRaw` carries the wire value; the 21 existing comparison sites are UNCHANGED and pass (that they need no edit is the design's proof)
- [x] `internal/submit` fast-fails a rejected submission: a `SubmissionStatus.<anything>` value normalizes so the existing `== StatusError` terminal path fires without burning the poll budget — test comment marks the ERROR literal shape-inferred, never observed
- [x] **Cross-repo contract pinned:** a test asserts the serialized `submission.json` `status` is the normalized vocabulary (`complete`), matching `kbench/e2e/thread_test.py:75`; `StatusRaw` is additive/`omitempty`
- [x] The pending path stays covered: the live capture has no pending-unscored row, so a **synthetic** table case pins "`LatestScored` skips the newest pending row" (today's `parse_test.go:36-50` coverage must not be lost in the fixture swap)
- [x] An unrecognized status parses without error and is preserved verbatim in `Status` (regression test for the guessing failure mode)
- [x] `Submission.Ref` populated from the live capture; absent-`ref` case covered
- [x] `COMPLETE`-without-score is pinned: `Scored()` false, `LatestScored` skips it (divergence 4)
- [x] `LatestScored` still selects on `PublicScore != nil`, verified against the captured mixed rows
- [x] `cmd/fake-kaggle` emits the live schema (live status spellings + observed date shape) AND **mints a monotonic `ref` in its persisted state at submit time**, echoing it at `submissions` — otherwise the field this issue adds is never exercised through the process-level seam; its doc comment names complete-without-score, and the unfaithful `prior.csv` filename (kaggle#9's entry point), as states it does not model
- [x] `//go:build live` conformance test in **`internal/kagglecli`** (not the IO-free `pkg/kaggle`), driving the production `CLI.Submissions()` seam: requires `KAGGLE_LIVE_CONFORMANCE=1`, REFUSES to run against the fake binary, asserts header == `submissionsCSVHeader` (sourced from the constant, not restated) and every non-empty status matches `^SubmissionStatus\.[A-Z]+$`; skips on missing opt-in/creds/slug/rows. Documented in the atlas as the drift-detection cadence
- [x] `atlas/kaggle-layer.md` corrected at **all five** restatements (:12 vocab, :13, :22 fake transition, :33 and :80 honesty caveats) — including the false claim at :13 that `ParseSubmissions` skips `#` comment lines (it does not; `stripComments` lives in `parse_test.go`)
- [x] `go test ./...` green

## Estimate

```estimate
model: estimate-logic-v3.1
familiarity: 1.0
design-buffer: 0.15
item: smaller-go-module      design=0.20 impl=0.45
item: smaller-go-module      design=0.00 impl=0.20
item: smaller-go-module      design=0.05 impl=0.25
item: atlas-docs             design=0.00 impl=0.15
total: 1.34
```

Sizing (0.67 → 1.08 → 1.34 → 1.24 → **1.34** across four review rounds; the last one moved DOWN because
round 3's cross-repo finding produced a *simpler* design — normalizing at the parse boundary removes
the 21-site call sweep entirely, so item 2 shrinks to verification-plus-fake-state while item 1
absorbs the extra design; the last +0.10 is item 3 taking the exported-header API addition rather
than letting it absorb silently — which was the stated reason for splitting item 3 out): item 1 is `pkg/kaggle` — `Ref` through parse/format, `NormalizeStatus`/`StatusIs`,
constant re-documentation, fixture swap, and ~6 tests (unknown-status regression, complete-without-
score pin, synthetic pending case, `Ref` present/absent). Item 2 is the call-site sweep the first
estimate missed entirely — 21 references across `internal/submit` (2 source + 11 test),
`cmd/kaggle-submit` (1 + 1), `cmd/fake-kaggle` (3 + 2), plus `cmd/kaggle/main.go:96` and the
cross-repo `kbench/e2e/thread_test.py:75` — now mostly *verification* that these need no edit
(the point of normalizing at the boundary), plus the fake's **state-model** change to mint refs. Item 3 splits
the `//go:build live` conformance test out per the review, since it was the least-specified item and
an overrun there should be visible rather than hidden inside item 1. Item 4 is five atlas paragraphs
plus the false comment-skipping claim.

Design is small (the capture is the spec) but not nil: the normalize-vs-retarget fork was taken
deliberately because two production comparisons are dead on live, and the fold-vs-defer call on
divergence #5 (→ kaggle#9) is recorded above.

## Plan

- [x] `pkg/kaggle/testdata/submissions.csv` ← the live capture, provenance header citing `workshop/captures/submissions-live-2026-07-28.csv`
- [x] `pkg/kaggle/submission.go`: add `Ref` + `StatusRaw`; `NormalizeStatus`; re-document constants as the normalized vocabulary (ERROR spelling still unobserved — say so)
- [x] `pkg/kaggle/parse.go`: `colRef` + `ref` in `submissionsCSVHeader`, parse when present, emit in `FormatSubmissionsCSV`; update the header doc comment (no longer "authored")
- [x] `pkg/kaggle/*_test.go`: fixture-count + assertions to the live rows; unknown-status regression; complete-without-score pin; SYNTHETIC pending case (preserves today's `LatestScored`-skips-pending coverage); `Ref` present/absent table
- [x] `internal/submit` + `cmd/kaggle-submit` + `cmd/kaggle`: comparisons NEED NO EDIT (normalized at parse) — verify by test, not by change; add the rejected-submission fast-fail test as BEHAVIOR (shape-inferred ERROR literal, commented never-observed); `cmd/kaggle/main.go:96` prints `StatusRaw` in the timeout message (the wire value is the useful diagnostic)
- [x] `cmd/fake-kaggle/main.go`: live status spellings + observed date shape; **mint monotonic `ref` in `fakeState` at submit, echo at submissions**; doc comment names complete-without-score AND the unfaithful `prior.csv` (kaggle#9's entry point) as unmodeled states
- [x] `internal/kagglecli/conformance_live_test.go` (`//go:build live`): drives `CLI.Submissions()`, opt-in gated, fake-refusing, header-from-constant + prefix-shape assertions
- [x] `atlas/kaggle-layer.md`: all five restatements + the false `#`-comment claim; note the live conformance test and the captures/ archive convention
- [x] `go test ./...`

## Log

### 2026-07-28
- 2026-07-28: closed — judgment actual (telemetry unavailable: session cwd was the kbench peer, so transcripts aren't under this repo). EVIDENCE: go vet ./... clean; go test -count=1 ./... green 9/9 packages. LIVE conformance PASSES against real Kaggle (KAGGLE_LIVE_CONFORMANCE=1 ... go test -tags live ./internal/kagglecli/ -run Conformance -v => 18 rows, header [ref fileName date description status publicScore privateScore] matched vs the exported constant, all statuses matching the enum-repr shape) — the ARCH-MOCK drift check is verified running, not just written. CROSS-REPO contract verified end-to-end: uv run pytest e2e/thread_test.py in kbench => 3 passed, incl. the submit[status]=='complete' assertion the store-raw alternative would have broken. DESIGN PROOF: normalizing at the parse boundary left all 21 enumerated call sites unedited (only the fixture-count assertion failed after the core change), fixing the two dead live comparisons without touching them.; review verdict: FIX-THEN-SHIP

Filed from the kbench rogii-v2 session (the live-Kaggle work deferred to the operator per kbench#1),
with a real authenticated capture attached (both COMPLETE and PENDING rows observed).

**Plan revised after the `change-code` plan-quality review** (estimate 0.67 → 1.08). The review was
right on every checkable claim, and two findings changed the issue's character:
- it found a **fourth divergence** — `COMPLETE` with an empty score (capture row 19), which the fake
  models as impossible; verified in the capture before adopting;
- tracing the status constants to their call sites showed the divergence is **not cosmetic**:
  `internal/submit/submit.go:75` and `cmd/kaggle-submit/main.go:78` are always-false on live, so a
  rejected submission burns the full poll budget and reports the wrong error. That promoted the fix
  from "retarget three literals" to "normalize at the boundary" (ARCH-DRY), which also resolves the
  unobserved `ERROR` spelling without inventing it.
It also caught a false statement in `atlas/kaggle-layer.md:13` (claims `ParseSubmissions` skips `#`
comment lines; it is fixture-agnostic by design and `stripComments` lives in the test) — verified and
folded into the plan.

Procedural knowledge from the same session (CLI forms, kernel metadata, server-side gotchas) landed
separately as the `kaggle-base` skill in `construct/local/base/`.

### 2026-07-28 (implementation)

Shipped. `go vet ./...` clean; `go test -count=1 ./...` green across all 9 packages.

**The design's own proof:** normalizing at the parse boundary meant the 21 enumerated call sites
needed **zero edits** — the only test that failed after the core change was the fixture-count
assertion. The two dead live comparisons (`internal/submit/submit.go:75`,
`cmd/kaggle-submit/main.go:78`) are fixed without being touched.

**Live-verified, not just described:**
- `KAGGLE_LIVE_CONFORMANCE=1 KAGGLE_CONFORMANCE_SLUG=rogii-wellbore-geology-prediction go test -tags live ./internal/kagglecli/ -run Conformance -v`
  → **PASS against real Kaggle**: 18 rows, header `[ref fileName date description status publicScore privateScore]`
  matched against the exported constant, every status matching `^SubmissionStatus\.[A-Z]+$`. The
  ARCH-MOCK drift-detection gap is closed by a check that has actually run, not a documented intent.
  (Caught while running it: the first draft used `CLI{}` instead of `New()`, bypassing the
  `${KAGGLE_CLI:-kaggle}` resolution — the exact reimplement-the-seam mistake the review warned about;
  it silently skipped instead of testing. Fixed to use the production constructor.)
- **Cross-repo contract verified for real:** `uv run pytest e2e/thread_test.py` in kbench → 3 passed,
  including the `submit["status"] == "complete"` assertion that the round-2 (store-raw) design would
  have broken.

Deviations from plan, both simplifications: (a) the fixture is a curated 5-row subset of the archive
(3 scored + the complete-unscored row + the shape-inferred pending row) rather than all 18 — it
covers every state the tests pin and keeps the diff readable; the archive stays canonical. (b) No
`StatusIs` helper (dropped in the round-3 redesign) — with `Status` normalized, `==` is correct.

`prior.csv` in the fake is deliberately left unfaithful, with a comment naming it as kaggle#9's RED
test entry point.

### 2026-07-28 (close review — FIX-THEN-SHIP, fixes bundled into the close commit)

Verdict FIX-THEN-SHIP: no Critical, eight Important. All eight fixed before committing, per #174.

- **(a) ARCH-DRY, the real one:** the wire PREFIX — the half that actually broke production — was
  still restated in three files while only the column set had been single-sourced. Exported
  `StatusWirePrefix` + `WireStatus()` (the inverse of `NormalizeStatus`); the fake and the
  conformance regexp now derive from it.
- **(b) ARCH-MOCK:** `FormatSubmissionsCSV` fell back to the *normalized* word when `StatusRaw` was
  empty, so wire fidelity was opt-in per call site and six existing test constructions rendered
  spellings Kaggle never emits. Now derives the wire form via `WireStatus` — fidelity by default.
- **(c)** Pinned the fake's two new behaviors through the process seam (wire spelling asserted on raw
  stdout BEFORE parsing — parsing would normalize it away; `ref` non-empty; a resubmit's ref strictly
  greater). Both were deletable without a test noticing.
- **(d)** The curated fixture had dropped every quoted cell; swapped in captured row `54996757` and
  asserted its comma-bearing description round-trips. A parser rewritten on `strings.Split` would
  have passed the whole suite and broken on the first live poll.
- **(e)** A checked Plan item was undelivered: `cmd/kaggle/main.go` still printed `sub.Status`. Now
  prints the wire value via `statusForMessage`, whose fallback is load-bearing — on budget exhaustion
  `pollScore` returns a synthetic pending record with no `StatusRaw`.
- **(f)** The fake's header still advertised the blind spot this commit closed; rewritten, and it now
  names BOTH unmodeled states (complete-without-score, `prior.csv`).
- **(g)** Atlas env tables gained `KAGGLE_LIVE_CONFORMANCE` / `KAGGLE_CONFORMANCE_SLUG`, and
  `workshop/captures/` is documented with its canonical-copy rule. Deliberately NOT registered in
  AGENTS.md's Directory Structure: that file is composed from `construct/base.manifest`, so a local
  edit is clobbered on the next sync — registering `captures/` is an ariadne base-layer change.
- **(h)** The live-tagged file was excluded from every default build and could rot unnoticed. Added
  `make go-check` (gofmt + `go vet ./...` + **`go vet -tags live ./...`** + `go test ./...`) in
  **`Makefile.local`** — caught mid-flight that this repo's `Makefile` is a SYMLINK to the ariadne
  base, so the first version of this target was silently appended to the shared base layer and would
  have followed every consumer repo; reverted there, moved to the `-include Makefile.local` hook the
  base provides for exactly this, and the caveat is now written at the top of that file. Also
  downgraded the atlas's "cadence" claim to the truth: operator-run, not scheduled. Side-quest: the
  new gate immediately caught a pre-existing gofmt violation in `cmd/kaggle-download/pins_test.go`
  (untouched by this issue) — formatted.

Re-verified after the fixes: `make go-check` green; live conformance still PASSES against real
Kaggle; kbench `e2e/thread_test.py` still 3 passed.
