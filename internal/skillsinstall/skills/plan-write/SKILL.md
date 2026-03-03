---
name: plan-write
description: >
  Create detailed implementation plans through interactive research and iteration.
  Use when the user wants to plan a feature, task, or ticket before implementing.
  Expects a research document as input. Interviews the user, then uses sub-agents
  to draft and review the plan before writing a structured plan to .hive/plans/.
allowed-tools: "Bash(git status:*),Bash(git log:*),Bash(git diff:*),Read,Write,Task(*)"
version: "2.0.0"
author: "User"
---

# Implementation Plan

Create detailed implementation plans from an existing research document. Be skeptical,
thorough, and work collaboratively to produce high-quality technical specifications.

This skill assumes a research document already exists in `.hive/research/` — produced
by the `/research` skill. If no research doc exists, run that skill first.

## Context Directory

Plans are stored in `.hive/plans/` managed by `hive ctx`.

**IMPORTANT:** `.hive` must be a symlink, not a directory. If it doesn't exist, run
`hive ctx init` to create it — NEVER use `mkdir`.

---

## Step 1: Read the Research Document

Read the provided research document **fully** using the Read tool WITHOUT limit/offset
parameters. Do not proceed until you have read it completely.

If no research document is specified, ask the user which research doc to use:
```bash
hive ctx ls  # list available research docs
```

---

## Step 2: Interview the User

Present your understanding of the research and resolve all open decisions before
dispatching sub-agents. Sub-agents cannot ask the user questions — all ambiguity
must be resolved here.

### 2a. Present informed understanding

Summarize what the research doc says we need to build:

```
Based on the research, I understand we need to [accurate summary].

Key findings:
- [Current implementation detail with file:line reference]
- [Relevant pattern or constraint]
- [Potential complexity or edge case]

Open questions from research:
- [Any items marked as open questions in the research doc]
```

### 2b. Use AskUserQuestion for decisions

- Design preferences between valid alternatives
- Scope boundaries and explicit out-of-scope items
- Business logic decisions the research doc couldn't answer
- Performance or compatibility requirements
- Approved implementation approach when multiple are valid

Only ask questions you genuinely cannot answer from the research doc.

### 2c. Ambiguity checkpoint

Before proceeding, ask yourself:

> "What decisions would a fresh agent, reading only the research doc and approved
> approach, have to guess at?"

For each answer: resolve it now with the user. Do not leave implicit assumptions.

### 2d. Confirm the phase structure

Propose how you'll break the work into phases:

```
Here's my proposed plan structure:

1. [Phase name] — [what it accomplishes]
2. [Phase name] — [what it accomplishes]
3. [Phase name] — [what it accomplishes]
```

Get explicit approval before dispatching sub-agents.

---

## Step 3: Sub-Agent Dispatch

Spawn sub-agents in two sequential waves.

### Wave 1: Draft Plan

Spawn a single **general-purpose** sub-agent to write the initial plan draft.

**Prompt template:**
```
You are writing an implementation plan. Read the research document at:
  [path to research doc]

The approved approach and phase structure are:
  [paste the approved approach and phases from Step 2]

User decisions made:
  [paste all decisions from the Step 2 interview]

Out of scope:
  [paste explicit out-of-scope items]

Write a detailed implementation plan to:
  .hive/plans/[YYYY-MM-DD-description.md]

Use this exact template: [embed the Plan Template from the appendix below]

Requirements:
- Include specific file paths and line numbers from the research doc
- Every phase must have both Automated and Manual success criteria
- Testing strategy must explicitly address unit, integration, and e2e tests
- Do not invent decisions not documented above
- If you encounter a gap, write "[OPEN: description]" — do not guess
```

Wait for the draft agent to complete before proceeding.

### Wave 2: Parallel Reviews

Spawn both reviewers simultaneously. Each receives the research doc path AND the
plan file path written by the draft agent.

#### Design Reviewer

**Agent type:** general-purpose

**Prompt:**
```
You are a design reviewer. Critique an implementation plan for design quality.
Do not rewrite it — only identify issues and make recommendations.

Research doc: [path]
Plan file: [path]

Review for:
1. Code duplication — does the plan recreate patterns that already exist in the
   codebase? Reference file:line for any duplication found.
2. Testability — are proposed components testable in isolation? Flag tight coupling
   or hidden dependencies.
3. Abstraction level — is the plan over-engineered for the scope? Is anything
   under-abstracted that will cause maintenance pain?
4. Language/framework best practices — flag patterns that violate conventions for
   the tech stack described in the research doc.

Output format:

## Design Review

### Critical Issues
- [issue]: [location in plan] — [recommendation]

### Minor Issues
- [issue]: [location in plan] — [recommendation]

### Looks Good
- [what is well-designed and why]
```

#### Spec Reviewer

**Agent type:** general-purpose

**Prompt:**
```
You are a specification reviewer. Ensure the implementation plan has complete,
verifiable success criteria. Do not critique the design.

Research doc: [path]
Plan file: [path]

Review for:
1. Missing integration tests — does each phase explicitly call out integration
   test coverage? Unit tests alone are not sufficient.
2. Missing e2e tests — if there is a separate integration/ or e2e/ test directory
   implied by the tech stack, are tests for it specified?
3. Vague success criteria — flag any criteria that can't be verified by running
   a specific command or following a specific manual step.
4. Missing edge cases — what failure modes or boundary conditions are not covered
   in the testing strategy?
5. Incomplete manual steps — are manual verification steps specific enough for
   someone unfamiliar with the feature to execute?

Output format:

## Spec Review

### Missing Coverage
- [gap]: [which phase] — [specific test or criterion needed]

### Vague Criteria
- [criterion]: [why it's ambiguous] — [how to make it specific]

### Looks Good
- [what is well-specified]
```

Wait for **both** reviewers to complete before proceeding.

---

## Step 4: Synthesis & Final Review

### 4a. Read all outputs

Read the draft plan file and both reviewer outputs fully.

### 4b. Resolve any `[OPEN:]` items

If the draft agent flagged open questions, resolve them now. If you can't resolve
from context, use **AskUserQuestion** before revising.

### 4c. Reconcile conflicts

If design and spec reviewers disagree, prefer the recommendation that serves the
user's stated goals from Step 2. When genuinely ambiguous, use **AskUserQuestion**.

### 4d. Revise the plan

Apply findings to the plan file:
- All Critical Issues from design review → fix or explicitly defer with rationale
- All Missing Coverage gaps from spec review → add the missing criteria
- Minor Issues → apply judgment based on scope; skip if not worth the complexity

### 4e. Create review todo

```bash
hive todo add \
  --title "Review plan: <description>" \
  --uri "review://.hive/plans/<filename>"
```

---

## Step 5: Present to User

```
Implementation plan written to:
  `.hive/plans/YYYY-MM-DD-description.md`

Design review: [N critical issues fixed, M minor issues addressed]
Spec review: [N coverage gaps closed]

Please review and let me know:
- Are the phases properly scoped?
- Are the success criteria specific enough?
- Any technical details that need adjustment?
- Missing edge cases or considerations?
```

Iterate based on feedback until the user is satisfied.

---

## Appendix: Plan Template

Use this template in the Wave 1 draft agent prompt and for the final plan file.

```markdown
# [Feature/Task Name] Implementation Plan

## Overview

[Brief description of what we're implementing and why]

## Current State Analysis

[What exists now, what's missing, key constraints — drawn from research doc]

## Desired End State

[Specification of the desired end state and how to verify it]

### Key Discoveries:

- [Important finding with file:line reference]
- [Pattern to follow]
- [Constraint to work within]

## What We're NOT Doing

[Explicitly list out-of-scope items to prevent scope creep]

## Implementation Approach

[High-level strategy and reasoning — the approved approach from user interview]

## Phase 1: [Descriptive Name]

### Overview

[What this phase accomplishes]

### Changes Required:

#### 1. [Component/File Group]

**File**: `path/to/file.ext`
**Changes**: [Summary of changes]

```[language]
// Specific code to add/modify
```

### Success Criteria:

#### Automated Verification:

- [ ] [Command that proves this phase works]: `make test-X`
- [ ] Linting passes: `make lint`
- [ ] Type checking passes: `make typecheck`

#### Manual Verification:

- [ ] [Specific step with expected observable outcome]
- [ ] No regressions in [specific related feature]

**Implementation Note**: After completing this phase and all automated verification
passes, pause here for manual confirmation from the human before proceeding.

---

## Phase 2: [Descriptive Name]

[Similar structure...]

---

## Testing Strategy

### Unit Tests:
- [What to test]
- [Key edge cases]

### Integration Tests:
- [Cross-component scenarios]
- [External dependency interaction tests]

### E2E Tests:
- [Full workflow scenarios]
- [Location: `tests/e2e/` or equivalent]

### Manual Testing Steps:
1. [Specific step to verify feature]
2. [Edge case to test manually]

## Performance Considerations

[Any performance implications or optimizations needed]

## Migration Notes

[If applicable, how to handle existing data/systems]

## References

- Original ticket: `[ticket reference]`
- Research doc: `.hive/research/[filename]`
- Similar implementation: `[file:line]`
```

---

## Guidelines

### No Open Questions in the Final Plan
- Resolve all ambiguity in Step 2 before dispatching
- Sub-agents document gaps as `[OPEN: description]` rather than guessing
- Main agent resolves all `[OPEN:]` markers before presenting to user

### Success Criteria Format

**Automated Verification** (runnable by execution agents):
- Commands: `make test`, `npm run lint`, etc.
- Specific files that should exist
- Code compilation/type checking

**Manual Verification** (requires human):
- UI/UX functionality
- Performance under real conditions
- Edge cases that are hard to automate
- User acceptance criteria

### Common Patterns

**Database Changes**
Schema/migration → store methods → business logic → API → clients

**New Features**
Data model → backend logic → API endpoints → UI last

**Refactoring**
Document current behavior → incremental changes → migration strategy
