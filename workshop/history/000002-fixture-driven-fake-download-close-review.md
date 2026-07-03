# Boundary Review — kaggle#2 (whole-issue close)

| field | value |
|-------|-------|
| issue | 2 — fake-kaggle: fixture-driven download (serve real competition columns for full-thread e2e) |
| repo | kaggle |
| issue file | workshop/issues/000002-fixture-driven-fake-download.md |
| boundary | whole-issue close |
| milestone | — |
| window | 5a705d830463dba0a0ebed5441ca1fd688482aee..HEAD |
| command | sdlc close --issue 2 |
| reviewer | claude |
| timestamp | 2026-07-02T17:45:05-07:00 |
| verdict | SHIP |

## Review

Ignoring 6 permissions.allow entries from .claude/settings.json: this workspace has not been trusted. Run Claude Code interactively here once and accept the trust dialog, or set projects["/Users/xianxu/workspace/kaggle"].hasTrustDialogAccepted: true in /Users/xianxu/.claude.json.
```verdict
verdict: SHIP
confidence: high
```

A clean, tightly-scoped back-compatible extension that fully delivers the issue's purpose. `doDownload` now reads `KAGGLE_FAKE_DATA_DIR` at the IO boundary and delegates to a new `downloadFiles(dir)` that serves fixture files byte-for-byte when set, falls back to the `PassengerId,Survived` stub when unset, and errors (never a silent empty zip) on a missing or empty dir. All three Done-when criteria are pinned by tests, the full suite is green (`go test ./...` all ok, `go vet` clean, 6 download tests pass including the end-to-end unzipped-bytes assertion), and the atlas surface sketch + env-var table are updated in the same range. Nothing blocks SHIP; only minor polish notes below.

**1. Strengths**
- Env read lifted to the boundary (`main.go:77`) so `downloadFiles(dir string)` takes the dir as a parameter — testable without touching the environment (`TestDownloadFilesStubWhenUnset`, `TestDownloadFilesServesFixtureDir` call it directly). Correct ARCH-PURE seam.
- Tri-state pinned deliberately: `""` (unset *or* empty env) → stub, kept distinct from set-but-missing → error (`main.go:99` + `TestDownloadFilesStubWhenUnset`), which guards the existing kaggle e2e from flipping to an error. This is exactly the back-compat trap and it's covered.
- The end-to-end test asserts *unzipped bytes* equal the fixture, not just entry names (`main_test.go:171`) — pins the actual point of the issue (real column shapes reach the consumer's unzip path), and the subdir-skip is asserted by count, not just presence (`main_test.go:117`).
- Consolidation, not duplication: the stub map moved out of `doDownload` into the single `downloadFiles` source (ARCH-DRY pass — the only "repeat" is the test re-stating the stub constant, which is intentional pinning).

**2. Critical findings**
None.

**3. Important findings**
None.

**4. Minor findings**
- `cmd/fake-kaggle/main.go:111` / doc comment `main.go:90-97` / error `main.go:121` all say "regular file", but the code only skips `e.IsDir()` — a symlink or FIFO is treated as a file (a symlink-to-dir would then fail `os.ReadFile` with a bare error). Benign for a flat committed fixture dir; either tighten to `e.Type().IsRegular()` or relax the wording to "non-directory entry" so comment and behavior agree.
- `cmd/fake-kaggle/main.go:114-116`: the per-file `os.ReadFile` error is returned bare, while the `ReadDir` error above it is wrapped with `KAGGLE_FAKE_DATA_DIR %q` context. Minor inconsistency — wrapping the read error with the file name would match the surrounding style.
- `os.ReadDir` includes dotfiles, so a stray `.DS_Store` in a fixture dir would be packed into the zip. Fixtures are committed so unlikely, but worth a one-line note if it ever bites.

**5. Test coverage notes**
Coverage matches the risk surface: both branches (stub / fixture), both error paths (missing dir, empty dir), subdir-skip, and the full download→zip→unzip byte path. Tests exercise real temp dirs (real IO via injected `t.TempDir()`), no mocks — appropriate for a process-level fake. The kind of bug this diff could ship (unset flipping to error, subdir causing an error, contents not byte-faithful) is each directly covered.

**6. Architectural notes for upcoming work**
- ARCH-DRY: pass. ARCH-PURE: pass — env read at boundary, `downloadFiles` does inherent filesystem IO (its job) tested with real fixtures, no heavy logic buried in IO. ARCH-PURPOSE: pass — shadow-sweep of the Done-when contract shows all three consumers-of-the-behavior delivered; the actual downstream consumer (kbench's committed fixture) is correctly a separate repo/issue per layer discipline, not an under-delivery of this issue.
- For the downstream kbench full-thread e2e: the map-iteration order in `writeZip` makes zip *byte* layout non-deterministic (pre-existing, not introduced here). Consumers unzip so it's fine — but do not add any test that hashes the raw `.zip`; assert on extracted files.

**7. Plan revision recommendations**
None — the plan's Core-concepts intent (single `downloadFiles` helper, atlas seam note, single atomic close boundary) matches the code exactly. All three Plan checkboxes are genuinely delivered.
