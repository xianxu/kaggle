---
id: 000011
status: open
created: 2026-10-19
updated: 2026-10-19
estimate_hours:
github_issue:
---

# CI: build weave from a pinned ariadne commit

## Problem

ariadne#459 reproduced the cold generated-vocabulary failure in published weave
0.1.0, before ariadne#422's shipped-output sidecar ownership. This repository's
active GitHub main merge-check installs the published formula without an exact
source pin. Verified 2026-10-19 against [the exact workflow blob](https://api.github.com/repos/xianxu/kaggle/git/blobs/29440f124ceaff4c682d4a82fde63a339ae6a1d1).
