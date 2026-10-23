# Hive Drive is one document tree under the context root, and a canvas is one document kind in it

- **Status:** accepted
- **Date:** 2026-10-11

## Context

Agents write two kinds of documents: markdown files in the context root
(`<ctx>/<owner>/<repo>/`, the directory a checkout links as `.hive`, written by
agents, skills, and Obsidian straight to disk) and canvases written over MCP.
With two stores, an agent has to know which system its output belongs to, and
the user has no one place to browse, search, or reorganize what agents wrote.

## Decision

**The context root is the drive root, and `.hive` is a space.** A drive is
spaces: one per repository at `<ctx>/<owner>/<repo>/`, one per Chats
workspace at `<ctx>/workspaces/<name>/`, and one shared at `<ctx>/shared/`.
A space's key is its path under the root. A folder is a directory in a
space, with no metadata beside it, so Obsidian, `ls`, and the desktop agree
on the layout. A document is a file whose extension names its kind: `.md`,
`.html`, or `.canvas.json`; any other file is listed as a plain file. A
document is identified by `(space, path)` with no stable id, so a move is a
rename.

**A workspace's space lives in the root, not in the workspace folder.** The
workspace folder mixes files the user authored with files Hive generated, and
Hive must not list, move, or delete the user's from a drive view. One root also
means a later sync is a sync of one directory. Deleting a workspace (ADR
deleting-a-workspace-deletes-its-directory) leaves its space, and a workspace
renamed by hand leaves its space behind for the user to move or delete.

**The shared space replaces a separate global canvas owner.** The MCP
`session: "global"` argument resolves to the shared space. Canvas tool results
keep `@global` as the owner key of a shared canvas, for agents and links that
match on it. The shared space has no pane: `open_document`, `open_canvas` and
`close_canvas` refuse it, and the user reads it in the Drive page.

**A canvas file is `<name>.canvas.json`.** A space holds files Hive did not
write, so `.json` alone cannot mean canvas. The `canvas` name an agent passes
may be a path: `reports/2026-10/perf` files at
`reports/2026-10/perf.canvas.json`, and a bare name files under `canvases/`.

**An HTML document runs in a sandboxed iframe, as written. An HTML block
does not.** A block sits in the app's DOM beside the Wails bindings and stays
as ADR a-canvas-html-block-is-restricted-by-what-it-can-reach-not-by-how-it-looks
describes. A document is a whole page that owns its look and fills the pane,
so the reasons ADR
canvas-html-blocks-are-sanitized-in-go-and-styled-by-an-app-owned-class-vocabulary
gives against an iframe for blocks do not apply. The document renders in an
`<iframe sandbox="allow-scripts">` with no `allow-same-origin`, and Hive
injects a CSP `<meta>` that loads scripts, styles, and fetches over `https:`
only and allows `data:` only for images, fonts, and media, so the page cannot
call Hive's bindings or send requests to localhost. Isolation, not
sanitization, is the boundary for a document. Hive parses nothing out of an
HTML document: no front matter, no title.

**One engine service owns the tree; the desktop renders it.**
`internal/domain/drive` holds the model and `internal/hive/drive` the service:
spaces, the snapshot, documents, folders, canvases, the watch. Both programs
read this tree, which makes it the engine's job (ADR
both-programs-run-on-one-layered-hive-engine). The HTML sanitizer, the CSP
injection, and markdown export stay in the desktop, because the CLI never
renders.

**The engine holds a snapshot and refreshes it from one watch per
directory.** The snapshot holds path, kind, size, times, and parsed front
matter, and answers every listing, search, and properties read without
touching a file. fsnotify has no recursive watch, so `internal/platform/fswatch`
adds one watch per directory, adds one when a directory appears, and debounces
at 100 ms. On a change the service re-reads the affected directory from disk
and diffs it into the snapshot instead of applying the event delta, because
coalesced and dropped events make deltas drift. Dot directories are excluded
from the scan and the watch. A change publishes `drive.changed` with the space
and the changed paths, and the desktop forwards it to the frontend as
`drive:updated`.

The scan is lazy and in memory: the first `Tree` or `Search` on a space scans
that space, so a one-shot CLI command does not pay for spaces it never reads.
No index lives in `hive.db`.

## Consequences

- Legacy canvases move into the drive, idempotently, by rename. Before the
  first scan of a space the engine renames `<space>/canvases/<name>.json` to
  `<name>.canvas.json` when the JSON has a `blocks` key, once per space. The
  desktop moves canvases from its data directory and from workspace folders
  into the shared and workspace spaces, because only it knows those paths. A
  `.json` that is not a canvas stays and lists as a plain file; a target that
  already exists is skipped and logged.
- `workspaces` and `shared` are reserved names under the root. A GitHub owner
  with either name collides.
- A canvas html block is sanitized in Go and carries no script. An `.html`
  document keeps its script and runs in an opaque-origin iframe under a CSP Go
  injects, so the frontend holds no policy for either. The frame has
  `allow-scripts` only, so links inside an html document stay inert.
- `hive ctx` is an alias of `hive drive`, and `/mcp/canvas` keeps answering,
  because both are public contracts in users' rules, skills, and `CLAUDE.md`
  files.
- A move breaks a link to the old path.
- On macOS fsnotify (kqueue) opens a descriptor per watched file, so
  `fswatch` keeps the watches within a descriptor budget.
