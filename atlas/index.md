# kaggle atlas

Codebase map for the **kaggle** layer — the Kaggle platform-integration layer of
the `kaggle-ml-base-layer` project (chain `kbench → kaggle → metis → ariadne`).

- [kaggle-layer.md](kaggle-layer.md) — the layer's surface: `pkg/kaggle` (pure
  state + parsers), `internal/kagglecli` (injectable CLI seam), `cmd/fake-kaggle`
  (process-level fake), `internal/stepio` (Go step-side metis-contract reader),
  the `kaggle/download` + `kaggle/submit` step-types + `steps/kaggle/*` wrappers,
  `internal/submit` (the shared submit→poll→score core), `cmd/kaggle` (the thin
  user-facing `kaggle submit` CLI — ad-hoc winner submission, kaggle#5),
  `internal/kaggletest` (shared test helpers), and the `download → make-submission
  → submit` e2e under real `metis run`. Current: **M1 + M2 shipped** + the ad-hoc
  `kaggle submit` CLI (fake-verified, live-Kaggle deferred to the operator / kbench#1).
