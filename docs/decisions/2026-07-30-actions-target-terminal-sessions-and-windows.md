# An action declares which surfaces it targets, and a terminal action runs without a durable command

- **Status:** accepted
- **Date:** 2026-07-30

## Context

Terminal mode had no per-session operations of the user's own: "open this
worktree in Zed", "open it in Finder", "run the test task" all meant leaving the
app. The app already has an actions model — `actions.yml`, a directory watcher,
an editable catalog, a `shell` executor, per-type docs — but every part of it
assumes a **feed item**. `applies_to` matches an item kind; `show_in_detail`
means the item detail pane; `OutputData` carries `.Payload`, `.Key` and `.Raw`,
all of which come from the item that triggered the run.

A second config file for terminal actions would duplicate the loader, the
watcher, the editor, the executors and the docs, and would give the same user
two vocabularies for "run a command".

## Decision

1. **An action declares its surfaces in `targets:`.** The vocabulary is
   `item`, `session` and `window`; declaring none means `item`, so every action
   written before terminal mode keeps meaning exactly what it meant. Declaring
   `[item]` is the default spelled out and collapses to it, so the YAML writer
   never adds a redundant key to a file it re-serialises.

   `applies_to` and `show_in_detail` were left alone and **refine the item
   target only**: an item kind is not a thing a session has, and the presence
   of a terminal target in `targets` is already the decision to offer it.
   Folding `show_in_detail` into `targets` was considered and rejected — it is
   a breaking schema change, and the migration runner (ADR yaml-config-migration) rewrites
   without preserving comments, which is a poor trade for a hand-authored,
   dotfiles-managed file when the gain is cosmetic.

2. **The target's data is resolved by the core, not sent by the caller.** A
   client sends `TerminalTarget{Slug, WindowID}` — identity only. Everything a
   template reads (`.Session.Path`, `.Session.Repo`, `.Session.Branch`, …)
   comes from the hive session record at invocation time, so a client cannot
   hand an executor a checkout path of its choosing. `OutputData` grew
   `Session` and `Window` pointers, both nil on the item path, so a template
   that reads the wrong surface's data fails the run rather than rendering a
   blank command.

   The window carries **only its tmux id**, deliberately. The id is what
   addresses a window in a tmux command and it survives a rename; the name does
   not, and adding it would mean a tmux query on every invocation for a value
   no command needs.

3. **A terminal action enqueues no `output_command`.** A durable command's
   `UNIQUE (action_id, key)` is the only thing stopping an already-run action
   from re-firing — and that is exactly wrong for a manual operation against
   live local state. "Open the worktree in Finder" must be repeatable without a
   rerun confirmation, and replaying it after a restart would be a side effect
   nobody asked for. It runs through the same `Dispatcher` and the same
   executors, inside `jobs.Track`, so the jobs UI is where its outcome lands —
   the same shape delete, recycle and prune already have.

   The cost is that there is no durable row holding the run's streams, so a
   failed shell action would otherwise report only its exit status. The tail of
   stderr goes into the job's failure reason instead.

4. **`launch-session` cannot target a terminal surface.** It creates a *new*
   session: the interactive variant needs the New Session form the row menus do
   not have, and the headless variant's `repo_template` renders over a feed
   item's payload, which a terminal target carries none of. `validateActions`
   refuses the combination when the catalog is parsed, so the refusal lands
   while authoring rather than on a click, and the editor disables the boxes.

5. **A `shell` action's `cwd` defaults to the session's checkout on a terminal
   target.** Without it every terminal shell action would have to restate
   `cwd: "{{ .Session.Path }}"`, and `mise run test` would not be a complete
   action. A configured `cwd` still wins.

6. **A clipboard action is copied, never run**, on the terminal surfaces as on
   the detail pane. It produces text rather than a side effect, so it has its
   own render-only path (`RenderTerminalClipboardAction`) and dispatches through
   `ClipboardExecutor` so the copied text cannot drift from what a run of the
   same action would produce.

7. **The surface is the row menu.** Session-targeted actions land in
   `SessionRowMenu`'s existing host-contributed `extra` section, under the
   session's own operations. A window row gained a menu of its own — but only
   when something targets a window, because a window row has no operations of
   its own and an empty menu is an affordance that does nothing.

## Consequences

- `SessionsService` owns the terminal-action surface (`TerminalActionViews`,
  `InvokeTerminalAction`, `RenderTerminalClipboardAction`): the target is a
  session, and resolving one from its slug is already this service's job.
- The `Dispatcher` is now built by `App` and shared with the output worker
  rather than being private to it. A second dispatcher would be a second
  executor map to keep in step.
- `actions.View` is unchanged, so the terminal's menus, the detail pane and the
  item menu all present actions from the same contract, and declared inputs
  (ADR action-declared-inputs) work on a terminal target with nothing further to wire.
- A terminal action's run does not survive an app restart, and a mid-run quit
  loses it. That is the intended trade for repeatability: the durable path
  exists for flow-fired automation, where replay is the point.
- Nothing offers terminal actions over the agent HTTP API yet. Adding it is a
  controller over the same three service methods.
