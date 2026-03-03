---
---

**Instructions for the CLI / Engineer:**

1. **Prompt for Workflow Details**
   - Ask the user for:
     - Workflow title
     - Description / purpose
     - Key commands, steps, and notes
     - Slack context or external references

   _Only ask for missing information if needed._

2. **Generate Markdown File**
   - Use the `add-how-to` CLI to create the file:

     ```bash
     add-how-to "Workflow Title" -
     ```

   - The CLI will create the Markdown file in `$OBSIDIAN_HOW_TO_DIR`.
   - Keep the document as brief and clear as possible.
   - Your target audience is engineers who work on these systems.

3. **Structure the Markdown using this template**

```markdown
---
tags:
  - how-to
  - devops
  - engineering
created: YYYY-MM-DD
updated: YYYY-MM-DD
---

# How To [Workflow Title]

## Overview

Goal: Describe what this guide helps achieve.

## Prerequisites

- [ ] Requires Time Access

## Steps

### 1. Prepare

### 2. Execute

### 3. Verify

## Notes & Gotchas

Add common issues, Slack conversations, or troubleshooting tips.

## External Resources

List useful references or documentation links.
```
