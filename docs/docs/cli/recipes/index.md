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

### [Inter-Agent Code Review](inter-agent-code-review.md)

Spin up a dedicated reviewer agent via inbox messaging. Your session sends context, the reviewer analyzes the branch, and feedback comes back through pub/sub — no manual coordination needed.

## Context Management

### [Git-backed Context](git-backed-context.md)

Store plans and research documents in a git repository by pointing `context.base_dir` at a dedicated repo. Gives you full history, diffs, and cross-machine sharing for all your `.hive/` documents.

## Automation

### [Ralph Loop](ralph-loop.md)

Work through an `hive hc` epic backlog autonomously — runs a fresh Claude process per task, retrying until quality gates pass before moving on.
