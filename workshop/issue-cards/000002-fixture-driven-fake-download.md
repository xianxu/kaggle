---
id: '000002'
status: done
started: 2026-07-02T17:37:24-07:00
created: 2026-07-02
updated: 2026-07-02
estimate_hours: 0.58
actual_hours: 0.13
---

# fake-kaggle: fixture-driven download (serve real competition columns for full-thread e2e)

## Problem

`cmd/fake-kaggle`'s `competitions download` emits a hardcoded `PassengerId,Survived` two-row stub (`main.go` `doDownload`). That's enough for the kaggle layer's own e2e (which only needs the download→unzip plumbing to produce loose files), but it can't drive a **full three-layer thread**: a downstream competition adapter (kbench's `titanic/adapt`) needs the **real competition columns** (`Pclass,Sex,Age,SibSp,Parch,Fare,…`), not a two-column stub. Consumers doing a hermetic end-to-end run therefore have no way to make the fake serve realistic competition data.
