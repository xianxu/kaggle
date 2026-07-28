---
id: 000009
status: open
deps: [kaggle#8]
github_issue:
created: 2026-07-28
updated: 2026-07-28
estimate_hours:
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

## Spec

Correlate by submission **identity**, not by file name.

- `Submission.Ref` (the numeric submission id) arrives with kaggle#8 and is the stable handle.
- **Open design question, settle first with one cheap probe:** does `kaggle competitions submit` emit
  the new submission's `ref` on stdout? If yes, capture it at submit time and poll for that exact ref.
  If no, use a **ref snapshot**: read `submissions` immediately before submitting, then treat the
  first row whose `ref` is absent from that snapshot as ours. The snapshot form needs no new CLI
  surface and tolerates eventual consistency; prefer it unless submit's output makes the direct form
  trivial.
- `wantFile` correlation is removed or demoted to a tie-breaker — never again the sole discriminator.
- **Fake fidelity (ARCH-MOCK):** `cmd/fake-kaggle` must name its prior row `submission.csv` like real
  Kaggle, and mint monotonic `ref`s. Doing so makes `internal/submit/submit_test.go:66-71` **fail
  first** against current code — that failing test is the correct TDD entry point for this issue, and
  kaggle#8 deliberately leaves the fake's `prior.csv` in place (documented as unfaithful) so the
  failure lands here rather than blocking a schema change.

## Done when

- [ ] A test reproduces the wrong-score path: prior scored row + our row not yet registered ⇒ current code returns the prior score (RED), fixed code keeps polling (GREEN)
- [ ] Correlation keys on `ref`; the chosen mechanism (submit-output vs snapshot) recorded with the probe evidence that settled it
- [ ] `cmd/fake-kaggle` emits `submission.csv` for every row + monotonic refs; no test depends on a distinguishing filename
- [ ] Timeout/error records still identify which submission they refer to (`ref` in `submission.json`)
- [ ] `atlas/kaggle-layer.md:55` corrected — the documented correlation matches the code
- [ ] `go test ./...` green

## Plan

- [ ] Probe: capture `kaggle competitions submit` stdout on a live submission; decide submit-output vs ref-snapshot
- [ ] RED test in `internal/submit` for the previous-score path
- [ ] Implement ref correlation in `pollScore`; thread `ref` through `SubmitAndPoll`
- [ ] Fake: `submission.csv` filenames + monotonic refs; update the tests that leaned on `prior.csv`
- [ ] Atlas correction

## Log

### 2026-07-28

Filed from kaggle#8's plan review (finding F1). Evidence: the 18-row live capture, uniform
`fileName=submission.csv` (`awk -F, 'NR>1{print $2}' … | sort | uniq -c` → `18 submission.csv`).
Severity is "silently reports another run's score"; reachability requires a poll landing in the
eventual-consistency window with a previously-scored submission present — likely why it has not been
observed in practice yet. Blocked on #8 only for the `Ref` field.
