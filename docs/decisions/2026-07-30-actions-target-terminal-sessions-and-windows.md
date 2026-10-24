# An action declares which surfaces it targets, and a terminal action runs without a durable command

- **Status:** accepted
- **Date:** 2026-07-30

## Context

Terminal mode had no user-defined per-session operations ("open this checkout
in Zed", "run the test task"). The actions model (`actions.yml`, watcher,
editable catalog, executors, docs) assumed a feed item: `applies_to` matches an
item kind, `show_in_detail` means the item detail pane, and `OutputData`
carries the item's `.Payload`, `.Key` and `.Raw`. A second config file for
terminal actions would duplicate all of it and give users two vocabularies for
"run a command".

## Decision

1. **An action declares its surfaces in `targets:`**: `item`, `session`,
   `window`. Declaring none means `item`, and `[item]` collapses to the
   default so the YAML writer never adds a redundant key. `applies_to` and
   `show_in_detail` refine the item target only. Folding `show_in_detail`
   into `targets` would be a breaking schema change whose migration drops
   comments from a hand-authored file (ADR yaml-config-migration), for a
   cosmetic gain.

2. **The core resolves the target's data; the caller sends identity only.**
   A client sends `dispatch.TerminalTarget{Slug, WindowID}`. Everything a
   template reads (`.Session.Path`, `.Session.Repo`, `.Session.Branch`, ...)
   comes from the hive session record at invocation, so a client cannot hand
   an executor a checkout path of its choosing. `OutputData.Session` and
   `OutputData.Window` are nil on the item path, so a template that reads the
   wrong surface fails instead of rendering blank. The window carries only its
   tmux id, which addresses it in tmux commands and survives a rename.

3. **A terminal action enqueues no `output_command`.** The durable row's
   `UNIQUE (action_id, key)` stops a run action from firing again, which is
   wrong for a repeatable manual operation, and replaying it after a restart
   would be an unrequested side effect. It runs through the shared
   `Dispatcher` and executors inside `jobs.Track`, so its outcome lands in the
   jobs UI. The tail of stderr goes into the job's failure reason, since no
   durable row holds the streams.

4. **`launch-session` cannot target a terminal surface.** It creates a new
   session from item data or the New Session form, neither of which a
   terminal target has. `validateActions` refuses the combination at parse
   time.

5. **A `shell` action's `cwd` defaults to the session's checkout on a
   terminal target.** A configured `cwd` still wins.

6. **A clipboard action is copied, never run**, through
   `RenderTerminalClipboardAction` (ADR clipboard-action-type).

7. **The surface is the row menu.** Session actions go in `SessionRowMenu`'s
   host-contributed section. A window row gets a menu only when some action
   targets a window. The Code status bar became a third terminal surface
   later (ADR the-code-status-bar-offers-configured-actions-as-a-statusbar-target).

## Consequences

- `SessionsService` owns the terminal-action surface (`TerminalActionViews`,
  `InvokeTerminalAction`, `RenderTerminalClipboardAction`), because resolving
  a session from its slug is already its job.
- `App` builds the `Dispatcher` and shares it with the output worker, so
  there is one executor map.
- `actions.View` is the same contract for terminal menus, the detail pane and
  the item menu, and declared inputs (ADR action-declared-inputs) work on a
  terminal target with no extra wiring.
- A terminal action's run does not survive an app restart. That is the trade
  for repeatability; the durable path exists for flow-fired automation.
