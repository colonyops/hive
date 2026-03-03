---
name: obsidian
description: >
  Save documents to Obsidian notebook. Use when the user asks to save, write, or put
  a document (test plan, design doc, etc.) into their Obsidian notebook.
allowed-tools: "Bash(mkdir:*),Write,Read"
version: "1.1.0"
author: "User"
---

# Obsidian Notebook

Save structured documents into the user's Obsidian vault.

## Vault Location

The vault root is stored in `$OBSIDIAN_NOTEBOOK_DIR`. Always resolve this before writing:

```bash
echo "$OBSIDIAN_NOTEBOOK_DIR"
```

If the variable is empty, stop and tell the user to set `OBSIDIAN_NOTEBOOK_DIR`.

## Document Types

When saving a `design-doc` or `research` document, check whether a project context is
available (the user mentions a project, or the document is being created as part of
`project-advance`). If a project is known, save under the project folder. Otherwise fall
back to the vault-root folder.

| Type       | With project context                                    | Without project context   | Filename Pattern              |
| ---------- | ------------------------------------------------------- | ------------------------- | ----------------------------- |
| test-plan  | _(no project variant)_                                  | Test Plans                | `YYYY-MM-DD-<slug>.md`        |
| design-doc | Projects/\<Project Name\>/Design Docs                   | Design Docs               | `YYYY-MM-DD-<slug>.md`        |
| research   | Projects/\<Project Name\>/Research                      | Research                  | `YYYY-MM-DD-<slug>.md`        |
| project    | Projects/\<Project Name\>                               | _(always project-scoped)_ | `<Project Name>.md`           |
| work-item  | Projects/\<Project Name\>/Work Items                    | _(always project-scoped)_ | `<Work Item Name>.md`         |

For `project` and `work-item` types, use the `project-draft` skill instead — it handles the full interview and folder structure automatically.

If the user requests a type not in this table, ask which folder to use and suggest they add it to this table.

## Writing a Document

1. **Resolve the vault path** from `$OBSIDIAN_NOTEBOOK_DIR`
2. **Determine the document type** from user intent
3. **Create the target folder** if it doesn't exist:
   ```bash
   mkdir -p "$OBSIDIAN_NOTEBOOK_DIR/<Folder>"
   ```
4. **Generate the filename**: `YYYY-MM-DD-<slugified-title>.md` using today's date
5. **Write the file** using the Write tool with the frontmatter and content

## Frontmatter

Every document must start with YAML frontmatter:

```yaml
---
title: "<Document Title>"
repository: "<owner>/<repo>"
created: YYYY-MM-DD
status: draft
---
```

Field details:
- **title**: Descriptive title for the document
- **repository**: Infer from the current git remote (`git remote get-url origin`, extract `owner/repo`). If not in a git repo, ask the user.
- **created**: Today's date
- **status**: Always `draft` unless the user specifies otherwise

## Slug Generation

Convert the title to a filename-safe slug:
- Lowercase
- Replace spaces and non-alphanumeric characters with hyphens
- Strip leading/trailing hyphens

Example: `Auth Service Redesign` -> `auth-service-redesign`

## Creating hive todos for Obsidian documents

After saving a document, if a `hive todo` item should be created pointing to it, use the
`obsidian://vault/<VaultName>/<url-encoded-path>` URI scheme. The vault name is the last
path component of `$OBSIDIAN_NOTEBOOK_DIR`.

```bash
# Derive vault name from $OBSIDIAN_NOTEBOOK_DIR
VAULT=$(basename "$OBSIDIAN_NOTEBOOK_DIR")

hive todo add \
  --title "Review <document title>" \
  --uri "obsidian://vault/${VAULT}/Projects/Foo%20Bar/doc.md"
```

- Spaces in path segments must be percent-encoded as `%20`.
- Do **not** use bare `obsidian://Projects/...` — the vault name is required.

## Example

User says: "Save this test plan for the auth refactor to my obsidian notebook"

Result: `$OBSIDIAN_NOTEBOOK_DIR/Test Plans/2026-02-18-auth-refactor-test-plan.md`

```markdown
---
title: "Auth Refactor Test Plan"
repository: "hay-kot/hive"
created: 2026-02-18
status: draft
---

<content from the conversation>
```
