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

## Chunk 2: M2 — the integration (step-types + e2e) — DETAILED (2026-07-02, post-`start-plan`, digest of the metis contract)

**Goal:** the two step-types run under the **real built `metis` binary** against the fake, producing a typed `Submission` + public score — the kaggle half of the Titanic thread, hermetic.

> **Metis-contract facts driving this detail (from `metis/{cmd/metis/exec.go,main.go,run.go},metis/io.py,atlas/experiment.md`):**
> 1. **Steps are committed *executable* files, NOT compiled/lowered binaries.** metis's `steps/metis/*` are hand-authored `100755` bash wrappers (`ROOT=$(cd "$(dirname "$0")/../.." && pwd); exec uv run --project "$ROOT" python -m metis.steps.X`). There is **no** Makefile/codegen lowering. → kaggle ships committed `steps/kaggle/{download,submit}` wrappers that `exec go run` the `cmd/*` entrypoint. **Task 4 is no longer a build system — it's two 4-line scripts.**
> 2. **Step path:** `metis run` searches `$METIS_STEP_PATH` (colon-sep, `filepath.SplitList`) if set, else `<repo-root>/steps`. **No `--step-path` flag.** → the e2e sets `METIS_STEP_PATH` to include kaggle's `steps/` (+ the test's `make-submission` steps dir). (**Decision B** confirmed: env, explicit.)
> 3. **cwd == step dir; contract is env-driven.** The runner sets `cwd = $METIS_RUN_DIR/<step-id>` AND the 5 `METIS_*` env vars (all absolute). Python `io.step_context()` `_require_env`s all 5. → `stepio.Context()` **requires** the vars it uses (errors if empty), reading paths from **env, not cwd** — this is what makes the drift guard real (§ drift note below).
> 4. **`run.json`** (`pkg/experiment.Run`) at `<expDir>/runs/<runID>/run.json`: `{id,experiment,seed,started,finished,status,metrics{name:number},artifacts[]}`. `Metrics` is a flat merge of every step's `metrics.json`; `Artifacts` are `<step-id>/<file>` slash-paths (with.json + metrics.json excluded at top level). Non-zero step exit → `status:"failed"`, run halts, ledger still written.
> 5. **Upstream-artifact convention:** a downstream step names the **upstream step's id** as a `with` value; the *filename* is a convention of the step-type pair (metis: `folds: split` → reads `$METIS_RUN_DIR/split/folds.json`). → kaggle `submit` takes `with.submission: <upstream-id>` and reads `$METIS_RUN_DIR/<id>/submission.csv` (ARCH-DRY: same convention as metis, not a bespoke path scheme).

**Two deviations from the M1-era sketch, both simplifications (call out at `change-code`):**
- **No cross-repo go.mod changes (drop sketch Task 0).** Under **Decision A2**, `stepio` declares the contract strings locally and kaggle production code imports **zero** metis Go packages (steps read only `with.json`, never the experiment types — confirmed by the digest). The e2e drives the *metis binary* as a **subprocess**, built via `go build -C <sibling metis> ./cmd/metis`, which resolves metis's own `replace ../ariadne` inside the metis module — so **kaggle stays a standalone module**. Adding `require`+`replace` for metis/ariadne (the sketch's Task 0) would be dead go.mod surface that `go mod tidy` fights (can't blank-import a `package main`). *Simplicity-First.*
- **Task 4 (build/lower) collapses** into writing the two committed `steps/kaggle/*` wrappers (fact 1).

### Task 1 — `internal/stepio` (Go step-side contract reader) — **Decision A2**

**Files:** create `internal/stepio/stepio.go`, `internal/stepio/stepio_test.go`.

- [ ] **Step 1 (red):** `stepio_test.go` — table/subtests against a `t.TempDir()` with the `METIS_*` env set (mirrors metis `test_steps` / `env-dump`):
  - `Context()` with all vars set → returns the resolved `Context{StepDir,RunDir,StepID}` (+ optional ExpDir/Seed); with **`METIS_STEP_DIR` unset → error** naming the var (the `_require_env` analog — this *is* the drift-guard unit encoding).
  - `ReadWith(ctx, &T)` unmarshals `$STEP_DIR/with.json` into a struct (write a fixture `with.json`).
  - `WriteMetrics(ctx, map[string]float64{"public_score":0.8})` writes `$STEP_DIR/metrics.json` as flat JSON the runner's `readMetrics` accepts (assert it round-trips + is `map[string]float64`-shaped).
  - `UpstreamPath(ctx,"make-submission","submission.csv")` == `filepath.Join(RunDir,"make-submission","submission.csv")`.
  - `OutPath(ctx,"submission.json")` == `filepath.Join(StepDir,"submission.json")`.
- [ ] **Step 2:** run → FAIL.
- [ ] **Step 3 (green):** implement. Declare the contract strings **locally** with a provenance header comment → `metis/atlas/experiment.md` `### Step-executable contract` (the M2/M3 surface) as the authoritative prose, and a doc-note: *"drift is caught by the M2 e2e — it drives the real `metis` binary, which sets these exact `METIS_*` names; a rename there makes `Context()` fail the step and the run."* `Context()` requires `METIS_STEP_DIR`/`METIS_RUN_DIR`/`METIS_STEP_ID` (errors if empty); `ExpDir`/`Seed` read best-effort (kaggle steps don't consume them, so not required — honest coupling to only the surface used). `WriteMetrics` takes `map[string]float64` (the runner unmarshals into exactly that; a non-numeric value would fail its `readMetrics`).
- [ ] **Step 4:** run → PASS.
- [ ] **Step 5: Commit** — `#1 M2: internal/stepio — Go step-side metis contract reader (Decision A2, local consts + drift-guard)`.

> **Drift-guard (M1-review item, resolved):** the guard is real because (a) `stepio` reads `METIS_*` from **env** and requires them — never falls back to cwd — and (b) the e2e (Task 5) runs the **actual `metis` binary**, which emits those exact names. If metis renamed e.g. `METIS_RUN_DIR`, `submit.UpstreamPath` would resolve against an empty base / `Context()` would error → the step exits non-zero → `run.json.status:"failed"` → e2e RED. It is NOT `stepio`'s own consts echoed back to itself. The `Context()`-errors-on-missing-var unit test encodes the (a) half; the e2e encodes the (b) half.

### Task 2 — `kaggle/download` step-type

**Files:** create `cmd/kaggle-download/main.go`, `cmd/kaggle-download/main_test.go`.

- **Behavior:** read `with.competition` (a `kaggle.Competition`) via `stepio.ReadWith`; `Validate()`; `kagglecli.New().Download(slug, ctx.StepDir)` → yields `<StepDir>/<slug>.zip` (fake + real CLI both emit a zip); **unzip** the zip into `ctx.StepDir` and **remove the zip**, so the artifacts metis records are the loose data files (`train.csv`/`test.csv`) — the *download half* of an Adapter (kbench's `adapt` consumes loose files; **record this shape in atlas**). No `metrics.json` (download emits no metric).
- **Unzip seam:** a small `unzip(src, destDir)` helper local to `cmd/kaggle-download` (IO glue; `archive/zip`). YAGNI on sharing until a second unzipper exists.
- [ ] **Step 1 (red):** `main_test.go` — build `fake-kaggle` into `t.TempDir()`; set `KAGGLE_CLI=<fake>`, `KAGGLE_FAKE=1`, `KAGGLE_FAKE_STATE=<tmp>`, and the `METIS_*` env for a temp step dir; write `with.json` (`{"competition":{"slug":"titanic"}}`); run `main()` (call an extracted `run(env) error`, not `os.Exit`, so it's testable). Assert: `train.csv`+`test.csv` exist in the step dir, **no `*.zip` remains**, exit nil. Missing `competition.slug` → error.
- [ ] **Step 2:** run → FAIL. **Step 3 (green):** implement (`run()` returns error; `main` maps to `os.Exit(1)` + stderr). **Step 4:** run → PASS.
- [ ] **Step 5: Commit** — `#1 M2: kaggle/download step-type (auth → CLI download → unzip loose data artifacts)`.

### Task 3 — `kaggle/submit` step-type (async poll)

**Files:** create `cmd/kaggle-submit/main.go`, `cmd/kaggle-submit/main_test.go`.

- **Behavior:** read `with.competition` + `with.submission` (upstream step id); resolve the CSV at `stepio.UpstreamPath(ctx, w.Submission, "submission.csv")`; `CLI.Submit(slug, csv, msg)`; then **poll** `CLI.Submissions(slug)` → `kaggle.ParseSubmissions` → `kaggle.LatestScored` with **bounded retries** (`maxAttempts`, `delay`), the retry loop factored as `pollScore(sub Submissions-fn, max int, sleep func(int)) (kaggle.Submission, bool, error)` with an **injected `sleep`** (ARCH-PURE/controllable-time: tests pass a no-op, `main` passes `time.Sleep(delay)`) so the loop is unit-testable with zero wall-clock. On scored: write `submission.json` (the scored `kaggle.Submission`, `Competition` filled) via `stepio.OutPath` + `WriteMetrics{"public_score": *s.PublicScore}`, exit 0. **Timeout contract:** retries exhausted still-unscored → write `submission.json{status:pending}` (debug aid) and **exit non-zero** (submit's purpose is a score; unscored == failed → runner records `status:"failed"`).
- **Config:** `maxAttempts` (default e.g. 30) + `delay` (default e.g. 5s) from env (`KAGGLE_SUBMIT_MAX_ATTEMPTS`/`KAGGLE_SUBMIT_DELAY`), so real Kaggle's slow scoring is tunable and the test drives it fast. The fake's `KAGGLE_FAKE_SCORE_AFTER=1` → poll #1 pending, #2 scored, so the loop **provably iterates ≥1**.
- [ ] **Step 1 (red):** `main_test.go` — (a) fake wired as in Task 2 + an upstream `make-submission`-style dir containing `submission.csv` under `RunDir`; inject `sleep=no-op`; run `run()` → `submission.json` present + `Scored()`, `metrics.json.public_score` present, exit nil, **and assert the poll iterated** (e.g. `KAGGLE_FAKE_SCORE_AFTER=1` needs ≥2 `submissions` calls — assert via the fake's poll counter or a scored result that could only come after a pending). (b) `maxAttempts=1` with `KAGGLE_FAKE_SCORE_AFTER=5` → `run()` returns non-nil error, `submission.json` exists with `status:pending`, no `public_score`.
- [ ] **Step 2:** run → FAIL. **Step 3 (green):** implement. **Step 4:** run → PASS.
- [ ] **Step 5: Commit** — `#1 M2: kaggle/submit step-type (submit + bounded async-score poll, injected clock)`.

### Task 4 — committed step wrappers (no build system)

**Files:** create `steps/kaggle/download`, `steps/kaggle/submit` (both `chmod 755`, git-tracked as executable — mirror metis's committed-wrapper pattern, ARCH-DRY on the step-wrapper shape).

```bash
#!/usr/bin/env bash
# kaggle/download — step-type wrapper. The runner invokes with cwd == the step dir
# and the METIS_* env set (absolute). Resolve the kaggle repo root from $0 and hand
# off to the Go entrypoint; the step reads paths from METIS_* env, not cwd, so
# `go run -C "$ROOT"` (module root) is correct. See metis/atlas/experiment.md.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
exec go run -C "$ROOT" ./cmd/kaggle-download "$@"
```
(submit wrapper identical, `./cmd/kaggle-submit`.) **Verify `.gitignore` doesn't ignore `steps/`** and `git add` preserves mode 755 (`git ls-files -s steps/kaggle` → `100755`). *At implementation, spot-check that `go run -C "$ROOT" ./cmd/... ` under a foreign cwd (the run dir, outside the kaggle module) resolves the module — it does, `-C` chdirs go before module resolution; the program then reads absolute `METIS_*` paths so its cwd is irrelevant.*

- [ ] **Step 1:** write both wrappers, `chmod +x`. **Step 2:** smoke-check one directly: `METIS_STEP_DIR=… METIS_RUN_DIR=… METIS_STEP_ID=… KAGGLE_CLI=<fake> KAGGLE_FAKE=1 KAGGLE_FAKE_STATE=… ./steps/kaggle/download` writes loose data. **Step 3: Commit** — `#1 M2: committed steps/kaggle/{download,submit} go-run wrappers`.

### Task 5 — hermetic e2e under the real `metis` binary (the issue Done-when)

**Files:** create `testdata/experiment/kaggle-thread.md`, `testdata/steps/test/make-submission` (executable stub), `e2e_test.go` (package-level, e.g. `cmd/kaggle-submit/e2e_test.go` or a top-level `e2e/`).

- **Data-flow (review C1):** `submit` needs `submission.csv` as a real **upstream artifact** under `$METIS_RUN_DIR/<id>/`; `download` produces competition *data*, not a submission. So a **3-step** experiment `download → make-submission → submit`, where `make-submission` (a tiny committed bash stub in `testdata/steps/test/make-submission`, reusing metis's `test/echo` shape) writes a fixed `submission.csv` into its step dir. *(In kbench's real thread, kbench's own submission-producing step plays this role — the stub stands in here.)*
- **Experiment md** (frontmatter per metis `toy-pipeline.md`):
  ```yaml
  type: experiment
  id: kaggle-thread
  seed: 42
  status: active
  steps:
    - id: download
      uses: kaggle/download
      with: {competition: {slug: titanic, metric: accuracy}}
    - id: make-submission
      uses: test/make-submission
      needs: [download]
    - id: submit
      uses: kaggle/submit
      needs: [make-submission]
      with: {competition: {slug: titanic}, submission: make-submission, message: "e2e"}
  ```
- **Harness:** `go build -C <siblingMetis> -o <tmp>/metis ./cmd/metis` (resolve `<siblingMetis>` = `<kaggleRoot>/../metis`; `t.Skip` if absent — like metis's uv-absent skip). Also `go build` the `fake-kaggle`. Copy the experiment md into a temp expDir. `exec.Command(metisBin,"run","--run","run-e2e", <exp>)` with `cmd.Env` = os.Environ + `METIS_STEP_PATH=<kaggleRoot>/steps:<testdataStepsDir>` + `KAGGLE_CLI=<fake>` + `KAGGLE_FAKE=1` + `KAGGLE_FAKE_STATE=<tmp>` + `KAGGLE_FAKE_SCORE_AFTER=1` + `KAGGLE_SUBMIT_MAX_ATTEMPTS=5` + `KAGGLE_SUBMIT_DELAY=0`. Read `<expDir>/runs/run-e2e/run.json`.
- **Asserts:** `run.Status=="ok"`; `run.Metrics["public_score"]>0`; `submit/submission.json` exists on disk + parses to a `Scored()` `kaggle.Submission`; `download/train.csv` exists (loose, not a zip). **This is the issue Done-when** (minus live Kaggle — the fake faithfully stands in; state that at close, don't claim live-verified).
- [ ] **Step 1 (red):** write the experiment md + `make-submission` stub + e2e test → run, expect RED (steps not yet found / wired). **Step 2 (green):** iterate to green (this is the integration-debug loop — `go run -C` resolution, step-path, env). **Step 3:** `go test ./...` fully green. **Step 4: Commit** — `#1 M2: hermetic e2e — download→make-submission→submit under real metis run + fake kaggle`.

### Task 6 — atlas + milestone-close M2 + issue close

- [ ] Update `atlas/`: a `## Surface (M2)` covering `internal/stepio` (the metis contract seam + local-consts/Decision-A2 + drift-guard), the two step-types + the **loose-files download shape** (kbench's `adapt` contract) + the **async-scoring poll + timeout** contract, and the `steps/kaggle/*` go-run wrapper convention. Keep `atlas/index.md` linking every file.
- [ ] `go test ./...` + `go vet ./...` green (paste output into `--verified`).
- [ ] `sdlc milestone-close --issue 1 --milestone M2` (measured `--actual` via `sdlc actual`; the auto-dispatched boundary review runs here — fix Critical/Important before crossing; log the `Review-Verdict:`).
- [ ] `sdlc close --issue 1` — `--verified` = e2e output + the explicit *fake-verified, live-deferred* honesty; atlas updated (no `--no-atlas`).

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

## Revisions

### 2026-07-01 — M1 shipped + boundary review (FIX-THEN-SHIP → fixed)
- **Exported names.** The Core-concepts tables/prose above wrote `credentialSource`/`parseSubmissions`/`latestScored` lowercase; the code exports them (`CredentialSource`/`ParseSubmissions`/`LatestScored`, plus `FormatSubmissionsCSV`) because `internal/kagglecli` and `cmd/fake-kaggle` consume them across package boundaries. The design is otherwise as written.
- **Parser hardened (review Important).** `ParseSubmissions` no longer fails the whole list on one non-numeric `publicScore` cell — an unparseable score degrades that row to unscored (`nil`), so `LatestScored` still finds the newest validly-scored row. Added tests for both error branches (bad-float → row unscored; missing `fileName` column → error). Also dropped `csv.Reader.Comment='#'` from the production parser (it was shaping behavior off the authored fixture) — the test now strips the fixture's provenance header itself, keeping the parser fixture-agnostic.
- **M2 note from the review:** confirm the e2e actually fails on a renamed `METIS_*` var (reads the real metis-emitted env, not `stepio`'s own consts echoed back) — else Decision A2's "drift caught by the e2e" guard is illusory. Detail this when planning M2.

### 2026-07-02 — M2 detailed (post-`start-plan`; metis-contract digest)
- **Chunk 2 upgraded sketch → detailed** after re-reading the metis step contract (`cmd/metis/{exec,main,run}.go`, `metis/io.py`, `atlas/experiment.md`). The task-by-task plan now pins the exact `METIS_*` env surface, the `with.json`/`metrics.json` filenames, the `run.json` shape, and the upstream-artifact id-naming convention (kaggle reuses metis's `folds: split` convention as `submission: <upstream-id>` → `submission.csv`, ARCH-DRY).
- **Deviation 1 — drop sketch Task 0 (cross-repo go.mod).** The digest confirms steps read **only** `with.json`, never metis's experiment types, so under Decision A2 kaggle production code imports **zero** metis packages → **kaggle stays a standalone module**. The e2e drives the metis *binary* as a subprocess built via `go build -C ../metis` (resolves metis's own `../ariadne` replace internally). Adding `require`+`replace` for metis/ariadne would be dead surface `go mod tidy` fights (a `package main` can't be blank-imported to pin it). Net: **no go.mod change in M2.** (Simplicity-First.)
- **Deviation 2 — Task 4 is not a build system.** metis has no lowering/codegen; its `steps/metis/*` are committed `100755` wrappers. kaggle mirrors this: two committed `steps/kaggle/*` bash wrappers that `exec go run -C "$ROOT" ./cmd/kaggle-<type>` (binaries stay out of git — metis `/bin/` lesson). ARCH-DRY on the wrapper shape.
- **Drift-guard resolved (M1-review item):** `stepio.Context()` **requires** the `METIS_*` vars from env (never cwd-fallback) — a `Context()`-errors-on-missing-var unit test encodes the local half; the e2e runs the **real `metis` binary** (which emits the exact names) so a rename → step non-zero exit → `run.json.status:"failed"` → e2e RED. Guard is genuine, not a self-echo.
- **Decision B confirmed:** e2e sets `METIS_STEP_PATH` (colon-sep) to include kaggle's `steps/` + the test's `make-submission` dir (metis exposes no `--step-path` flag; env is the seam).
