# Lessons

Rules distilled from review findings, to prevent repeats. Read at session start (AGENTS.md §4).

## Fakes for external services (kaggle#1 plan review, 2026-07-01)
- **Don't let the fake and the parser co-derive from one unvalidated schema.** If the fake emits its wire format via the same `pkg/kaggle` code the parser consumes, the round-trip is self-consistent and CANNOT catch divergence from real Kaggle's actual CSV columns / status vocabulary / flag forms. Anchor the fixture to a real captured sample, or (when no live CLI exists) mark it "authored, validate-on-first-live-run" — never call it "captured."
- **Model the state transition, not the terminal state.** A fake that returns an already-*scored* submission leaves the submit step's async poll loop unexercised. Have the fake walk `pending → complete` (a `KAGGLE_FAKE_SCORE_AFTER` knob) so the retry logic actually iterates in the e2e.
- **A fixture is not an `UpstreamPath` artifact.** metis's `$METIS_RUN_DIR/<id>/<file>` contract requires a *prior step* to have written the file. An e2e that needs `submission.csv` as upstream input needs a producer step (or an exp-relative path), not a static testdata fixture.

## ARCH-PURE (kaggle#1 plan review)
- **Credential resolution is IO, not a pure entity.** Reading env + statting `~/.kaggle/kaggle.json` is IO; keep only a pure decision fn (`credentialSource(username, key, fileExists)`) in `pkg/kaggle` and do the env/stat in the `internal/kagglecli` IO layer. If a "pure" test needs `t.Setenv` / a temp `HOME`, it isn't pure.
- **Don't gate behavior on a binary's literal name.** "skip auth check when `bin != "kaggle"`" mis-fires for a real CLI at a full path (`.venv/bin/kaggle`). Gate on an explicit signal (`KAGGLE_FAKE=1`).
