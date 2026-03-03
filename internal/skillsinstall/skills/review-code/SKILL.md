---
name: review-code
description: >
  Comprehensive code review of the current branch against a base branch using parallel
  sub-agents for correctness, design, comments, and tests. Use when reviewing local changes
  before opening a PR, or when the user asks for a code review.
argument-hint: "[base-branch]"
---

# Code Review

Orchestrate a comprehensive review of the current branch's changes by dispatching parallel
sub-agent reviews, then synthesize findings into a single cohesive report.

## Scope

Review ALL changes in the current branch compared to the base branch, including unstaged changes.

**Default base branch:** `main`
**Override:** If an argument is provided, use it as the base branch (e.g., `/review-code develop`).

## Step 1: Gather the Diff

```bash
# Get the full diff including unstaged changes against the base branch
git diff <base-branch>...HEAD

# Also capture unstaged changes not yet committed
git diff

# List changed files for context
git diff --name-only <base-branch>...HEAD
git diff --name-only
```

Combine both diffs to get the complete picture of what will differ from the base branch.

## Step 2: Dispatch Sub-Agent Reviews

Spawn **four sub-agents in parallel** using the Task tool, each focused on one review dimension.
Provide each sub-agent with the full diff and the list of changed files.

**Critical:** Sub-agents do not have skills loaded automatically. Each sub-agent prompt MUST
instruct the agent to first read the skill file, then follow its instructions. Include the
full diff content in the prompt so the sub-agent has the code to review.

### Sub-Agent 1: Correctness Review
> Read the `review-correctness` skill and follow its instructions. Review all changed code
> for logic bugs, error handling gaps, edge cases, race conditions, resource leaks, and
> security vulnerabilities. For Go projects, run `deadcode -test ./...` in the repo root
> to identify unreachable functions as part of the dead code analysis step — do not skip
> this even though you have the diff; the tool requires the full build graph. Return findings
> in the output format specified by the skill.

### Sub-Agent 2: Design Review
> Read the `review-design` skill and follow its instructions. Review all changed code for
> naming, structure, abstraction quality, API surface, boundary clarity, and coupling. Return
> findings in the output format specified by the skill.

### Sub-Agent 3: Comment Review
> Read the `review-comments` skill and follow its instructions. Review all comments in the
> changed code for relevance and purpose. Identify comments that restate code, are stale, or
> are missing where they would help. Return findings in the output format specified by the skill.

### Sub-Agent 4: Test Review
> Read the `review-tests` skill and follow its instructions. Review all test files in the
> changed code for coverage quality, redundancy, and value. For Go projects, run
> `go test ./... -coverprofile=coverage.out -covermode=atomic && go tool cover -func=coverage.out`
> in the repo root to get per-function coverage data before analyzing the diff — do not skip
> this even though you have the diff; coverage requires actually running the tests. Identify
> missing tests for new code paths. Return findings in the output format specified by the skill.

## Step 3: Synthesize Results

After all sub-agents complete, aggregate their findings into a single review.

### Severity Classification

Classify every finding into one of three levels:

| Level | Meaning | Requires Action? |
|-------|---------|-----------------|
| **Critical** | Bug, security issue, data loss risk, race condition | Yes -- must fix before merge |
| **Suggestion** | Design improvement, missing error context, better naming | Recommended |
| **Nit** | Style, comment quality, minor naming preference | Optional |

### Deduplication

Sub-agents may flag the same location for different reasons. When this happens:
- Merge findings for the same location into one entry
- List all applicable categories (e.g., "Correctness + Design")
- Use the highest severity across the merged findings

## Output Format

```markdown
# Code Review: <branch-name> → <base-branch>

## Overview
[2-3 sentence summary: what this branch does, overall quality assessment, and whether
it is ready to merge]

## Critical Issues
[Items that MUST be fixed before merge. If none, say "None found."]

| # | Location | Category | Issue | Recommendation |
|---|----------|----------|-------|----------------|
| 1 | file:line | Correctness | Description | Fix |

## Suggestions
[Items worth improving but not blocking]

| # | Location | Category | Issue | Recommendation |
|---|----------|----------|-------|----------------|
| 1 | file:line | Design | Description | Suggestion |

## Nits
[Minor items, optional to address]

| # | Location | Category | Issue | Recommendation |
|---|----------|----------|-------|----------------|
| 1 | file:line | Comments | Description | Suggestion |

## Test Coverage
[Summary from test review: are new code paths covered? Any redundant tests?]

## Verdict
**[APPROVE / REQUEST CHANGES / NEEDS DISCUSSION]**
[One sentence justification]
```

## Guidelines

1. **Be direct** - State what's wrong and how to fix it. Skip the compliments.
2. **Prioritize** - Critical issues first. Don't bury a race condition under nits.
3. **Be specific** - Reference exact files and lines. Vague feedback is useless.
4. **Assume competence** - The author can read code. Don't explain what the code does.
5. **Suggest, don't demand** - For non-critical items, frame as suggestions.
6. **Acknowledge good work** - A brief note in the overview if the code is solid. Don't over-praise.
