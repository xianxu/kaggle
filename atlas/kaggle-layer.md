# kaggle layer

The Kaggle **platform-integration** layer of the `kaggle-ml-base-layer` project
(chain `kbench → kaggle → metis → ariadne`). Rule: *does it touch the Kaggle
API/CLI?* → it lives here, not in metis. **Go owns the STATE; the official
`kaggle` CLI owns the TRANSPORT.**

## Surface (M1) — the kaggle library

### `pkg/kaggle` — pure state + parsers (IO-free, table-tested; ARCH-PURE)
- **`Competition`** (`competition.go`) — thin config `{Slug, Metric, Deadline}` supplied by an experiment's `with`; `Validate()` requires a slug.
- **`Submission`** (`submission.go`) — one upload's durable record `{Competition, File, Message, SubmittedAt, Status, PublicScore *float64}`, serialized as `submission.json`. `PublicScore` is a pointer because Kaggle scores **asynchronously** (nil until scored). `Scored()` reports non-nil. Status vocab: `pending|complete|error`.
- **`CredentialSource(username, key string, fileExists bool)`** (`credentials.go`) — the **pure** auth decision (present iff env-pair OR file); `ErrNoCredentials` names both mechanisms. The env-read + `os.Stat` that feed it are IO and live in `kagglecli` — never here.
- **`ParseSubmissions` / `LatestScored` / `FormatSubmissionsCSV`** (`parse.go`) — the single CLI-text↔typed-state boundary. Header-driven CSV parse (column-by-name, order-independent; `#` comment lines skipped). `LatestScored` returns the newest scored row (Kaggle lists newest-first). Format is the inverse, used by the fake so fake+parser share **one** schema (`submissionsCSVHeader`) — ARCH-DRY.
  - **Deferred (YAGNI):** `Leaderboard` — the public-score purpose is served off `Submission.PublicScore`; a full leaderboard record waits for a `kaggle/leaderboard` step.

### `internal/kagglecli` — the thin IO seam (ARCH-PURE boundary)
- **`CLI`** wraps `${KAGGLE_CLI:-kaggle}` (injectable). Methods `Download(slug,dest)`, `Submit(slug,file,msg)`, `Submissions(slug) → raw --csv stdout`. **No parsing** here (that's `pkg/kaggle`). Each real call runs `checkCredentials()` first — **skipped iff `KAGGLE_FAKE=1`** (explicit signal, never a binary-name match).

### `cmd/fake-kaggle` — process-level fake (the deliverable's fake, not scaffolding)
A real subprocess speaking `competitions {download, submit, submissions}`:
- `download` writes a real-shaped **`.zip`** into `-p`.
- `submit` records state; `submissions` models the async **transition** — `pending` for the first `KAGGLE_FAKE_SCORE_AFTER` (default 1) polls, then `complete`+scored — so a consumer's poll loop iterates.
- Output via `kaggle.FormatSubmissionsCSV` (shared schema).

### Fake / test contract (env)
| var | meaning |
|-----|---------|
| `KAGGLE_CLI` | path to the binary `CLI` shells (`kaggle` by default; the fake in tests) |
| `KAGGLE_FAKE=1` | skip the credential precheck (fake needs no auth) |
| `KAGGLE_FAKE_STATE` | dir where the fake keeps per-competition state |
| `KAGGLE_FAKE_SCORE_AFTER` | polls before the fake reports a score (default 1) |

**Honesty caveat:** this repo has no `kaggle` CLI and no credentials, so the fake+e2e is the *verified* path; the live-Kaggle path is code-complete but **not live-verified**. `pkg/kaggle/testdata/submissions.csv` is an **authored** fixture (Kaggle-CLI-docs provenance) — its columns/status vocab must be validated against the first live capture; fake and parser co-derive from it, so it is the one unverified schema point.

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
Reads `with.competition` + `with.submission` (an **upstream step id**; the file is the conventional `submission.csv` at `UpstreamPath` — metis's id-naming convention, ARCH-DRY). `CLI.Submit`, then **polls** `Submissions`→`ParseSubmissions`→`LatestScored` with bounded retries. The retry loop is `pollScore(subFn, max, sleep)` with an **injected clock** (ARCH-PURE — unit-tested pure, zero wall-clock). On scored → `submission.json` (scored) + `metrics.json{public_score}`. **Timeout contract:** retries exhausted still-unscored → write `submission.json{status:pending}` (debug aid) and **exit non-zero** (an unscored run is a failed run → runner records `status:"failed"`).

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

**M2 honesty (carries M1's):** verified path = the fake + green e2e; the **live-Kaggle path is code-complete but NOT live-verified** (no CLI/creds here). The single unverified point gating the live path remains the **authored `pkg/kaggle/testdata/submissions.csv` schema** (column names / status vocab / score column) — fake + parser + submit + e2e all co-derive from it, so a wrong column passes the hermetic e2e green and only surfaces on the first live capture (operator / kbench#1).
