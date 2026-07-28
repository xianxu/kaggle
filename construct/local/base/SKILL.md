---
name: base
description: "Use when working any Kaggle competition — determining the submission format BEFORE framing (CSV upload vs re-run notebook, which constrains model families), driving the CLI (push/submit/poll/pull/standings), avoiding the server-environment gotchas that silently burn submissions, reading the public commons as intel, and handling competition-mechanics exploits under quarantine. Load at charter time alongside metis-ml-research. Triggers: kaggle competition, submit to the leaderboard, kernel/notebook submission, code competition, why is the public LB better than our CV, public kernel, leaderboard standings, submission format."
version: 0.1-prose
status: forming
---

# Kaggle base — platform procedures

**Scope rule** (the layer's own charter, `atlas/kaggle-layer.md`): *does it touch the Kaggle
API/CLI or the competition's scoring mechanics?* → **here**. Modeling epistemics — arrows, oracles,
the impostor ladder, honest CV — → **`metis-ml-research`**. Load both; they compose. This skill is
prose you follow by hand; where the `kaggle` Go binary already automates a step, it says so.

---

## 1. Charter time: determine the SUBMISSION FORMAT first

Do this **before framing**, not at first submission — the format constrains which model families
are even deployable, and discovering that late invalidates design work.

| | **CSV upload** | **Re-run notebook (code competition)** |
|---|---|---|
| What the judge takes | a predictions file | your notebook/script, which Kaggle **re-executes** against hidden data |
| Where the model may live | anywhere (train offline, upload numbers) | **inside the artifact** — code + all reference data |
| Internet at scoring | n/a | usually **disabled** |
| Learned weights | free | must be inlined in source or attached as a dataset |
| Practical consequence | model size/compute unconstrained | favors **analytic/recomputable** assemblies; every scoring run rebuilds from the mounted data |
| Runtime | your machine | the judge's limit (often ~9–12 h, GPU optional) |

**How to determine:** the competition page's *Code Requirements* section is authoritative. Empirically,
a code competition rejects file-only submissions — `competitions submit` needs `-k <kernel> -v <version>`.

**Also charter now** (goes in the experiment's `framing.md §submission-and-scoring`): submission caps
and scoring cadence; the public/private split and its sizes; how the judge's population maps to your
CV; and the exact submit procedure as copy-paste commands. See §7.

---

## 2. The CLI

**Shadowing (bites first, every time):** this workspace's own `kaggle` Go binary is on `PATH` and
handles a *different* flow (CSV submit from `runs/`). For competition/kernel operations invoke the
**official Python CLI explicitly**:

```bash
uvx --from kaggle kaggle <cmd>      # or: command kaggle <cmd>
```

```bash
# competition data + standings
kaggle competitions download -c <slug> -p <dir>
kaggle competitions leaderboard <slug> --download -p <dir>   # full standings CSV (see §5)

# kernels (code competitions)
kaggle kernels push -p <dir>                    # dir holds the code file + kernel-metadata.json
kaggle kernels status <owner/slug>              # -> KernelWorkerStatus.RUNNING|COMPLETE|ERROR
kaggle kernels output <owner/slug> -p <dir>     # pulls output files AND the run log
kaggle kernels list --competition <slug> --sort-by voteCount --page-size 100
kaggle kernels pull <owner/slug> -p <dir>

# submitting
kaggle competitions submit <slug> -k <owner/kernel> -v <version> -f submission.csv -m "msg"
kaggle competitions submissions <slug>          # -> SubmissionStatus.PENDING|COMPLETE + publicScore
kaggle quota                                    # GPU/TPU hours remaining this week
```

**Submit syntax is fussy** — the working form takes the competition **positionally**, plus `-f` naming
the *kernel's output file*. Both `-c <slug>` and omitting `-f` return `400 Bad Request` with no
explanation. (Cost when unknown: two failed attempts and a confusing detour.)

**`kernel-metadata.json`** (beside the code file):
```json
{"id": "<user>/<slug>", "title": "...", "code_file": "x.py", "language": "python",
 "kernel_type": "script",  "is_private": true, "enable_gpu": true, "enable_internet": false,
 "dataset_sources": ["owner/dataset"], "competition_sources": ["<slug>"], "kernel_sources": []}
```

**Polling:** kernel runs and scoring reruns take hours. Poll in the background with an until-loop on
`kernels status` / `competitions submissions`; never block a session on a foreground sleep.

**Status strings are Python enum reprs** (`SubmissionStatus.COMPLETE`, `KernelWorkerStatus.RUNNING`),
not bare words — parsers keying on `complete|pending` will miss.

---

## 3. Server-environment gotchas

Each of these fails **silently** — the run succeeds, the numbers look plausible, the measurement is void.

- **Data nesting.** Locally your data sits at `data/raw/train/`; on the judge it is under
  `/kaggle/input/...` and a re-run judge may nest it **deeper than the interactive mount**. Always
  derive paths from a recursive glob and reuse *that* directory:
  ```python
  train_paths = sorted(glob.glob(f"{BASE}/**/train/*.csv", recursive=True))
  TRAIN_DIR = os.path.dirname(train_paths[0])   # never hardcode f"{BASE}/train"
  ```
  A hardcoded path finds nothing, the dependent feature no-ops, and you burn a submission measuring
  your unchanged baseline. (Measured cost: one submission.)
- **The interactive mount is NOT the scoring mount.** Sample test entities visible while developing
  are generally *not* the scored ones. Anything keyed to specific test IDs behaves differently at
  scoring — verify such a mechanism by its **effect on the score**, never by the interactive log.
- **The scoring environment DRIFTS.** Bit-identical code resubmitted two days later scored
  **9.529 → 9.662** (+0.133 RMSE) — library versions moved under an in-kernel-trained component.
  Same-day reruns were identical, so this is environment vintage, not seed noise. **Measure it**
  (resubmit an unchanged version occasionally) and require the spend bar to exceed it.
- **No internet** → dependencies ship as attached datasets (the "offline wheel" pattern), and anything
  learned offline must be inlined or attached.
- **Quota** is weekly (`kaggle quota`); kernel runs consume GPU hours but **not** submission slots —
  reruns for diagnosis are cheap, submissions are not.

---

## 4. Submissions are a scarce, NOISY instrument

Spend one only on **(a)** a structural mechanism whose offline margin clears both selection noise and
the measured rerun drift, or **(b)** a deliberate one-axis diagnosis (a dose-response along a single
knob is the strongest mechanism confirmation available). Grid-argmax micro-gains and group-exploiting
legs without their leave-group-out rung are not spends. When the expected effect is smaller than the
environment drift, the result is uninterpretable regardless of outcome — that is not a cheap
experiment, it is a wasted measurement.

---

## 5. Reading the commons (public kernels + standings are intel)

Public artifacts are **ACHIEVED-grade about what scores** and **code-grade fact about how** — far
stronger provenance than forum self-reports. Procedure:

1. **Download the full standings** (`competitions leaderboard --download`) and histogram them. Look for
   **score clusters**: ~100 teams within 0.03 of each other is the signature of a forked deterministic
   notebook, not independent method. That single read reprices "the leaders are far ahead."
2. **List by votes, pull the top kernels**, and digest each: method, pipeline order, target
   parameterization, magic numbers, claimed scores, exploitation signs, and *lineage* (diff them — a
   whole band is often one codebase).
3. **Reproduce the best one under your own account** — push it verbatim with its dataset dependencies,
   run it, submit it. This converts their CLAIMED numbers into ACHIEVED ones and splits "their method
   is better" from "their score is mechanics." Their in-run CV usually prints too, giving you their
   honest number for free.
4. **Re-measure any borrowed idea inside your own rig**, with your gates and anchors. Never import a
   verdict wholesale — the prior-run policy in `metis-ml-research` applies to other people's work too.

---

## 6. Poisonous arrows — competition-mechanics exploits, QUARANTINED

A **poisonous arrow** is a mechanism that moves the leaderboard **without solving the problem**. They
are worth understanding (they explain otherwise-baffling score gaps) and dangerous to hold (they
corrupt a ledger's belief state and generalize to nothing).

**The quarantine charter** — keep these in a `## Poisonous arrows` section, structurally separate from
the arrows ledger:
1. **No verdict here feeds an arrow's belief**, and **nothing here graduates** into the reusable
   mechanisms a research skill extracts.
2. **Run one only to EXPLAIN A DISCREPANCY** ("why is the public frontier 2 better than our honest
   CV?"), never as strategy — and say so in the row.
3. **Record what you refuse** and why. A refused exploit is charter, not an oversight.
4. **Public ≠ private.** Most of these are public-split-only; they vanish or invert on the hidden
   split — the public kernels' own comments admit it. Note the expected private behavior explicitly.

**The recurring catalog:**
- **Train/test entity overlap** — the same entity appears in `train/` (labeled) and `test/`. Two
  channels: *copy* its labels, or *include it as a neighbor/training row* (survives version drift that
  breaks copying, because interpolation smooths rather than inherits).
- **Public-LB probing** — fit a per-entity constant by successive scored submissions (submit +2, +3,
  quadratic-fit the optimum). Turns the leaderboard into a training signal; pure public overfit.
- **Public-set backtesting** — per-entity model selection scored on the *public* entities' own visible
  portion, then applied to those same entities.
- **Fork-and-tweak** — forking a public high-scorer and micro-tuning blend weights (the score cluster
  in §5).
- **External-data leakage** — an attached dataset containing the answers or their upstream source.

**Detection in public code:** hardcoded entity IDs, sha-audited parent submissions, "profiles"
(conservative/balanced/aggressive), move caps in hundredths of a unit, `if id in train_ids:`.

**Worked example (rogii-v2, 2026-07-28).** The public frontier sat ~2 below our honest CV. Four
channels were enumerated and adjudicated at the cost of two submissions: the label-copy fired
locally but its guard refused at scoring (versions drift); the backtest overlay moved predictions
by ~1e-13 (inert); ID-probing was recorded and refused; and disabling our own duplicate-exclusion
changed the score by **0.000** — proving the scoring split shares no IDs with train. Net yield: a
false "we are below the field median" panic became a measured "the field's honest CV equals ours,"
and the remaining gap became a well-posed *transfer* question. That is the entire legitimate use of
this section.

---

## 7. Competition peculiarities — the per-competition inventory

Every competition has mechanics that are **not** the modeling problem but **do** shape framing. Keep
them in the experiment's `framing.md §submission-and-scoring`, written at charter time:

- judge type + artifact (§1), what must ship inside it, internet/runtime/quota limits;
- submission caps, scoring cadence, public/private split sizes and selection rules;
- **how the judge's population maps to your CV** — and, as they accumulate, the measured
  **per-model-family CV→judge gaps** (a family whose gap is 2× another's has a fold artifact);
- data quirks: duplicated entities, ID schemes, NaN patterns, columns present in train but not test;
- the exact submit procedure as copy-paste commands, including the local shadowing workaround.

Keep peculiarities out of the arrows ledger (they are not hypotheses) — but let them revise the
framing, because a re-run judge forbidding weight-shipping is a **design constraint**, not trivia.
