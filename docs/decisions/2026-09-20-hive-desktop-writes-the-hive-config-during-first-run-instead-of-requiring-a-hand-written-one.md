# Hive Desktop writes the Hive config during first run instead of requiring a hand-written one

- **Status:** accepted
- **Date:** 2026-09-20

## Context

The session launcher is built from the external Hive config: hive's
`SessionLaunchOptions` lists repositories by scanning `workspaces` and offers
the agents in `agents`. Neither has a useful default. With no config file,
hive's `applyDefaults` invents a single `claude` profile whether or not claude
is installed, and no workspaces at all.

A user who has never run the `hive` CLI therefore reached a new session dialog
with an empty repository list and one agent they may not have. Nothing in the
app said why, and Settings ▸ Hive CLI only reported the file's path and offered
to open it in an editor. The config was required for half the product and was
unreachable except by hand-writing YAML.

The file is shared with the `hive` CLI. It is dotfiles-managed, may carry
rules, keybindings and user commands this app has no opinion on, and `hive
init` writes it too. Anything the desktop does to it has to survive being one
of two writers.

## Decision

Ask for the two values as the first step of first run, and write them: which
agents can start a session, and which parent folders hold the repositories
they run in. First run is the only writer. Settings ▸ Hive CLI keeps pointing
at the file and the hive documentation: a surface that edits the config after
first run needs a strategy for the whole file, not a second form over two keys
of it, and that strategy is not designed yet.

Branch first run on whether the config is **usable** — it declares at least one
agent profile and at least one workspace — not on whether the file exists. A
file that declares neither leaves the launcher exactly as empty as no file at
all. A usable config is confirmed on screen and adopted unchanged.

Read the file's own YAML to decide that, never hive's merged config, so hive's
`claude` fallback is not mistaken for a choice the user made.

Own two keys and no others. Creating a file renders a commented template
holding only those two keys; `hive init`'s template also writes `version`,
`tmux` and `rules`. Editing an existing file edits its parsed node tree in
place (the `flow/yamldoc.go` pattern), so comments, key order, unknown keys,
`agent_selector`, and profiles this build has no control for all survive.
Validate before writing: hive fails the **whole** config when `agents.default`
or a `rules[].agent` names no profile, so a bad write does not degrade the
app, it stops it starting. `hiveconf` checks the keys it owns, and the app runs
hive's own loader over the written candidate before it replaces the file.

Keep the step skippable and the catalog open. The inbox half of the app is not
gated on any of this, and an agent that is not on PATH is still selectable —
a Dock launch's resolved PATH is not the one the user sees in their terminal.

## Consequences

- A first run ends with a session launcher that has something in it.
- The desktop is now a writer of a file another product owns. Every write is
  atomic and scoped to `workspaces` and `agents`; a rejected edit writes
  nothing.
- Adding a key to the editor means extending `hiveconf.Edit`, its validation,
  and — if it feeds a service — `hiveServices` and the `Rebind` calls in
  `App.ReloadHiveRuntime`; see
  ADR the-hive-runtime-rebinds-on-a-config-write-instead-of-requiring-a-restart.
- The workspace repository count is the cheap shape of hive's scan — a `.git`
  entry, no git process per repository. It can read one high, because hive
  skips a repository it cannot read an origin remote from. It confirms the
  folder is the one the user meant; it does not promise a launch list.
- `HIVE_DEFAULT_AGENT` still wins over `agents.default` at load. The form says
  so when it is set rather than letting the picker appear broken.
- The step is first run only. A returning user with an unusable config is not
  taken over by a full-screen setup: Settings ▸ Hive CLI reports what would
  not parse, and the repair is a hand edit plus a restart.
