# kaggle atlas

Codebase map for the **kaggle** layer — the Kaggle platform-integration layer of
the `kaggle-ml-base-layer` project (chain `kbench → kaggle → metis → ariadne`).

- [kaggle-layer.md](kaggle-layer.md) — the layer's surface: `pkg/kaggle` (pure
  state + parsers), `internal/kagglecli` (injectable CLI seam), `cmd/fake-kaggle`
  (process-level fake), and the fake/test env contract. Current: **M1 shipped**
  (the kaggle library); M2 pending (step-types + e2e under `metis run`).
