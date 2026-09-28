---
id: '000008'
status: done
started: 2026-07-28T10:24:27-07:00
created: 2026-07-28
updated: 2026-07-28
estimate_hours: 1.34
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
