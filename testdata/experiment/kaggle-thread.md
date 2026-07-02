---
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
    with: {competition: {slug: titanic}, submission: make-submission, message: "e2e baseline"}
---
# kaggle-thread — kaggle#1 M2 hermetic e2e

The kaggle half of the Titanic thread, run against the process-level fake `kaggle`
CLI so CI never touches live Kaggle. Three steps:

1. `kaggle/download` — auth + pull the competition data (a `.zip`), unzip to loose
   files (`train.csv`/`test.csv`) — the download half of an Adapter.
2. `test/make-submission` — a stub producer that writes a fixed `submission.csv`
   (in the real thread, kbench's own submission step plays this role — `submit`
   needs a real UPSTREAM artifact, and a fixture is not one).
3. `kaggle/submit` — submit the CSV, poll the async scoring until scored, emit a
   typed `submission.json` + `metrics.json{public_score}`.

## Runs
