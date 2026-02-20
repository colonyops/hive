---
icon: lucide/book-open
---

# Recipes

Practical guides for common hive workflows and integrations.

## Code Review

Two multi-agent review patterns using hive's tmux window spawning and messaging primitives. Both open a dedicated set of windows on the current session — no separate session needed.

### [Sequential Chain Review](sequential-chain-review.md)

Serial — each agent reads and challenges the previous. Best for thorough adversarial analysis where you want reviewers to explicitly refute each other; contradictions get resolved rather than buried.

### [Parallel Code Review](parallel-code-review.md)

Parallel specialists plus a coordinator. Best for fast feedback with active coordination: both specialists run simultaneously and the leader drives targeted follow-up rounds.

## Automation

### [Ralph Loop](ralph-loop.md)

Work through a beads backlog autonomously — runs a fresh Claude process per task, retrying until quality gates pass before moving on.
