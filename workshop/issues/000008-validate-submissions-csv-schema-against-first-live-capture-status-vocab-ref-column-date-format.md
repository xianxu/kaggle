---
id: 000008
status: open
deps: []
github_issue:
created: 2026-07-28
updated: 2026-07-28
estimate_hours:
---

# Validate submissions --csv schema against first live capture (status vocab, ref column, date format)

## Problem

`pkg/kaggle/testdata/submissions.csv` is an **authored** fixture and is documented in-file and in
`atlas/kaggle-layer.md` as *"the one unverified schema point"* — the fake and the parser co-derive
from it, so the fake structurally cannot catch a divergence from real Kaggle output. That gap closes
only against a live capture.

**The live capture now exists.** Captured 2026-07-28 from
`kaggle competitions submissions rogii-wellbore-geology-prediction --csv` (official Python CLI,
authenticated, live competition) during the rogii-v2 investigation in kbench — i.e. exactly the
command `internal/kagglecli.CLI.Submissions()` shells:

```
ref,fileName,date,description,status,publicScore,privateScore
55058575,submission.csv,2026-07-28 15:17:26.513000,p-dup-surface diagnosis: ...,SubmissionStatus.COMPLETE,9.662,
55047852,submission.csv,2026-07-28 07:07:22.190000,commons-repro diagnosis: ...,SubmissionStatus.COMPLETE,7.701,
54996757,submission.csv,2026-07-26 09:07:41.073000,"v9: seq fallback leg (in-kernel TCN, a-seq-blend) — LOO 9.164, ...",SubmissionStatus.COMPLETE,9.529,
```

A `PENDING` row (same shape, `SubmissionStatus.PENDING`, empty `publicScore`) was also observed while
polling, confirming the async transition the fake models.

**Three divergences from the fixture.** Note up front: **nothing crashes today** — the parser is
defensively written (header-driven column lookup, `Status` stored as an opaque string,
`LatestScored` keying on `PublicScore != nil` rather than on the status word). That design is what
contains the blast radius. The divergences are still real and each has a consumer-facing consequence:

1. **Status vocabulary is wrong.** Live emits `SubmissionStatus.COMPLETE` / `SubmissionStatus.PENDING`
   (Python enum reprs), not `complete` / `pending`. So `pkg/kaggle/submission.go`'s
   `StatusPending/StatusComplete/StatusError` constants match **no live value**, and any consumer that
   writes `s.Status == kaggle.StatusComplete` is silently always-false against real Kaggle while
   passing every fake-backed test. `Scored()` is unaffected (it reads `PublicScore`).
   Unknown: whether the error state is `SubmissionStatus.ERROR` — not observed; do not guess it.
2. **A leading `ref` column exists and is dropped.** Live's first column is the numeric submission id
   (e.g. `55058575`). The header-driven parser ignores it harmlessly, but `Submission` has no field
   for it and `FormatSubmissionsCSV` omits it — so the fake's output is schema-divergent, and the
   submission id (the only stable handle for referring to a submission, e.g. in an experiment ledger's
   anchors table) is unrepresentable in our state. This was used constantly by hand in rogii-v2.
3. **Date format differs.** Live: `2026-07-28 15:17:26.513000` (space separator, microseconds, no
   timezone). Fixture: `2026-07-01T15:00:00Z` (RFC3339). `SubmittedAt` is a string so parsing does not
   fail, but any consumer doing time arithmetic on it breaks, and the fake teaches the wrong shape.

Related but out of scope for this issue (a different seam): the `competitions submit` argument form
for **code competitions** is `submit <slug> -k <owner/kernel> -v <version> -f <output-file> -m <msg>`;
the `-c <slug>` form and omitting `-f` both return bare `400 Bad Request`. `internal/kagglecli`
currently models only the file-submit flow.

## Spec

Replace the authored fixture with the live capture and reconcile the three divergences, keeping the
parser's defensive behavior (an unknown status string must remain non-fatal — the live vocab may
change again, and this issue exists because we guessed once).

- `testdata/submissions.csv` becomes the **captured** rows (provenance comment updated: captured
  2026-07-28, competition + CLI version noted, replacing the authored fixture).
- Status constants carry the live values. Keep them as *documentation of observed values*, not as a
  closed enum: no code path may reject an unrecognized status.
- `Submission` gains `Ref` (the submission id); `ParseSubmissions` reads it when the column is present
  (absent ⇒ empty, so older captures still parse); `FormatSubmissionsCSV` emits it, so fake and
  parser stay co-derived from one schema.
- Date: keep `SubmittedAt` a string (ARCH-simplicity — no consumer needs `time.Time` yet); record the
  observed format in the fixture header so a future parser has the truth.
- The `ERROR` status string stays unknown and is marked as such in the comment — do not invent it.

## Done when

- [ ] `testdata/submissions.csv` is the live capture, header comment states captured-not-authored with date + source command
- [ ] `pkg/kaggle` round-trips it: `ParseSubmissions` → `FormatSubmissionsCSV` reproduces the captured rows (modulo documented normalization)
- [ ] `Submission.Ref` populated from the live capture; absent-column case covered by a table test
- [ ] A status value not in the known set parses without error and is preserved verbatim (regression test — the guessing failure mode)
- [ ] `LatestScored` still selects on `PublicScore != nil`, verified against the captured mixed pending/complete rows
- [ ] `cmd/fake-kaggle` emits the live schema (ref column + live status strings), so fake-backed consumers see real shapes
- [ ] `atlas/kaggle-layer.md` honesty caveat updated: the schema point is validated; the remaining unverified surface is named (error status, submit-argument forms for code competitions)

## Plan

- [ ] Capture: land the live CSV as the fixture (raw, unedited rows + provenance header)
- [ ] `pkg/kaggle`: add `Ref`, reconcile status constants, keep unknown-status non-fatal; table tests
- [ ] `cmd/fake-kaggle`: emit the live schema via the shared `FormatSubmissionsCSV`
- [ ] Atlas: update the honesty caveat + status vocabulary note

## Log

### 2026-07-28

Filed from the kbench rogii-v2 session (the live-Kaggle work deferred to the operator per kbench#1).
Evidence is a real authenticated capture against an active competition, including both COMPLETE and
PENDING rows. Blast radius characterized before filing: no current crash — the parser's
header-by-name lookup, opaque `Status`, and score-based `LatestScored` all contain the divergence;
the cost is a fake that teaches wrong shapes, three misleading exported constants, and a dropped
submission id. Procedural knowledge from the same session (CLI forms, kernel metadata, server-side
gotchas) landed separately as the `kaggle-base` skill in `construct/local/base/`.
