---
icon: lucide/folder-symlink
---

# Context & Review

Context directories give all sessions from the same repository access to shared documents — plans, research notes, and other artifacts. The review tool lets you annotate these documents with inline comments.

## The Workflow

Hive's context directories and review tool support a structured workflow for developing code with AI agents:

1. **Research** — Gather information, explore the codebase, and store findings in `.hive/research/`
2. **Plan** — Write implementation plans in `.hive/plans/` that any agent can reference
3. **Iterate** — Review and annotate plans with `hive review`, add comments, refine before coding
4. **Build** — Execute the plan with confidence, referencing shared context across sessions

Context directories make this possible by giving all sessions from the same repository access to the same documents. An agent working on authentication can read the plan written by the agent that did the initial research — no copy-pasting, no context loss between sessions.

## Context Directories

Context directories provide shared storage for plans, research notes, and other artifacts across all sessions from the same repository. Documents stored in `.hive/` are accessible to all agents working on the repository.

### Directory Structure

```
.hive/                                    # Symlink (per session)
  ↓ points to
~/.local/share/hive/context/{owner}/{repo}/
├── plans/                                # Implementation plans
│   ├── 2026-01-15-auth-refactor.md
│   └── 2026-01-20-api-redesign.md
├── research/                             # Research notes
│   ├── authentication-analysis.md
│   └── performance-profiling.md
└── context/                              # General context documents
    └── architecture-decisions.md
```

### Initialization

Before storing documents, initialize the context directory for your repository:

```bash
# Creates .hive symlink in current directory
hive ctx init

# Verify symlink and list contents
hive ctx ls
```

!!! warning
    `.hive/` must ONLY be a symlink, never a regular directory. Always use `hive ctx init` to create it. Creating a regular directory will break context sharing between sessions.

### Finding Documents

!!! tip
    Always use `hive ctx ls` to list context documents — standard `ls` and glob tools do not follow the `.hive` symlink reliably.

To locate context documents for review:

```bash
# List all context documents (follows symlink correctly)
hive ctx ls

# Review with interactive picker
hive review

# Review specific document
hive review -f .hive/plans/2026-01-15-auth-refactor.md

# Review most recent document
hive review --latest
```

### Usage Patterns

**Planning workflow:**

```bash
# Create plan document
echo "# Auth Refactor Plan" > .hive/plans/auth-refactor.md

# Review and annotate
hive review --latest

# Later sessions can review the same document
cd ~/another-session
hive review -f .hive/plans/auth-refactor.md
```

**Research workflow:**

```bash
# Store research findings
hive review -f .hive/research/api-analysis.md

# Add comments with findings
# > **Comment:** Found bottleneck in user lookup - missing index on email column
```

## Review Tool

Review and annotate markdown documents stored in context directories. Opens an interactive document picker when run without arguments, or directly reviews a specified file.

```bash
hive review                          # Interactive picker
hive review -f .hive/plans/auth.md   # Review specific file
hive review --latest                 # Review most recent document
```

### Interactive Features

- **Document Picker** — Fuzzy search through context documents (only available when multiple documents exist)
- **Line Selection** — Navigate and select specific lines with visual feedback
- **Inline Comments** — Add multiline comments at any line with automatic indentation
- **Smart Wrapping** — Comments wrap at 80 characters preserving indentation
- **Search** — `/` to search within document, `n/N` to navigate matches
- **Persistence** — Comments are saved directly to the file

### Keyboard Navigation

| Key                  | Action                               |
| -------------------- | ------------------------------------ |
| `↑/k`                | Move up                              |
| `↓/j`                | Move down                            |
| `g`                  | Jump to top                          |
| `G`                  | Jump to bottom                       |
| `enter`              | Select line and add comment          |
| `/`                  | Search in document                   |
| `n/N`                | Next/previous search match           |
| `esc`                | Cancel comment/search, back to picker |
| `q`/`ctrl+c`         | Quit                                 |

### Comment Format

Comments are inserted with proper markdown formatting and indentation alignment:

```markdown
│  42 │ ## Implementation Steps

       > **Comment:** This section needs to address the authentication flow.
       > The current approach doesn't handle token refresh properly.

│  43 │ 1. Add OAuth2 client configuration
```
