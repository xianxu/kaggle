# kaggle platform integration — Implementation Plan (kaggle#1)

> **For agentic workers:** Consult AGENTS.md Section 3 (Subagent Strategy) to determine the appropriate execution approach: use superpowers-subagent-driven-development (if subagents are suitable per AGENTS.md) or superpowers-executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give the kaggle layer typed Go **state** records (Competition/Submission/Leaderboard/Credentials) and the `kaggle/download` + `kaggle/submit` **step-types** the metis runner invokes — wrapping the official `kaggle` CLI for **transport** — with a **process-level fake** backing a green e2e so CI never touches live Kaggle.

**Architecture:** Split by the Spec's rule — **Go owns the STATE, the official CLI owns the TRANSPORT.** A pure `pkg/kaggle` models the records + parses the CLI's stdout (no IO); a thin `internal/kagglecli` shells an **injectable** command (`${KAGGLE_CLI:-kaggle}`) so a fake executable can replace it in tests; two small Go step-type binaries lower to `steps/kaggle/{download,submit}` and honor metis's files+subprocess step contract. The e2e drives the real `metis run` against a process-level **fake `kaggle`** CLI. Real Kaggle (live auth + async scoring) is code-complete but exercised manually by the operator / kbench#1 — this machine has no CLI or credentials, so the fake is the *verified* path.

**Tech Stack:** Go (`module github.com/xianxu/kaggle`, `replace` metis + ariadne siblings), the official `kaggle` CLI as an out-of-process dependency (injectable), the metis step contract (reused), TDD (`go test`).

---

## Core concepts

### Pure entities (the conceptual core — `pkg/kaggle`, unit-tested with NO IO)

| Name | Lives in | Status |
|------|----------|--------|
| `Competition` | `pkg/kaggle/competition.go` | new |
| `Submission` | `pkg/kaggle/submission.go` | new |
| `credentialSource` (pure auth decision) | `pkg/kaggle/credentials.go` | new |
| `parseSubmissions` (CLI stdout → []Submission) | `pkg/kaggle/parse.go` | new |
| `latestScored` (pick the scored submission) | `pkg/kaggle/parse.go` | new |

- **Competition** — the thin config identifying a competition: `Slug` (e.g. `titanic`), `Metric` (e.g. `accuracy`), optional `Deadline`. Supplied by the experiment's `with` (kbench sets it); not fetched. Pure struct + `Validate()` (non-empty slug).
  - **Relationships:** 1:N with Submission (a competition accretes submissions). Referenced by both step-types via `with.competition`.
  - **DRY rationale:** one definition of "which competition + how it's scored" that both `download` and `submit` read from `with`, instead of each step re-parsing raw slugs.
  - **Future extensions:** a `data manifest` (expected files); a fetched-metadata variant behind the same struct.
- **Submission** — one upload's durable record: `Competition`, `File`, `Message`, `SubmittedAt`, `Status` (`pending|complete|error`), `PublicScore` (`*float64` — nil until scored). Serialized as `submission.json` (a step artifact). Pure struct + `Scored() bool`.
  - **Relationships:** N:1 with Competition. Produced by the `submit` step.
  - **DRY rationale:** the single shape every layer reads a submission result from (kbench reads `PublicScore` for the leaderboard proof) — no ad-hoc score-scraping downstream. **This is where the Spec's "read the public leaderboard score" done-when is satisfied** — off `Submission.PublicScore` (via `parseSubmissions`), not a separate Leaderboard pull.
  - **Future extensions:** `PrivateScore`, `Rank`, a submissions *history* list (project `explicitly_out` for now).
- **credentialSource** — the **pure** auth decision (ARCH-PURE fix — the *IO* of reading env + statting the file lives in `internal/kagglecli`, not here): `credentialSource(username, key string, fileExists bool) (present bool, err error)` returns present when env-pair OR file is available, else a typed `ErrNoCredentials` naming both mechanisms. Table-tested with no IO. The IO layer gathers `os.Getenv` + `os.Stat(~/.kaggle/kaggle.json)` (existence only — never parse/log the secret) and calls this. Never holds the key value.
  - **Relationships:** the pure half consulted by `internal/kagglecli` before shelling the *real* CLI (the fake path needs none).
  - **DRY rationale:** one credential-decision point; the IO layer doesn't scatter env/file checks.
  - **Future extensions:** a Go REST auth token when the CLI is swapped for a native client.
- **parseSubmissions / latestScored** — pure parsers turning the `kaggle competitions submissions --csv` stdout into `[]Submission`, and selecting the newest scored one. **This is the load-bearing pure boundary** (the fragile bit — CLI output shape — is pure and table-tested, not buried in exec glue).
  - **DRY rationale:** the *only* place CLI text becomes typed state; both the `submit` step (read back the score) and any future leaderboard read reuse it.
  - **Future extensions:** a JSON-output parser variant if the CLI/`--json` shape is adopted.

> **Deferred (YAGNI): `Leaderboard`.** The Spec lists a Leaderboard record, but the walking skeleton reads *our* public score off `Submission.PublicScore` — nothing consumes a full leaderboard snapshot. Building an unused `Leaderboard` struct now is exactly the Simplicity-First smell to avoid; deferred until a `kaggle/leaderboard` pull step needs it (a natural later extension). The Spec's leaderboard-*score* purpose is met (see Submission). Flagged as a conscious scope call, not an omission.

### Integration points (where pure meets the world)

| Name | Lives in | Status | Wraps |
|------|----------|--------|-------|
| `CLI` (kaggle client) | `internal/kagglecli/cli.go` | new | the official `kaggle` CLI via `os/exec` |
| `steps/kaggle/download` | `cmd/kaggle-download/main.go` → `steps/kaggle/download` | new | metis step contract + CLI |
| `steps/kaggle/submit` | `cmd/kaggle-submit/main.go` → `steps/kaggle/submit` | new | metis step contract + CLI |
| `stepio` (step-side contract) | `internal/stepio/stepio.go` | new | the metis `METIS_*` env + `with.json`/`metrics.json` files |
| process-level fake `kaggle` | `cmd/fake-kaggle/main.go` | new | a fake of the CLI's subcommands |

- **CLI** — the injectable transport: `New()` reads `${KAGGLE_CLI:-kaggle}`; methods `Download(slug, dest)`, `Submit(slug, file, msg)`, `Submissions(slug) (stdout string)`. Each is a thin `exec.Command` wrapper returning raw bytes/err; **no parsing here** (parsing is the pure `parseSubmissions`). Runs the env-read+stat and calls `credentialSource` before a real invocation; **skips the auth precheck only on an explicit `KAGGLE_FAKE=1` signal** (NOT by string-matching the binary name — a real CLI provisioned at a full path like `.venv/bin/kaggle` must still be auth-checked).
  - **Injected into:** the two step `main`s; they call `CLI` + pure `pkg/kaggle` parsers, so step logic stays testable.
  - **Future extensions:** swap the exec impl for a native Go REST client behind the same method set — consumers unchanged (the Spec's stated end-state).
- **steps/kaggle/{download,submit}** — the metis-invoked executables. `download`: read `with.competition`, `CLI.Download` into `$METIS_STEP_DIR` (the *download half* of an Adapter — raw files land as artifacts; kbench's `adapt` turns them into a Dataset). `submit`: read `with` (`competition`, `submission_file` — an upstream artifact path under `$METIS_RUN_DIR/<id>/`), `CLI.Submit`, poll `CLI.Submissions` + `latestScored` until scored (bounded), write `submission.json` + `metrics.json{public_score}`.
  - **Injected into:** resolved by the metis runner via `uses: kaggle/download` on the step path.
  - **Future extensions:** a `leaderboard` step; retry/backoff policy for async scoring.
- **stepio** — the **Go step-side** reader of metis's contract (env + `with.json` in, `metrics.json` + artifacts out). metis today encodes this contract on the *runner* side (`cmd/metis/exec.go`) and the *Python step* side (`metis/io.py`); **there is no Go step-side library yet**, so kaggle is the first Go step-author and must read the contract. See **Decision A** for where the shared *string constants* live.
  - **Injected into:** both step `main`s.
  - **Test surface:** unit-tested against a temp step dir with the `METIS_*` env set (mirrors metis's `test_steps.py`).
- **process-level fake `kaggle`** — a real subprocess speaking the CLI's surface (`competitions download|submit|submissions`), backed by a `$KAGGLE_FAKE_STATE` dir. This is the **deliverable's fake**, not throwaway scaffolding (per the "model external services" rule); it makes the e2e hermetic. Two fidelity requirements the review surfaced:
  - **Model the async *transition*, not just the end-state.** Real Kaggle scores asynchronously (`submit` returns, the score appears on a later `submissions` poll). The fake must return `status=pending, PublicScore=nil` for the first `KAGGLE_FAKE_SCORE_AFTER` (default 1) `submissions` calls, then `status=complete` with a score — so the `submit` step's poll loop **actually iterates ≥1 time** in the e2e. A fake that returns a scored row immediately would leave the genuinely-fragile poll logic unexercised (the whole reason a process-level fake exists).
  - **Mirror the real artifact shape.** Real `competitions download` yields a **`.zip`**; the fake must write a zip (not loose files) so the download half's e2e proves the shape the live path produces. (Who unzips — the `download` step vs kbench's `adapt` — is pinned in Chunk 2 Task 2.)
  - **Injected into:** the e2e sets `KAGGLE_CLI=<built fake>` + `KAGGLE_FAKE=1`; `CLI` shells it identically to the real one.
  - **Future extensions:** a failure-injection mode (auth error, permanent scoring timeout) to test the step's error paths.

---

## Open decisions (resolve at approval / `sdlc change-code` plan-quality gate)

- **Decision A — where the step-contract *string constants* live (ARCH-DRY).** kaggle's `internal/stepio` is the first Go step-side reader; the `METIS_*` env names + `with.json`/`metrics.json` filenames are currently *string literals* in metis `cmd/metis/exec.go` (unexported) and re-stated in `metis/io.py` (Python, unavoidably separate). Two options for the Go side:
  - **A1:** metis exports the contract constants — add `metis/pkg/stepcontract` (just the `METIS_*` names + `WithFile`/`MetricsFile` consts, ~15 lines) and have `cmd/metis/exec.go` reference them; kaggle's `internal/stepio` imports it. One Go source of truth. But it's a **peer-repo edit folded under kaggle's issue number** (muddies the two trackers), and with only two Go consumers today (metis runner + kaggle) it's a rule-of-*two* extraction.
  - **A2 (now recommended):** kaggle's `internal/stepio` declares the ~5 contract strings locally, with a provenance comment pointing at metis atlas `## Surface (M3)` as the authoritative prose, and a `stepio` doc-note that drift is caught by the M2 e2e (a renamed `METIS_*` var → kaggle steps fail the run). Self-contained kaggle#1; no metis edit.
  - *Recommendation: **A2 now, promote to `metis/pkg/stepcontract` when kbench becomes the 3rd Go step-author*** (rule of three) — file that as its own small **metis** issue at that point, keeping the change in metis's tracker. The DRY exposure meanwhile is 5 opaque constant strings guarded by an e2e, not a duplicated *model*. (If the operator prefers A1's single-source-now, it's a clean small metis change — say so at approval.)
- **Decision B — cross-layer step resolution (carried from metis#1's close review).** `metis run` resolves `steps/<layer>/<steptype>` by searching its **step path**; kaggle's steps live in the kaggle repo, and kbench experiments run from the kbench workspace. For *this* issue's e2e we set `$METIS_STEP_PATH` to include kaggle's `steps/` dir (sufficient + explicit). The general **layered precedence** (metis + kaggle + kbench step dirs auto-composed) is a separate metis concern — recommend filing a small **metis follow-up issue** rather than solving it here. (Confirm: file it now vs defer to kbench#1.)
- **Decision C — real `kaggle` CLI provisioning.** Out of this issue's *verified* scope (no CLI/creds here; fake is tested). When the operator runs it live, the CLI is a Python tool; recommend provisioning hermetically via **uv** (consistent with metis) or an `ensure-kaggle` bootstrap step — track under the ariadne#161 bootstrap family, not here.

---

## Milestone shape

The single `- [ ] M1` in the issue is two genuine review boundaries; split (issue `## Plan` updated via `## Revisions`):

- **M1 — the kaggle library:** `pkg/kaggle` state + pure parsers + `internal/kagglecli` + the process-level fake, with unit tests **and** a client-vs-fake integration test. No metis dependency yet. Boundary review.
- **M2 — the integration:** `internal/stepio` (+ Decision A), the two step-types, and the e2e under real `metis run` against the fake; atlas; issue close. Boundary review.

---

## Chunk 1: M1 — the kaggle library (state + CLI client + fake)

### Task 1: Go module

**Files:**
- Create: `go.mod`

- [ ] **Step 1: Init a standalone module.** M1 imports NOTHING from metis/ariadne (pure `pkg/kaggle` + `internal/kagglecli` + the fake are self-contained); the sibling `replace`s are added in **M2 Task 1** when `internal/stepio` first imports across repos. So M1's go.mod is minimal (match the siblings' pinned toolchain — they use `go 1.26.3`):

```
module github.com/xianxu/kaggle

go 1.26.3
```

- [ ] **Step 2:** `go mod tidy` — a no-op here (no external deps yet); it must not add or strip anything.
- [ ] **Step 3: Verify** `go build ./...` is clean (no packages yet → trivially green).
- [ ] **Step 4: Commit** — `#1 M1: Go module skeleton (kaggle; standalone until M2 cross-repo imports)`.

> **M2 Task 1 will extend go.mod** with `replace github.com/xianxu/metis => ../metis` **and** `replace github.com/xianxu/ariadne => ../ariadne` (Go `replace` is non-transitive: kaggle→metis→ariadne means kaggle must declare the ariadne replace itself even though metis already does).

### Task 2: `Competition` + `credentialSource` (pure, no IO)

**Files:**
- Create: `pkg/kaggle/competition.go`, `pkg/kaggle/competition_test.go`
- Create: `pkg/kaggle/credentials.go`, `pkg/kaggle/credentials_test.go`

- [ ] **Step 1 (red):** `competition_test.go` — `Competition{Slug:""}.Validate()` returns an error; a well-formed one returns nil.
- [ ] **Step 2 (red):** `credentials_test.go` — table-driven over the **pure** `credentialSource(username, key string, fileExists bool) (present bool, err error)`: `("u","k",false)`→present,nil; `("","",true)`→present,nil; `("","",false)`→false,`ErrNoCredentials`; `("u","",false)`→false,`ErrNoCredentials` (partial env is not creds). Assert the error message names BOTH mechanisms (env pair + `~/.kaggle/kaggle.json`). **No `t.Setenv`, no temp `HOME`, no filesystem** — this is why it's pure; the env-read + `os.Stat` live in the IO layer (Task 5) and are exercised there.
- [ ] **Step 3:** Run the two tests → FAIL (undefined).
- [ ] **Step 4 (green):** implement `Competition.Validate()` and the pure `credentialSource` + exported `ErrNoCredentials` (never accepts or logs the key *value* — only whether the pair is non-empty).
- [ ] **Step 5:** Run → PASS.
- [ ] **Step 6: Commit** — `#1 M1: Competition + pure credentialSource (auth decision, IO-free)`.

### Task 3: `Submission` (pure)

**Files:**
- Create: `pkg/kaggle/submission.go`, `pkg/kaggle/submission_test.go`

(`Leaderboard` is deferred — see the YAGNI note under Pure entities.)

- [ ] **Step 1 (red):** `submission_test.go` — `Submission{}` with `PublicScore=nil` → `Scored()==false`; with a score set → `true`; JSON round-trip (`json.Marshal`→`Unmarshal`) preserves fields incl. the `*float64` score and `Status`.
- [ ] **Step 2:** Run → FAIL.
- [ ] **Step 3 (green):** implement the struct (JSON tags matching the `submission.json` artifact shape) + `Scored()`.
- [ ] **Step 4:** Run → PASS.
- [ ] **Step 5: Commit** — `#1 M1: Submission record (pure, JSON-serialized state)`.

### Task 4: `parseSubmissions` + `latestScored` (pure — the fragile boundary)

**Files:**
- Create: `pkg/kaggle/parse.go`, `pkg/kaggle/parse_test.go`
- Create: `pkg/kaggle/testdata/submissions.csv` — an **AUTHORED fixture** approximating `kaggle competitions submissions --csv` output. **Provenance:** derived from the Kaggle CLI docs, NOT captured (this machine has no CLI). Add a header comment stating this + `VALIDATE against the first live capture`. This is load-bearing honesty: the fake reuses `pkg/kaggle` for the CSV shape (good DRY), which means fake and parser **co-derive from this same unvalidated schema** and structurally cannot catch a divergence from real Kaggle's actual columns / status vocabulary / score-column name / `--csv` flag form. That gap is real and named here, not hidden by a green e2e.

- [ ] **Step 1 (red):** `parse_test.go` — table-driven over the `testdata/submissions.csv` fixture: `parseSubmissions(csv)` yields N `Submission`s with the right `File`/`Status`/`PublicScore` (including a `pending` row whose score is nil); `latestScored(subs)` returns the newest row whose score is non-nil (and an error/`ok=false` when none scored).
- [ ] **Step 2:** Run → FAIL.
- [ ] **Step 3 (green):** implement `parseSubmissions` with `encoding/csv` (header-driven column lookup — do NOT hardcode column indices; the CLI reorders) and `latestScored`. Pure, no IO.
- [ ] **Step 4:** Run → PASS.
- [ ] **Step 5: Commit** — `#1 M1: parse kaggle CLI submissions output → typed Submissions (pure)`.

### Task 5: `internal/kagglecli.CLI` (injectable transport)

**Files:**
- Create: `internal/kagglecli/cli.go`, `internal/kagglecli/cli_test.go`

- [ ] **Step 1 (red):** `cli_test.go` — (a) `New()` with `t.Setenv("KAGGLE_CLI", <tiny stub script in t.TempDir()>)`; assert `Download`/`Submit`/`Submissions` invoke the stub with the expected argv (stub records `"$@"` to a file the test reads) and return its stdout. (b) `checkCredentials()` IO wiring: this is where the env-read + `os.Stat(~/.kaggle/kaggle.json)` happen and feed the pure `credentialSource` — test with `t.Setenv` + a temp `HOME` (env-pair present → ok; empty → `ErrNoCredentials`), and assert that with `KAGGLE_FAKE=1` the precheck is skipped entirely (no creds needed).
- [ ] **Step 2:** Run → FAIL.
- [ ] **Step 3 (green):** implement `CLI{bin string}`; `New()` = `${KAGGLE_CLI:-kaggle}`; the three methods build argv (`competitions download -c <slug> -p <dest>`, `competitions submit -c <slug> -f <file> -m <msg>`, `competitions submissions -c <slug> --csv`) and `exec.Command(...).Output()`. Before a real invocation, `checkCredentials()` reads env + stats the file and calls `pkg/kaggle.credentialSource`; **skip the precheck iff `os.Getenv("KAGGLE_FAKE")=="1"`** (explicit no-auth signal — never by matching the binary name, which breaks for a real `.venv/bin/kaggle`).
- [ ] **Step 4:** Run → PASS.
- [ ] **Step 5: Commit** — `#1 M1: kagglecli — injectable ${KAGGLE_CLI} exec wrapper + credential precheck`.

### Task 6: process-level fake `kaggle`

**Files:**
- Create: `cmd/fake-kaggle/main.go`, `cmd/fake-kaggle/main_test.go`

- [ ] **Step 1 (red):** `main_test.go` — build the fake into `t.TempDir()`, `KAGGLE_FAKE_STATE=<tmp>`. (a) **download → zip:** `fake-kaggle competitions download -c titanic -p <dir>` writes a **`.zip`** into `<dir>` (mirrors the real CLI's artifact shape), and its entries are the fixture data files. (b) **async transition:** `... submit -c titanic -f sub.csv -m msg`, then the **first** `... competitions submissions -c titanic --csv` shows the row `status=pending, score empty`, and the **next** call shows `status=complete` with a deterministic `PublicScore` (default `KAGGLE_FAKE_SCORE_AFTER=1`; assert the transition explicitly). This is what forces the submit step's poll loop to iterate.
- [ ] **Step 2:** Run → FAIL.
- [ ] **Step 3 (green):** implement the fake: parse the subcommand, read/write `$KAGGLE_FAKE_STATE` (incl. a per-competition poll counter), emit `submissions` stdout via `pkg/kaggle`'s CSV shape so fake + parser can't drift (ARCH-DRY), honor `KAGGLE_FAKE_SCORE_AFTER`, and `download` emits a zip.
- [ ] **Step 4:** Run → PASS.
- [ ] **Step 5: Commit** — `#1 M1: process-level fake kaggle CLI (zip download + async pending→scored submit)`.

### Task 7: client-vs-fake integration test + milestone-close M1

**Files:**
- Create: `internal/kagglecli/integration_test.go`

- [ ] **Step 1:** integration test — build `fake-kaggle`, `t.Setenv("KAGGLE_CLI", <fake>)`, drive `CLI.Submit` then `CLI.Submissions` → `parseSubmissions` → `latestScored` returns a scored `Submission`. This is the M1 proof that the pure parsers + the exec seam + the fake compose.
- [ ] **Step 2:** `go test ./...` green; update `atlas/` (new `pkg/kaggle` + `kagglecli` + fake surface; keep `atlas/index.md` linking it).
- [ ] **Step 3:** `sdlc milestone-close --issue 1 --milestone M1` (measured `--actual` via `sdlc actual`; `--verified` = the integration test output). Fix any Critical/Important before crossing.

---

## Chunk 2: M2 — the integration (step-types + e2e) — sketch; detailed-plan when reached (re-run `sdlc start-plan`)

**Goal:** the two step-types run under real `metis run` against the fake, producing a typed `Submission` + public score — the kaggle half of the Titanic thread, hermetic.

- **Task 0 — extend go.mod** with the two sibling `replace`s (metis + ariadne, non-transitive; see M1 Task 1 note); `go mod tidy` now pulls metis into the graph. Commit.
- **Task 1 — `internal/stepio`** (**Decision A2**): Go step-side contract reader — `Context()` (resolve `METIS_STEP_DIR/RUN_DIR/STEP_ID/EXP_DIR/SEED`), `ReadWith(&T)`, `WriteMetrics(map)`, `UpstreamPath(stepID, file)`. Declare the ~5 contract strings locally with a provenance comment → metis atlas `## Surface (M3)`, and a doc-note that drift is caught by the Task-5 e2e. Unit-tested against a temp dir with the env set. *(If the operator chose A1 at approval: instead land `metis/pkg/stepcontract` as its own metis issue first, then import it.)*
- **Task 2 — `kaggle/download`** (`cmd/kaggle-download` → `steps/kaggle/download`): read `with.competition`, `CLI.Download` into `$METIS_STEP_DIR` → yields a `.zip`; **decide the unzip seam:** recommend the `download` step unzips into `$METIS_STEP_DIR` so downstream/`adapt` sees loose files (record in atlas so kbench's `adapt` knows what shape it consumes). The extracted files are the step artifacts. Test: injected fake (`KAGGLE_FAKE=1`) + temp step dir → expected files present, exit 0.
- **Task 3 — `kaggle/submit`** (`cmd/kaggle-submit` → `steps/kaggle/submit`): read `with.competition` + `with.submission_file` (upstream artifact via `UpstreamPath`), `CLI.Submit`, then **poll** `CLI.Submissions`+`latestScored` with **bounded retries** (config: max attempts + delay; the fake transitions pending→scored so the loop iterates ≥1 real time). On success write `submission.json` (scored) + `metrics.json{public_score}`. **Pin the timeout contract:** if retries exhaust while still unscored, write `submission.json{status:pending}` (for debugging) and **exit non-zero** — submit's purpose is to return a score, so an unscored run is a failed run (the metis runner records `status:failed`). Tests: (a) injected fake → poll iterates, `submission.json` scored, `metrics.json.public_score` present; (b) `KAGGLE_FAKE_SCORE_AFTER` beyond max-retries → step exits non-zero with a pending `submission.json`.
- **Task 4 — build + lower** the two `cmd/*` to executable `steps/kaggle/{download,submit}` (a `make`/script target; mirror metis's `steps/metis/*` wrapper pattern — decide: committed built binaries vs a `go run`/build-on-bootstrap wrapper; recommend a small wrapper that `go run`s or a bootstrap build, to keep binaries out of git per metis's `/bin/` gitignore lesson).
- **Task 5 — e2e:** `testdata/experiment/kaggle-thread.md` running via the built `metis` with `KAGGLE_CLI=<fake>`, `KAGGLE_FAKE=1`, and `METIS_STEP_PATH` including kaggle's `steps/` (**Decision B**). **Fix the data-flow (review C1):** `submit` needs `submission.csv` as an *upstream artifact* under `$METIS_RUN_DIR/<id>/`, but `download` produces competition *data*, not a submission. So the experiment is a 3-step **`download → make-submission → submit`**, where `make-submission` is a tiny stub producer step (in `testdata/steps/test/make-submission`) that writes a fixed `submission.csv` artifact — faithful to the `UpstreamPath` contract (a fixture is NOT an upstream artifact; a prior step must write it). *(In kbench's real thread, kbench's `titanic/submission` step plays the `make-submission` role.)* Assert `run.json` ok, `submission.json` scored, `metrics.json.public_score` present. **This is the issue Done-when** (minus live Kaggle, which the fake faithfully stands in for).
- **Task 6 — milestone-close M2 + `sdlc close --issue 1`:** atlas `## Surface (M2)` (the step-types + the `KAGGLE_CLI`/fake contract + the async-scoring poll); `sdlc actual`; `--verified`; boundary review; then whole-issue close.

### Notes for the implementer
- **ARCH-PURE:** `pkg/kaggle` (records + parsers) is pure + table-tested with zero IO; `kagglecli` + `stepio` + the step `main`s are the only IO. Don't parse CLI text inside exec glue — that's what `parseSubmissions` is for.
- **ARCH-DRY:** the fake emits the CLI CSV shape via `pkg/kaggle` so fake and parser can't drift; the step contract strings resolve per Decision A (prefer A1's single Go source).
- **ARCH-PURPOSE:** the step-types must *actually* shell a CLI and parse a real-shaped response to a typed scored Submission — the fake is a faithful process-level stand-in for live Kaggle, not a function-mock. The verified deliverable is feature + fake; the live path is code-complete, exercised by the operator/kbench#1 once credentials exist (state that explicitly at close — don't claim live-verified).

## Estimate
**3.5h** — the itemized `## Estimate` block lives in the issue (`estimate_hours: 3.5`, reconciled at `sdlc change-code`: Σdesign 1.3×1.15 + Σimpl 2.0 = 3.495). M1 = greenfield `pkg/kaggle` + `api-integration` (client+fake) + a small `real-api-discovery`; M2 = `smaller-go-module` (step-types + `stepio`) + 2 milestone-reviews. Sanity-anchored to metis#1 (est 6 / actual 3.83, similar Go+fake+wiring surface); calibration source `brain/data/life/42shots/velocity/estimate-logic-v3.1.md` flagged stale.

## References
- Project: `brain/data/project/kaggle-ml-base-layer.md`
- Issue spec: `kaggle/workshop/issues/000001-*.md`
- **metis step contract** (reuse, do not re-guess): `metis/cmd/metis/exec.go` (runner side: env + `with.json`/`metrics.json` + `<layer>/<steptype>` resolution), `metis/metis/io.py` (Python step side), `metis/atlas/experiment.md` `## Surface (M3)` (authoritative prose)
- Reuse from ariadne: `pkg/frontmatter` (if a markdown record is needed); metis `pkg/experiment` (types) if the step ever parses an experiment (it shouldn't — steps read only `with.json`)
- Carried-forward constraints: `kbench/workshop/issues/000001-*.md` `## Log`
- Official Kaggle CLI: `kaggle competitions {download,submit,submissions}` surface
