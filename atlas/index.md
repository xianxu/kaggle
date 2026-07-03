# kaggle atlas

Codebase map for the **kaggle** layer — the Kaggle platform-integration layer of
the `kaggle-ml-base-layer` project (chain `kbench → kaggle → metis → ariadne`).

- [kaggle-layer.md](kaggle-layer.md) — the layer's surface: `pkg/kaggle` (pure
  state + parsers), `internal/kagglecli` (injectable CLI seam), `cmd/fake-kaggle`
  (process-level fake), `internal/stepio` (Go step-side metis-contract reader),
  the `kaggle/download` + `kaggle/submit` step-types + `steps/kaggle/*` wrappers,
  `internal/kaggletest` (shared test helpers), and the `download → make-submission
  → submit` e2e under real `metis run`. Current: **M1 + M2 shipped** (the kaggle
  library + the platform-integration step-types; fake-verified, live-Kaggle
  deferred to the operator / kbench#1).
