# Lessons

Rules distilled from review findings, to prevent repeats. Read at session start (AGENTS.md §4).

## Fakes for external services (kaggle#1 plan review, 2026-07-01)
- **Don't let the fake and the parser co-derive from one unvalidated schema.** If the fake emits its wire format via the same `pkg/kaggle` code the parser consumes, the round-trip is self-consistent and CANNOT catch divergence from real Kaggle's actual CSV columns / status vocabulary / flag forms. Anchor the fixture to a real captured sample, or (when no live CLI exists) mark it "authored, validate-on-first-live-run" — never call it "captured."
- **Model the state transition, not the terminal state.** A fake that returns an already-*scored* submission leaves the submit step's async poll loop unexercised. Have the fake walk `pending → complete` (a `KAGGLE_FAKE_SCORE_AFTER` knob) so the retry logic actually iterates in the e2e.
- **A fixture is not an `UpstreamPath` artifact.** metis's `$METIS_RUN_DIR/<id>/<file>` contract requires a *prior step* to have written the file. An e2e that needs `submission.csv` as upstream input needs a producer step (or an exp-relative path), not a static testdata fixture.

## ARCH-PURE (kaggle#1 plan review)
- **If a "pure" test needs `t.Setenv` / a temp `HOME`, it isn't pure.** Keep IO (env reads, `os.Stat`) at the `internal/*` boundary; the pure layer takes already-gathered values. (The original credential example — a pure `credentialSource` fn fed by IO — was **superseded by kaggle#3**, which deleted the precheck outright: there was no auth *decision* to keep pure at all. See ARCH-DRY below. The general pure/IO-split lesson still holds.)
- ~~**Don't gate behavior on a binary's literal name** … gate on an explicit signal (`KAGGLE_FAKE=1`).~~ **Superseded by kaggle#3** — the auth precheck that `KAGGLE_FAKE=1` gated is gone, so the signal is gone too. The kernel is still true (never infer intent from a binary's name), but there is no longer a check to gate.

## ARCH-DRY — don't mirror an external tool's evolving rules (kaggle#3, 2026-07-02)
- **Delegate to the wrapped tool and surface its own error; don't re-implement its logic for a "friendlier" pre-check.** kaggle#1 added a pre-flight `checkCredentials` mirroring the `kaggle` CLI's auth (env pair OR `~/.kaggle/kaggle.json`). That mirror **drifted**: the current CLI authenticates via `~/.kaggle/access_token` (+ OAuth + `KAGGLE_API_TOKEN`), so the stale check false-negatived a valid setup and failed *before the real CLI — which would have authenticated fine — ever ran*. A local mirror of an external source of truth's rules is a DRY violation that drifts and blocks valid inputs. The fix was to **delete the mirror** and let the tool own its decision, surfacing its own error via `wrap()`. Before writing any pre-flight validation of an external tool's inputs, ask: does the tool already validate and report this itself? If so, you're duplicating a moving target — don't.
