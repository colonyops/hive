---
name: plan-to-hc
description: >
  Create a hive hc epic from the current plan file, inserting the ENTIRE PLAN TEXT
  into the epic and creating subtasks to track all work items.
allowed-tools: "Read,Bash(hive hc:*),Bash(echo:*),Bash(bpcopy:*)"
version: "1.0.0"
author: "User"
license: "MIT"
---

# Plan to Hive HC

Converts a Claude Code plan file into a hive hc epic with subtasks.

## Instructions

When this command is invoked:

1. **Locate the most recent (active) plan file**: Read the current plan file from the plans directory

2. **Extract the plan content**: Get the ENTIRE TEXT of the plan

3. **Create a hive hc epic**: Use `hive hc create --type epic` to create an epic with:
   - A descriptive title based on the plan's goal
   - The complete plan text in the description

4. **Create subtasks**: Analyze the plan and create subtasks for each major work item or implementation step using `hive hc create --parent <epic-id>` with:
   - Clear, actionable titles
   - Parent reference to the epic
   - Appropriate status and labels

5. **Link dependencies**: If the plan has sequential steps, link the subtasks with appropriate dependencies

6. **Copy the plan id**: Put the epic id on the clipboard
   - echo <plan id> | bpcopy

## Expected Workflow

```bash
# User runs:
/plan-to-hc

# Agent should:
# 1. Read the plan file
# 2. Create epic with full plan text
# 3. Create subtasks for each work item
# 4. Link dependencies as needed
```

## Notes

- The epic description should contain the COMPLETE plan text for reference
- Subtasks should capture all trackable work items from the plan
- Use clear, descriptive titles for easy identification
- Preserve any dependencies or sequential ordering from the plan
