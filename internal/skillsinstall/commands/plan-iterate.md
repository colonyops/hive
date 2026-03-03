---
allowed-tools: Bash(git status:*), Bash(git log:*), Bash(git diff:*)
description: Review and iterate on an existing task plan, discussing trade-offs and design decisions with the user.
---

## Overview

Review an existing plan from `.hive/plans/`, analyze its quality and completeness, and collaborate with the user to improve it through discussion of trade-offs and design decisions.

If a specific plan file is provided as an argument, use that file. Otherwise, find the latest plan in `.hive/plans/` (sorted by filename, which uses YYYY-MM-DD prefix).

## Review Process

### 1. Initial Assessment

Read and evaluate the current plan against these criteria:

- **Clarity**: Are the steps specific enough to execute without ambiguity?
- **Completeness**: Does the plan cover all aspects of the task?
- **Sequencing**: Are dependencies properly ordered? Are there steps that could be parallelized?
- **Risk Identification**: Are potential failure points and edge cases addressed?
- **Testability**: Does each step have clear validation criteria?
- **Scope**: Is the plan appropriately scoped, or does it over/under-engineer the solution?

### 2. Codebase Alignment Check

Verify the plan against the current repository state:

- Confirm referenced files and paths still exist
- Check for recent commits that may affect the plan
- Identify any new context that should inform the approach
- Flag any assumptions that may no longer hold

### 3. Gap Analysis

Identify what's missing or could be improved:

- Missing error handling considerations
- Unaddressed edge cases
- Opportunities for better abstraction or reuse
- Testing gaps
- Documentation needs

## Interactive Discussion Process

**After presenting your initial assessment, use AskUserQuestion tool to conduct a structured interview:**

### Architecture & Design Decisions

Use AskUserQuestion to gather input on:
- Alternative approaches you've identified - which aligns better with their goals?
- Constraints not mentioned in the plan (performance, backwards compatibility, etc.)
- Scope boundaries—what's explicitly out of scope?

### Implementation Trade-offs

Use AskUserQuestion to clarify:
- Trade-offs in the current approach (simplicity vs. flexibility, speed vs. maintainability)
- Acceptable compromises given timeline or resource constraints
- Build-vs-buy decisions for any components

### Integration Concerns

Use AskUserQuestion to determine:
- How this work should interact with existing systems
- Rollout strategy (feature flags, gradual rollout, etc.)
- Backwards compatibility requirements

### Validation & Success Criteria

Use AskUserQuestion to confirm:
- What "done" looks like from the user's perspective
- Priority of different requirements if trade-offs are needed
- Acceptable test coverage levels

**Important:** Group related questions (max 4 per AskUserQuestion call). Make multiple calls if needed to cover all categories.

## Output Format

Structure your review as:

1. **Summary of Current Plan**
   - Brief overview of what the plan proposes
   - Overall assessment (1-2 sentences)

2. **Strengths**
   - What the plan does well
   - Approaches worth preserving

3. **Gap Analysis**
   - What's missing or unclear
   - Potential issues identified
   - Areas needing clarification

4. **Interactive Interview**
   - **Use AskUserQuestion tool** to gather structured responses
   - Group questions by category (Design, Trade-offs, Integration, Validation)
   - Make multiple AskUserQuestion calls if needed (max 4 questions per call)
   - Explain context for each question category before asking

5. **Suggested Improvements** (after interview)
   - Based on user responses, propose specific changes
   - Offer to update the plan with agreed-upon modifications
   - Note areas you'll address based on their input

## Instructions

1. Read the plan file thoroughly before making any assessments
2. Explore relevant parts of the codebase to validate your understanding
3. **Use AskUserQuestion tool to conduct structured interviews**—user input should shape your recommendations
4. Be specific in your questions; avoid generic queries
5. Group related questions together (max 4 per AskUserQuestion call)
6. After gathering all responses, propose specific improvements
7. Offer to update the plan with agreed-upon changes
8. Preserve the original plan structure unless the user wants it reorganized
