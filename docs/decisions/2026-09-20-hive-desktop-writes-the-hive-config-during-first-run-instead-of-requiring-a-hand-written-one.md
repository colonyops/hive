# Hive Desktop writes the Hive config during first run instead of requiring a hand-written one

- **Status:** accepted
- **Date:** 2026-09-20

## Context

The session launcher lists repositories by scanning the Hive config's
`workspaces` and offers the profiles in `agents`. Neither has a useful
default: with no file, hive invents a single `claude` profile and no
workspaces. A user who never ran the `hive` CLI got an empty launcher and no
way to fix it short of hand-writing YAML.

The file belongs to the `hive` CLI too. It is often dotfiles-managed, may
carry rules, keybindings, and user commands the desktop has no opinion on,
and `hive init` writes it as well.

## Decision

First run asks for two things and writes them: which agents can start a
session, and which parent folders hold the repositories. First run is the
only writer. Settings ▸ Hive CLI points at the file and the hive docs, and
says a hand edit needs a restart.

First run branches on whether the config is **usable** (it declares at least
one agent profile and one workspace), read from the file's own YAML by
`hiveconf`, never from hive's merged config, so hive's `claude` fallback is
not mistaken for a user choice. A usable config is confirmed and adopted
unchanged.

The desktop owns `workspaces` and `agents` and no other key.
`config.ApplyEdit` (`internal/config/write.go`) creates a minimal file or
edits the existing file's node tree in place, so comments, key order, unknown
keys, `agent_selector`, and other profiles survive. It validates the edit
(including `rules[].agent` against the profile set), runs hive's own loader
over the candidate, and replaces the file atomically. A rejected edit writes
nothing.

The step is skippable, and an agent not found on PATH is still selectable,
because a Dock launch's PATH is not the user's shell PATH.

## Consequences

- A bad write would stop hive from loading at all, not degrade it, which is
  why every write is checked by hive's loader first.
- Adding a key the desktop writes means extending `config.Edit` and its
  validation. A write takes effect through `App.ReloadHiveRuntime`
  (ADR the-hive-runtime-rebinds-on-a-config-write-instead-of-requiring-a-restart).
- The workspace repository count only checks for a `.git` entry, so it can
  read one high. It confirms the folder, not the launch list.
- `HIVE_DEFAULT_AGENT` still wins over `agents.default` at load.
- A returning user with an unusable config is not sent back through setup.
  Settings ▸ Hive CLI reports what would not parse.
