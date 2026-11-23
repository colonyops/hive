---
icon: lucide/blocks
description: Components and wikilinks an agent can put in a Hive Drive markdown document, and how they render on a share link.
---

# Fences and Links

A Drive markdown document can hold three fenced components and links to other documents. Agents write them, and the `hive-drive` MCP server tells them the format. You can write them by hand too.

Every fence body is YAML. A key Hive does not know is ignored. A body that does not parse, or lacks a required key, shows an error above the raw source, so the document still reads. On GitHub, in Obsidian, and in a terminal the fence stays a readable code block.

## hive-tree

Shows changed files as a collapsible tree. Write a flat list of paths and Hive builds the folders. `root` is a prefix shown as the top folder. Each file has a `path` and, optionally, `status` (`added`, `modified`, `deleted`, or `renamed`), `from` (the old path of a rename), and `note`. A path that is a Drive document becomes a link.

````markdown
```hive-tree
root: web/packages/ui/src
files:
  - path: lib/githubMarkdown.ts
    status: modified
    note: split on any registered fence
  - path: components/CanvasMermaid.vue
    status: deleted
```
````

## hive-calls

Shows a call tree. Click a row, or move with the arrow keys, to read its explanation in the strip under the tree. Each call has a `name` and, optionally, `at` (`path:line`, a path, or a package name), `kind` (`function`, `method`, `component`, `computed`, or `external`), `note` (the one-line text in the row), `explain` (the text in the strip), `focus` (start selected), `more` (a count of calls left out), and `calls` (the children).

````markdown
```hive-calls
name: CanvasMarkdown.parts
at: web/packages/ui/src/components/CanvasMarkdown.vue:22
calls:
  - name: isFence
    at: web/packages/ui/src/components/fences/registry.ts:31
    focus: true
    explain: Decides whether a top-level fence becomes a component.
```
````

## hive-forge

Shows a list of pull request and issue cards from GitHub, Gitea, Forgejo, or GitLab. The data is a snapshot the agent wrote into the document. Nothing is fetched when you open it, so the cards can be out of date. `snapshot` records when the agent took the data.

Each item needs `kind` (`pr` or `issue`), `ref` (such as `owner/repo#123`), `title`, and `state` (`open`, `draft`, `merged`, or `closed`). `url`, `author`, `checks`, `diff`, `comments`, `labels`, and `updated` are optional. `host` sets the default host for the list.

!!! tip "Write url for other forges"
    Without `url`, Hive builds the link with GitHub's layout. Write `url` for any host other than github.com.

````markdown
```hive-forge
snapshot: 2026-11-22T12:30
items:
  - kind: pr
    ref: infra/deploy#212
    url: https://gitea.example.com/infra/deploy/pulls/212
    title: Pin the runner image
    state: merged
    checks: 6/6
```
````

## Dependency graphs

There is no fence for graphs. Use a Mermaid `flowchart LR` fence.

## Wikilinks

Link to another document in the same space with double brackets.

| Write | Links to |
| --- | --- |
| `[[plans/auth-rewrite]]` | The document at that path. `.md` is optional. |
| `[[plans/auth-rewrite\|the plan]]` | The same document, shown as "the plan". |
| `[[auth-rewrite]]` | The one document with that file name. |

Inside a table cell, write the alias pipe as `\|`, as in the table above.

Hive tries the exact path first, then a file name that is unique in the space. A link that matches nothing shows as plain text. Wikilinks work only in Drive documents. In chat messages, task text, and release notes, `[[...]]` stays as written.

## On a share link

A [share link](../../cli/drive-server/sharing.md) renders all three fences the same way, because their data is in the document. A share link holds one document, so wikilinks show as plain text there.
