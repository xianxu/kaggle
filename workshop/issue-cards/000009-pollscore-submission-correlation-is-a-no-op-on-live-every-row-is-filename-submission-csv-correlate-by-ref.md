---
id: 000009
status: open
created: 2026-07-28
updated: 2026-07-28
estimate_hours:
github_issue:
---

# pollScore submission correlation is a no-op on live (every row is fileName=submission.csv) — correlate by ref

## Problem

`internal/submit/submit.go:69` decides "is the newest row the submission we just uploaded?" by
comparing file names:

```go
// subs[0] is the newest = the one we just uploaded (unless it hasn't
// registered yet, or a concurrent submit raced in — in both cases keep
// polling rather than report a wrong score).
if len(subs) > 0 && (wantFile == "" || subs[0].File == wantFile) {
```

**On live Kaggle that predicate is true for every row.** All 18 rows of the first live capture
(`workshop/captures/submissions-live-2026-07-28.csv`, kaggle#8) carry `fileName=submission.csv` —
expected, not an artifact: the name is chosen by the submitter and essentially everyone uses
`submission.csv`, and for a **code competition** it is the kernel's fixed output name, so it is
*always* identical across submissions.

**Concrete failure.** Submit → the first poll fires before our row registers (Kaggle is eventually
consistent, exactly the case the guard was written for) → `subs[0]` is the *previous* submission,
already `SubmissionStatus.COMPLETE` with a score → `newest.Scored()` is true → we return **a previous
run's score**, and the caller writes it to `metrics.json` / `submission.json` as this run's result.
Silent, and wrong in the direction that looks plausible.

Same failure class as kaggle#8's divergences 1–2 (a guard that passes every fake-backed test and is
dead on live), and **invisible to the fake by construction**: `cmd/fake-kaggle/main.go:188` gives its
prior row the distinguishing filename `prior.csv`, a value real Kaggle never produces, and
`internal/submit/submit_test.go:66-71` is precisely the test that "proves" the correlation works —
using that unreachable filename.

`atlas/kaggle-layer.md:55` currently documents the correlation as correct; it must be corrected here.

Found by the `sdlc change-code` plan-quality judge while reviewing kaggle#8 (finding F1), against the
capture that issue landed. Not folded into #8: that issue is schema reconciliation, this is a
behavior redesign of the poll loop, and the correct correlation key (`Ref`) is a **deliverable of #8**
— hence `deps: [kaggle#8]`.
