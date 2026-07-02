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

## Pending (M2)
`internal/stepio` (Go step-side reader of metis's contract; **Decision A2** — local consts, promote to `metis/pkg/stepcontract` at the 3rd Go consumer), the `kaggle/download` + `kaggle/submit` step-types, and the `download → make-submission → submit` e2e under `metis run`. Cross-layer step resolution uses `$METIS_STEP_PATH` (Decision B; general precedence = a metis follow-up).
