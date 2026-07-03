---
id: 000002
status: codecomplete
deps: []
github_issue:
created: 2026-07-02
updated: 2026-07-02
estimate_hours: 0.58
started: 2026-07-02T17:37:24-07:00
actual_hours: 0.13
---

# fake-kaggle: fixture-driven download (serve real competition columns for full-thread e2e)

## Problem

`cmd/fake-kaggle`'s `competitions download` emits a hardcoded `PassengerId,Survived` two-row stub (`main.go` `doDownload`). That's enough for the kaggle layer's own e2e (which only needs the download→unzip plumbing to produce loose files), but it can't drive a **full three-layer thread**: a downstream competition adapter (kbench's `titanic/adapt`) needs the **real competition columns** (`Pclass,Sex,Age,SibSp,Parch,Fare,…`), not a two-column stub. Consumers doing a hermetic end-to-end run therefore have no way to make the fake serve realistic competition data.

## Spec

Make the fake's `download` **fixture-driven and competition-agnostic**: when `KAGGLE_FAKE_DATA_DIR` is set, pack every regular file in that dir into the download zip (so a consumer points it at a committed fixture with the real column shapes for *any* competition). When it's unset, keep the current minimal stub — so the kaggle layer's existing e2e (which only needs a zip to unzip) is untouched (back-compat).

- One small, generic env seam — `KAGGLE_FAKE_DATA_DIR` — mirroring the existing `KAGGLE_FAKE_*` tuning vars. No competition-specific logic in the fake.
- The fake stays a faithful process-level model (per "model external services"): it still produces a real `.zip` the real unzip path handles; only the *contents* become fixture-sourced.
- **Layer discipline:** this is a kaggle-layer capability (the fake lives here), so it rides under a kaggle issue, not under the kbench issue that consumes it (kbench#1 M2 deps on this).

## Done when

- With `KAGGLE_FAKE_DATA_DIR=<dir>`, `fake-kaggle competitions download -c <slug> -p <dest>` produces `<dest>/<slug>.zip` whose entries are exactly the files in `<dir>` (byte-for-byte), so unzip yields the real competition columns.
- With `KAGGLE_FAKE_DATA_DIR` unset, download still emits the existing `PassengerId,Survived` / `PassengerId` stub (existing kaggle e2e unaffected — `go test ./...` stays green).
- An empty/missing `KAGGLE_FAKE_DATA_DIR` is a clear error, not a silent empty zip.

## Plan

- [x] Add `downloadFiles()` — returns the zip contents: `KAGGLE_FAKE_DATA_DIR` files (competition-agnostic) if set, else the current stub; error on missing/empty dir. `doDownload` calls it. TDD: table test both branches (fixture dir → contents match; unset → stub) + the empty/missing-dir error.
- [x] Update `atlas/` — note the `KAGGLE_FAKE_DATA_DIR` seam on the fake's surface sketch.
- [x] `sdlc close --issue 2` (single atomic boundary — no milestones).

## Estimate

```estimate
model: estimate-logic-v3.1
familiarity: 1.0
design-buffer: 0.30
item: smaller-go-module      design=0.1 impl=0.15
item: atlas-docs             design=0.0 impl=0.10
item: milestone-review       design=0.0 impl=0.20
total: 0.58
```

*Produced via `brain/data/life/42shots/velocity/estimate-logic-v3.1.md` against `baseline-v3.1.md`. Method A only.* A ~40-line back-compatible extension of an existing Go command (`smaller-go-module`) + the atlas seam note + the single close-boundary review. `design-buffer: 0.30` (no separate durable plan doc — the issue Spec/Plan is the design; v3.1 rule #4). No live-API surface (the fake IS the model), so no `real-api-discovery`.

## Log

### 2026-07-02
- 2026-07-02: closed — fixture-driven fake download: KAGGLE_FAKE_DATA_DIR serves competition files byte-for-byte (top-level; subdirs skipped), unset → PassengerId,Survived stub (back-compat), set-but-missing/empty → error not silent empty zip. go test ./... green (8 fake tests incl 5 new; existing stub-reliant e2e passes) + go vet clean.; review verdict: SHIP

Created as the D1 vehicle for kbench#1 M2's hermetic full-thread e2e (see kbench#1 plan, Open decision D1). The operator chose the mocked/fake path over the metis-portion-only fallback. Kept in the kaggle layer (not kbench#1's number) per layer discipline — the fake is kaggle-owned, and a fixture-driven download is reusable for every future competition e2e, not just Titanic.

**Implemented (TDD).** Added `downloadFiles(dir string) (map[string]string, error)` in `cmd/fake-kaggle/main.go`; `doDownload` reads `os.Getenv("KAGGLE_FAKE_DATA_DIR")` at the boundary (ARCH-PURE nicety the change-code plan-judge suggested) and passes it in. When set: serve every top-level regular file byte-for-byte (subdirs skipped — flat fixtures); missing/empty dir → error. When "" (unset): the existing `PassengerId,Survived` stub (back-compat). Honored the plan-judge's three test-pinning points: (1) `KAGGLE_FAKE_DATA_DIR=""` == unset → stub (tri-state pinned so it can't flip to error and break the existing e2e); (2) a subdir in the fixture is skipped, not an error; (3) assert unzipped **bytes** equal the fixture (not just entry names), and keep pinning the stub content. **Evidence:** `go test ./...` green (8 fake tests incl. 5 new; the existing kaggle e2e that relies on the stub still passes) + `go vet ./...` clean.
