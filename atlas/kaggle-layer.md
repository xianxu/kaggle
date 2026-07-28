# kaggle layer

The Kaggle **platform-integration** layer of the `kaggle-ml-base-layer` project
(chain `kbench → kaggle → metis → ariadne`). Rule: *does it touch the Kaggle
API/CLI?* → it lives here, not in metis. **Go owns the STATE; the official
`kaggle` CLI owns the TRANSPORT.**

## Surface (M1) — the kaggle library

### `pkg/kaggle` — pure state + parsers (IO-free, table-tested; ARCH-PURE)
- **`Competition`** (`competition.go`) — thin config `{Slug, Metric, Deadline}` supplied by an experiment's `with`; `Validate()` requires a slug.
- **`Submission`** (`submission.go`) — one upload's durable record `{Competition, Ref, File, Message, SubmittedAt, Status, StatusRaw, PublicScore *float64}`, serialized as `submission.json`. `PublicScore` is a pointer because Kaggle scores **asynchronously** (nil until scored). `Scored()` reports non-nil. Status vocab (**NORMALIZED**): `pending|complete|error` — the wire format is a Python enum repr (`SubmissionStatus.COMPLETE`), reconciled by `NormalizeStatus` at parse; `StatusRaw` keeps the wire string. `Ref` is Kaggle's submission id. **`complete` does NOT imply scored** — live reports COMPLETE rows with an empty `publicScore`, so `Scored()` (not `Status`) is the test. `error` is SHAPE-INFERRED, never captured (kaggle#8).
- **`ParseSubmissions` / `LatestScored` / `FormatSubmissionsCSV`** (`parse.go`) — the single CLI-text↔typed-state boundary. Header-driven CSV parse (column-by-name, order-independent) that also **normalizes the status vocabulary** — this is the one CLI-text↔typed-state boundary, so it is where Kaggle's wire spelling is reconciled, which is what keeps every downstream comparison correct without a call-site sweep. The parser is fixture-agnostic: it does NOT skip `#` comment lines (the fixture's provenance header is stripped by `stripComments` in `parse_test.go`). `LatestScored` returns the newest scored row (Kaggle lists newest-first). Format is the inverse, used by the fake so fake+parser share **one** schema (`submissionsCSVHeader`) — ARCH-DRY.
  - **Deferred (YAGNI):** `Leaderboard` — the public-score purpose is served off `Submission.PublicScore`; a full leaderboard record waits for a `kaggle/leaderboard` step.

### `internal/kagglecli` — the thin IO seam (ARCH-PURE boundary)
- **`CLI`** wraps `${KAGGLE_CLI:-kaggle}` (injectable). Methods `Download(slug,dest)`, `Submit(slug,file,msg)`, `Submissions(slug) → raw --csv stdout`. **No parsing** here (that's `pkg/kaggle`). **Auth is delegated to the wrapped CLI** — it owns its own credentials (OAuth `kaggle auth login`, `KAGGLE_API_TOKEN`, `~/.kaggle/access_token`, or legacy `~/.kaggle/kaggle.json`) and emits its own error on missing creds, which surfaces through `wrap()`. We do **not** pre-check creds: mirroring the CLI's evolving auth rules drifted and false-negatived valid setups (kaggle#3). `ARCH-DRY`.

### `cmd/fake-kaggle` — process-level fake (the deliverable's fake, not scaffolding)
A real subprocess speaking `competitions {download, submit, submissions}`:
- `download` writes a real-shaped **`.zip`** into `-p`. **Fixture-driven** (kaggle#2): if `KAGGLE_FAKE_DATA_DIR` is set, the zip carries every top-level file in that dir byte-for-byte (competition-agnostic — real column shapes for a full-thread e2e); unset → a minimal `PassengerId,Survived` stub (back-compat). A set-but-missing/empty dir errors (never a silent empty zip).
- `submit` records state **and mints a monotonic `ref`**; `submissions` echoes it and models the async **transition** — `pending` for the first `KAGGLE_FAKE_SCORE_AFTER` (default 1) polls, then `complete`+scored — so a consumer's poll loop iterates. Rows are rendered in the **wire** vocabulary (`SubmissionStatus.PENDING`) and the live date shape, so fake-backed consumers see real shapes. **Two states it does NOT model** (recorded, not fixed): complete-without-score, and its prior row's `prior.csv` filename — live rows are all `submission.csv` (kaggle#9).
- Output via `kaggle.FormatSubmissionsCSV` (shared schema).

### Fake / test contract (env)
| var | meaning |
|-----|---------|
| `KAGGLE_CLI` | path to the binary `CLI` shells (`kaggle` by default; the fake in tests) |
| `KAGGLE_FAKE_STATE` | dir where the fake keeps per-competition state |
| `KAGGLE_FAKE_SCORE_AFTER` | polls before the fake reports a score (default 1) |
| `KAGGLE_LIVE_CONFORMANCE` | set to `1` to opt into the live-conformance test (`-tags live`); unset ⇒ skip |
| `KAGGLE_CONFORMANCE_SLUG` | competition the live-conformance test queries (must be one you have submissions in) |
| `KAGGLE_FAKE_DATA_DIR` | dir whose top-level files the fake `download` serves byte-for-byte (kaggle#2); unset → the `PassengerId,Survived` stub |

**Repo gate:** `make check` = gofmt + `go vet ./...` + **`go vet -tags live ./...`** + `go test ./...`.
The live-tag vet is what keeps `conformance_live_test.go` compiling despite being excluded from the
default build.

**`workshop/captures/`** — dated, IMMUTABLE archives of real CLI output (`submissions-live-2026-07-28.csv`
is the first). The canonical-copy rule: an archive file is never edited; `pkg/kaggle/testdata/` holds the
working fixture, curated from an archive and citing it in its header. Re-capturing adds a NEW dated file,
then updates the fixture. (Not registered in AGENTS.md's Directory Structure list on purpose — that file is
composed from `construct/base.manifest`, so a local edit would be clobbered; registering `captures/` as a
standard `workshop/` subdirectory is an ariadne base-layer change.)

**Honesty caveat (UPDATED 2026-07-28, kaggle#8):** the schema is no longer authored — `pkg/kaggle/testdata/submissions.csv` is a **live capture** (archive: `workshop/captures/submissions-live-2026-07-28.csv`), and a `//go:build live` conformance test (`internal/kagglecli/conformance_live_test.go`, opt-in via `KAGGLE_LIVE_CONFORMANCE=1`, refuses to run against the fake) checks the header + status shape against the real CLI — **verified passing against live Kaggle, 18 rows**. The capture disproved the authored guess in four ways (status vocab, dropped `ref`, date shape, complete-without-score), two of which had silently disabled production code paths. Still unverified: the `ERROR` status spelling (never observed), and the code-competition submit form.

## Surface (M2) — step-types + integration

The two step-types the metis runner invokes, wrapping the CLI for transport, plus
the Go step-side contract reader and a hermetic e2e. **kaggle stays a standalone Go
module** — steps read only `with.json`, never metis's types, so there is zero metis
import (the e2e drives the metis *binary* as a subprocess).

### `internal/stepio` — Go step-side metis-contract reader (**Decision A2**)
The first Go step-author's reader of the metis step contract. `New()` resolves the
`METIS_*` env the runner sets (**requires** `METIS_STEP_DIR`/`METIS_RUN_DIR`/`METIS_STEP_ID`; `EXP_DIR`/`SEED` best-effort); `ReadWith(&T)` unmarshals `with.json`; `WriteMetrics(map[string]float64)` writes the flat `metrics.json`; `UpstreamPath(id,file)` = `<RunDir>/<id>/<file>`; `OutPath(file)` = `<StepDir>/<file>`.
- **Decision A2:** the ~7 contract strings are declared **locally** (provenance → metis `atlas/experiment.md` "### Step-executable contract"), not imported — rule-of-two; promote to `metis/pkg/stepcontract` at the 3rd Go consumer (kbench).
- **Drift-guard (genuine, not a self-echo):** `New()` requires the vars from **env** (never a cwd fallback), and the M2 e2e drives the **real metis binary** (which emits the exact `METIS_*` names). A renamed metis var → step exits non-zero → `run.json.status:"failed"` → e2e RED. Verified by temporarily drifting a const.

### `cmd/kaggle-download` → step `kaggle/download` — the download half of an Adapter
Reads `with.competition`, `CLI.Download` into the step dir (yields `<slug>.zip`), then **unzips to loose files** (`train.csv`/`test.csv`) and drops the zip (zip-slip-guarded) — so metis records **loose artifacts**, the shape **kbench's `adapt` consumes**. No metric.

### `cmd/kaggle-submit` → step `kaggle/submit` — submit + async-score poll
Reads `with.competition` + `with.submission` (an **upstream step id**; the file is the conventional `submission.csv` at `UpstreamPath` — metis's id-naming convention, ARCH-DRY). Delegates the submit+poll core to **`internal/submit.SubmitAndPoll`** (shared with the ad-hoc `cmd/kaggle` CLI — kaggle#5); the step keeps only the step-specific tail (persist `submission.json` + `metrics.json`). On scored → `submission.json` (scored) + `metrics.json{public_score}`. **Timeout contract:** retries exhausted still-unscored → write `submission.json{status:pending}` (debug aid) and **exit non-zero** (an unscored run is a failed run → runner records `status:"failed"`).

### `internal/submit` — the shared submit→poll→score path (ARCH-DRY / ARCH-PURE, kaggle#5)
`SubmitAndPoll(cli Submitter, slug, csv, msg, maxAttempts, delay)` = `cli.Submit` then `pollScore`; the **one** correct blocking poll used by BOTH the `kaggle/submit` step and the `kaggle submit` CLI, so they can't drift. `pollScore` correlates the score to the submission by **file name** — which the first live capture proved is a **no-op on live**: every row is `submission.csv`, so the guard cannot discriminate and a poll landing before our row registers can return a PREVIOUS run's score (**kaggle#9**, open; the fix is to correlate on `Ref`).

### `cmd/kaggle` → the thin user-facing CLI (kaggle#5) — `kaggle submit`
`bin/kaggle` (auto-built by Makefile.workflow's cmd/* scan). First verb: **`kaggle submit [--run <id> | -f <file>] [-c <slug>] [-m <msg>]`** — the ad-hoc "offline sweep → promote a winner → submit that ONE run's `submission.csv` and tell me the score" flow, **no pipeline edit**. Resolution: file = `-f` else `runs/<id>/submission/submission.csv` (cwd-relative); slug = `-c` else best-effort `slugFromRecordJSON(runs/<id>/record.json)` (the first step `with.competition.slug` — a local minimal parse, **no metis import**, keeping the zero-dep posture) else an actionable "pass -c" error. Submits via the shared `internal/submit.SubmitAndPoll` (one submit/auth path with the step — same `kagglecli`/`${KAGGLE_CLI:-kaggle}` creds), **prints `public_score`** to stdout on success (non-zero exit + stderr note on timeout). Does NOT mutate the run record (keeps metis's `record.json` immutable; recording is a deferred follow-up). Same **M2 honesty** as the step: hermetic-fake-verified (green `cmd/kaggle` tests + a built-binary smoke), live-Kaggle path code-complete but not live-verified.

### `steps/kaggle/{download,submit}` — committed go-run wrappers
Mirror metis's committed-wrapper pattern (no build/codegen step): a `100755` bash wrapper resolves the repo root from `$0` and `exec go run -C "$ROOT" ./cmd/kaggle-<type>`. Binaries stay out of git; the step reads all paths from `METIS_*` env, so its `go run` cwd is irrelevant.

### `internal/kaggletest` — shared test helpers (ARCH-DRY)
`BuildBin` (build a cmd, incl. a sibling module via `-C`), `WireStep`/`WireFake`/`WriteUpstream`/`ReadJSON` — one set across the step + integration + e2e tests.

### e2e (`e2e/`, the issue Done-when)
`testdata/experiment/kaggle-thread.md` runs `download → make-submission → submit` under the **real built `metis`** against the fake, asserting `run.json` ok + `public_score>0` + a scored `submission.json` + loose download data. `testdata/steps/test/make-submission` is a stub producer writing a fixed `submission.csv` (**kbench's real submission step plays this role**; `submit` needs a real *upstream artifact*, not a fixture). Step resolution via `$METIS_STEP_PATH` (**Decision B**; general layered precedence = a metis follow-up).

### Step `with` contract + additional env
| step | `with` keys | outputs |
|------|-------------|---------|
| `kaggle/download` | `competition:{slug,metric?}` | loose data files |
| `kaggle/submit` | `competition:{slug}`, `submission:<upstream-id>`, `message?` | `submission.json` + `metrics.json{public_score}` |

| var | meaning |
|-----|---------|
| `KAGGLE_SUBMIT_MAX_ATTEMPTS` | submit poll attempts before timeout (default 30) |
| `KAGGLE_SUBMIT_DELAY` | delay between polls; Go duration or bare seconds (default 5s) |

**M2 honesty (UPDATED 2026-07-28):** verified path = the fake + green e2e; the live-Kaggle path is now **partially live-verified** — the `submissions` schema is captured and conformance-checked (kaggle#8), while `download`/`submit` transport remain fake-verified only. The formerly-gating authored-fixture point is CLOSED.
