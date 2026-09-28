---
id: 000010
status: open
created: 2026-09-18
updated: 2026-09-18
estimate_hours:
github_issue:
---

# atlas: migrate to purpose split (map / journeys / workflow)

## Problem

ariadne#238 splits atlas by purpose:
- **the map** (`atlas/` root and feature folders): short pointers, terminology,
  design reasons
- **user journeys** (`atlas/journeys/`): steps in the user's words plus an
  interruption table of current behavior
- **workflow** (`atlas/workflow/`)

It also sets a sorting rule for existing content: user-visible behavior goes to
`journeys/`, an invariant goes to `workshop/targets/`, a pointer or design reason
stays in the map (short), and prose that restates the code is deleted.

This repo's atlas predates that split.

**Current atlas (2026-09-18):** 107 lines across: `atlas/index.md`, `atlas/kaggle-layer.md`. Not yet surveyed in detail; the first step is to sort these pages and to identify the 2–5 journeys central to this repo's users, or to decide it has no user surface.
