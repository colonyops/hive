---
icon: lucide/folder-symlink
description: Share plans and research between sessions in Hive Drive, and review them with hive review.
---

# Hive Drive & Review

Hive Drive is the folder tree where plans, research notes, and other documents live. Every session from the same repository reads and writes the same documents. `hive review` lets you annotate them with inline comments.

`hive ctx` is the old name of `hive drive` and still works.

## The Workflow

1. **Research** -- gather information, explore the codebase, and store findings in `.hive/research/`
2. **Plan** -- write implementation plans in `.hive/plans/` that any agent can reference
3. **Iterate** -- review and annotate plans with `hive review`, add comments, refine before coding
4. **Build** -- execute the plan, referencing shared context across sessions

An agent working on authentication can read the plan written by the agent that did the initial research. There is no copy-pasting and no context loss between sessions.

## The Drive

The drive root is `$XDG_DATA_HOME/hive/context/` (`~/.local/share/hive/context/` by default). Set `context.base_dir` in the [configuration](../configuration/index.md) to move it, for example into a [git repository](../recipes/git-backed-context.md).

A space is a folder under the root:

| Space | Folder |
| --- | --- |
| A repository | `<root>/<owner>/<repo>/` |
| A Chats workspace in Hive Desktop | `<root>/workspaces/<name>/` |
| Shared across everything | `<root>/shared/` |

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
└── references/                           # Reference material
```

Hive Desktop shows the same tree in its Drive view, so a file you write from a terminal, an editor, or Obsidian appears there, and the reverse. See [Chats](../../desktop/chats/agent-workspaces.md#drive) and [Code](../../desktop/code/terminal-mode.md#drive).

### Initialization

Initialize the drive folder for your repository:

```bash
# Creates the .hive symlink in the current directory
# and the plans/, research/, and references/ folders
hive drive init
```

!!! warning
    `.hive/` must ONLY be a symlink, never a regular directory. Always use `hive drive init` to create it. A regular directory breaks sharing between sessions.

### Listing Documents

!!! tip
    Use `hive drive ls` to list documents. Standard `ls` and glob tools do not follow the `.hive` symlink reliably.

`hive drive ls` prints the repository's space with a kind for each entry: `dir`, `markdown`, `html`, `canvas`, or `file`. The kind comes from the file extension. Folders come before files at each level. `.canvas.json` is a canvas, a block document that agents write in Hive Desktop.

```
$ hive drive ls
/Users/you/.local/share/hive/context/my-org/my-repo
dir       canvases
canvas    canvases/perf.canvas.json
dir       plans
markdown  plans/2026-01-15-auth-refactor.md
dir       research
markdown  research/authentication-analysis.md
html      research/benchmarks.html
markdown  notes.md
```

```bash
hive drive ls --all      # every space, each under its key
hive drive ls --json     # one JSON entry per line
hive drive spaces        # the space keys
hive drive prune --older-than 30d
```

`hive drive spaces` prints each space's kind and key, such as `repository  my-org/my-repo`, `workspace  workspaces/hive`, and `shared  shared`. Another repository or the shared space is `hive drive -r owner/repo ls` or `hive drive -s ls`; `--all` cannot combine with `-r` or `-s`. `prune` removes files older than the given age.

### Finding Documents

```bash
# Review with interactive picker
hive review

# Review every space, with the space shown in the picker
hive review --all

# Review specific document
hive review -f .hive/plans/2026-01-15-auth-refactor.md

# Review most recent document
hive review --latest
```

### Document Types

The picker groups documents by type. A document's type is the `type:` key in its front matter. Without one, the top folder gives the type: `plans` is `plan`, `design-docs` is `design-doc`, `reviews` is `review`, `references` is `reference`, and any other folder is its own type. A document in no folder has the type `other`.

```markdown
---
type: design-doc
---
```

The picker lists these types first, in this order: `plan`, `design-doc`, `research`, `review`, `reference`, `context`. Other types follow alphabetically, and `other` comes last. Within a type, the newest document comes first. Only markdown files are listed.

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

Review and annotate markdown documents stored in the drive. Opens an interactive document picker when run without arguments, or directly reviews a specified file.

<video autoplay loop muted playsinline loading="lazy" style="width: 100%; border-radius: 8px;">
  <source src="https://github.com/user-attachments/assets/1bab8be5-c01e-441f-9dc2-4345737bec24" type="video/mp4">
</video>

```bash
hive review                          # Interactive picker
hive review -f .hive/plans/auth.md   # Review specific file
hive review --latest                 # Review most recent document
```

### Interactive Features

- **Document Picker** — Fuzzy search through documents (only available when multiple documents exist)
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
