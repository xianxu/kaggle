---
id: '000003'
status: done
started: 2026-07-02T20:22:04-07:00
created: 2026-07-02
updated: 2026-07-02
estimate_hours: 0.52
actual_hours: 0.17
---

# kaggle CLI wrapper: delegate auth to the CLI (support access_token + OAuth), drop the stale credential precheck

## Problem

Surfaced during the kbench Titanic **operator live-run** (kbench#1). The operator
installed the current Kaggle CLI (2.2.3) and authenticated the modern way —
`~/.kaggle/access_token` (the new default token file). `bin/krun` failed at the
`get-data` step:

```
kaggle/download: kaggle: no credentials — set KAGGLE_USERNAME + KAGGLE_KEY, or install ~/.kaggle/kaggle.json
```

That error is **ours**, not the CLI's. `internal/kagglecli.checkCredentials` runs a
pre-flight guard *before* shelling to the CLI, and it only recognizes two **legacy**
mechanisms: the `KAGGLE_USERNAME`+`KAGGLE_KEY` env pair, or `~/.kaggle/kaggle.json`.
The new CLI supports **four** auth methods (per its docs): OAuth (`kaggle auth login`),
`KAGGLE_API_TOKEN` env var, `~/.kaggle/access_token`, and legacy `kaggle.json`. Our
guard false-negatives a valid `access_token` setup (and would also block an OAuth
login, which leaves no file and no env var), blocking the real CLI before it can run.

Root cause: the guard is a **stale, partial mirror** of the wrapped CLI's own auth
logic (DRY violation against an external source of truth). It has already drifted
once (kaggle.json era → access_token era) and will drift again. We *wrap* the CLI;
the CLI is the single source of truth for its own auth, and already emits a clear
error on missing creds — which our `wrap()` surfaces via stderr. `ARCH-DRY`.
