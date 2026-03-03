---
name: design-doc
description: >
  Generate a design doc from template. Use when the user asks to create, draft, or
  write a design doc, design proposal, or technical proposal.
allowed-tools: "Read,Write,Bash(mkdir:*),Bash(git:*),Bash(echo:*)"
version: "1.0.0"
author: "User"
---

# Design Doc Generator

Generate design documents from a team template, saving to the Obsidian notebook by default.

## Template Location

The template lives at a path relative to this skill:

```
.ai/skills/design-doc/templates/design-doc.md
```

Read the template file before generating. If the template is missing, tell the user it needs to be created (it is gitignored and must be set up locally).

## Workflow

1. **Read the template** from the path above
2. **Gather information** from the user and codebase:
   - Title of the design
   - Background and problem statement
   - At least two proposals (including "do nothing")
3. **Research the codebase** to inform the proposals — understand existing architecture, relevant code, and constraints
4. **Fill in the template** replacing placeholders:
   - `{{TITLE}}` — design doc title
   - `{{AUTHOR}}` — infer from git config (`git config user.name`) or ask
   - `{{DATE}}` — today's date in display format (e.g. `Feb 18, 2026`)
   - `{{PROPOSAL_NAME}}` — name of the first real proposal
5. **Present the draft** to the user for review before saving
6. **Save to Obsidian** — this is the default. Before saving, read the `obsidian` skill and follow its conventions for vault resolution, frontmatter, slug filenames, and folder creation. Save to the `Design Docs` folder. Only save elsewhere if the user explicitly requests it.

## Audience

Assume readers work on the same codebase or a related codebase. They have general context on the project but not the specifics of what this design addresses. Don't over-explain the system — focus on the specific problem and proposals.

## Writing Style

- **Be brief.** Readers' time is valuable. Use only enough text to make each point clear and useful.
- Use tables, bullet points, and lists to aid readability over prose paragraphs.
- Keep language direct and concise — no filler, no hedging.

## Section Guidelines

- **Background**: Brief context on the relevant part of the system, enough to frame the problem
- **Problem**: Be specific about the pain point and its impact
- **Goals**: Distinguish must-haves from nice-to-haves
- **Proposals**: Each proposal should include:
  - A clear description of the approach
  - Trade-offs (pros/cons) — tables work well here
  - Rough scope/effort
  - Risks or unknowns
- **Proposal 0 (Do nothing)**: Always include — describe the cost of inaction
- Add additional proposal sections as needed (`### Proposal 2: ...`, etc.)
