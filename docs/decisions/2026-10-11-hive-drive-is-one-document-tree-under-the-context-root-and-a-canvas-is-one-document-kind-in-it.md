# Hive Drive is one document tree under the context root, and a canvas is one document kind in it

- **Status:** accepted
- **Date:** 2026-10-11

## Context

Agent-written documents live in two stores with two rulebooks. The engine's
`internal/hive/repocontext` owns the context root: `<ctx>/<owner>/<repo>/`
for each repository, the directory a checkout links as `.hive`, and
`<ctx>/shared/`. Agents, skills, and Obsidian write markdown there straight
to disk; `hive ctx ls` and `hive review` read it. The desktop's
`cmd/desktop/internal/app/canvas.Store` owns canvases: JSON block files
written over MCP, read by the pane and the full-page view, never watched.

The canvas store files under three roots. A Code session's canvases go in
the context root at `<ctx>/<owner>/<repo>/canvases/` (ADR
a-code-session-s-canvases-belong-to-its-repository-and-live-in-the-hive-context-directory).
A chat's go in its workspace folder at `<workspace>/canvases/`. Canvases
with no owner go in the desktop's data directory at `<data>/canvases/` under
the `@global` key (ADR agents-outside-every-session-share-one-global-canvas-owner).

So an agent has to know which system its output belongs to, the user has no
one place to browse, search, or reorganize what agents wrote, and the
desktop cannot see the hundreds of markdown documents already in the
context tree. `@global` and `shared` are the same idea twice. Issue #506
asks for folders and user reorganization of canvases; this decision answers
it for every document kind at once.

## Decision

**The context root is the drive root, and `.hive` is a space.** A drive is
spaces: one per repository at `<ctx>/<owner>/<repo>/`, one per Chats
workspace at `<ctx>/workspaces/<name>/`, and one shared at `<ctx>/shared/`.
A space's key is its path under the root. A folder is a directory in a
space, with no metadata beside it, so Obsidian, `ls`, and the desktop agree
on the layout. A document is a file whose extension names its kind: `.md`,
`.html`, or `.canvas.json`; any other file is listed as a plain file. A
document is identified by `(space, path)` with no stable id, so a move is a
rename. Nothing already in the root moves, and the `.hive` symlink keeps
pointing where it does.

**A workspace's space lives in the root, not in the workspace folder.** The
workspace folder mixes files the user authored with files Hive generated,
and Hive must not list, move, or delete the user's from a drive view. A
chat's canvases move to `<ctx>/workspaces/<name>/canvases/`. This leaves the
drive with one root, which is what makes a later sync a sync of one
directory. Deleting a workspace (ADR deleting-a-workspace-deletes-its-directory)
leaves its space, and a workspace renamed by hand leaves its space behind as
a leftover the user can move or delete.

**`@global` becomes the shared space.** Global canvases move from
`<data>/canvases/` to `<ctx>/shared/canvases/`. The MCP `session: "global"`
argument stays and resolves to the shared space. The `@global` store is
gone. Canvas tool results keep `@global` as the owner key of a shared
canvas, for compatibility with agents and links that match on it. The shared space has no pane: `open_document`, `open_canvas` and
`close_canvas` refuse it, and the user reads it in the Drive page.

**A canvas file is `<name>.canvas.json`.** A space holds files Hive did not
write, so `.json` alone cannot mean canvas. The on-disk shape and the ten
block tools do not change. The `canvas` name an agent passes may now be a
path: `reports/2026-10/perf` files at `reports/2026-10/perf.canvas.json`,
and a bare name still files under `canvases/`.

**An HTML document runs in a sandboxed iframe, as written. An HTML block
does not.** A block sits in the app's DOM beside the Wails bindings and
stays as ADR a-canvas-html-block-is-restricted-by-what-it-can-reach-not-by-how-it-looks
left it. A document is a whole page that owns its look and fills the pane,
so the two reasons ADR canvas-html-blocks-are-sanitized-in-go-and-styled-by-an-app-owned-class-vocabulary
rejected an iframe for blocks do not apply. The document renders in
`<iframe sandbox="allow-scripts" srcdoc=...>` with no `allow-same-origin`,
and Hive injects a CSP `<meta>` that loads scripts, styles, and fetches
over `https:` only and allows `data:` only for images, fonts, and media, so
the page cannot call Hive's bindings or send requests to localhost.
Isolation, not sanitization, is the boundary for a document. Hive parses nothing out of an
HTML document: no front matter, no title.

**One engine service owns the tree; the desktop renders it.**
`internal/domain/drive` holds the model and `internal/hive/drive` the
service: spaces, the snapshot, documents, folders, canvases, the watch. It
absorbs `repocontext` and the desktop `canvas.Store`. Both programs read
this tree, which makes it the engine's job (ADR
both-programs-run-on-one-layered-hive-engine). The HTML sanitizer, the CSP
injection, and markdown export stay in the desktop, because the CLI never
renders.

**The engine holds a snapshot and refreshes it from one watch per
directory.** The snapshot holds path, kind, size, times, and parsed front
matter, and answers every listing, search, and properties read without
touching a file. fsnotify has no public recursive watch, so the service adds
one watch per directory, adds one when a directory appears, and debounces at
100 ms. On an event it re-reads the affected directory from disk and diffs
it into the snapshot instead of applying the event delta, because coalesced
and dropped events make deltas drift. Dot directories are excluded from the
scan and the watch. A change publishes `DriveChanged` with the space and the
changed paths; the desktop degrades it to a `drive:updated` wake-up and
re-reads the subtree.

The scan is lazy, synchronous, and in memory, not a scan of the whole root
at start: the first `Tree` or `Search` on a space scans that space. The real
root walks in 4 ms and head-reads every markdown file in 12 to 54 ms, and a
one-shot CLI command should not pay for spaces it never reads. No index
lives in `hive.db`.

## Consequences

- Three file moves, each idempotent and each a rename on one filesystem.
  The engine renames `<space>/canvases/<name>.json` to `<name>.canvas.json`
  before the first scan of a space, only when the JSON has a `blocks` key.
  The service records the spaces it migrated, so the rename runs once per
  space and a later rescan does not repeat it. The
  desktop, after the engine starts, moves `<data>/canvases/*.json` to
  `<ctx>/shared/canvases/` and `<workspace>/canvases/*.json` to
  `<ctx>/workspaces/<name>/canvases/`, because only it knows those paths.
  A `.json` that does not parse as a canvas stays and lists as a plain file;
  a target that already exists is skipped and logged. No `hive.db`
  migration is involved.
- `workspaces` and `shared` are reserved names under the root. A GitHub
  owner with either name collides, the risk `shared` already carried.
- HTML has two tiers. A canvas html block is sanitized in Go, carries no
  script, and is held in by `contain: layout`. An `.html` document keeps its
  script and runs in an opaque-origin iframe under a CSP Go injects, so the
  frontend holds no policy for either tier. The frame ships with
  `allow-scripts` only, so links inside an html document stay inert until a
  later change verifies `allow-popups` on WKWebView.
- `hive ctx` becomes an alias of `hive drive`, because it is a public CLI
  contract in users' rules, skills, and `CLAUDE.md` files. `/mcp/canvas`
  keeps answering for the same reason.
- `internal/hive/repocontext` and the desktop `canvas.Store` are deleted.
- A move breaks a link to the old path, as it does in every editor already
  open on this tree.
- On macOS fsnotify opens a descriptor per watched file, about 1,200 for
  the current root. The service holds the concrete `*fswatch.Watcher`. Its
  backend-neutral contract is the `Change{Dirs}` value with `Changes()` and
  `Close()`; an interface waits for a second backend, such as FSEvents.
