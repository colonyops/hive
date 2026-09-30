# A pop-up terminal launcher is its own list in actions.yml, not an action

- **Status:** proposed
- **Date:** 2026-07-31

## Context

ADR ephemeral-popup-terminals built the pop-up terminal around a launch spec — a directory and a
shell command line — and left the command empty, because nothing in the UI set
it. It named what would: "launchers — a named `lazygit` button, a per-repo
command — are that same spec with a config surface in front of it."

ADR actions-target-terminal-sessions-and-windows had just given actions a `targets:` list (`item`, `session`, `window`)
rather than a second config file for terminal actions, so the obvious place to
put a launcher was the action catalog: a fifth `type`, or a fifth target.

## Decision

1. **A launcher is not an action, and does not live in the `actions:` list.**
   `targets` names the surfaces an action is *offered on*, and every one of them
   ends in the same place: the `Dispatcher`, an executor, a job with an exit
   status and captured streams. A pop-up is none of that — it is a PTY handed to
   the user. A fifth target would make one `shell` config mean two different
   executions depending on where it was clicked, and leave `timeout` and `env`
   silently inert in one of them.

   A fifth *type* was built first and rejected on the evidence: it needed seven
   separate carve-outs in production code — an early return in `HasTarget`, a
   case in `HeadlessCapable`, an `IsLauncher` predicate, a `Dispatched` helper
   whose only job was to exempt it from the executor-map bijection, four
   refusals in `validateActions`, an early return in `effectiveTargets`, and two
   `v-if` guards hiding half the editor form. Of the action envelope —
   `targets`, `applies_to`, `show_in_detail`, `inputs` — a launcher used none.
   Seven exemptions is the container being wrong, not the feature.

2. **It is the `launchers:` list in the same file.** ADR actions-target-terminal-sessions-and-windows's rule was about
   the *file*, not the type: a second config file would duplicate the loader,
   the watcher, the last-good reload, the migration runner, the editor and the
   docs path. A sibling top-level key keeps every one of those — one
   `ActionStore` owns actions.yml and both lists in it — while dropping the
   pretence that a launcher is an action. A `Launcher` carries only `id`,
   `label`, `command`, `cwd` and `icon`.

   Launcher ids and action ids are **separate namespaces**. Nothing resolves a
   launcher against the action catalog, so cross-checking would couple the lists
   for no benefit.

3. **`command` and `cwd` are not templates**, which is why neither carries the
   `_template` suffix the rendered action fields do. There is no triggering item
   to render over, and none is wanted: the command runs through a login shell in
   the working directory, so PATH, aliases, functions and `$PWD` already resolve
   it. The shell is the template engine here. A leading `~` in `cwd` is expanded,
   since `chdir` takes a path rather than a shell word.

4. **Each launcher is a bindable command, `launcher.<id>`, unbound by default.**
   The frontend command catalog was a static list; it now merges the configured
   launchers behind it, so one config entry yields a palette row, a settings row
   and a chord — `launcher.lazygit: [alt+g]` in `settings.yaml`. Launchers sort
   last, so a combo shared with a built-in resolves to the built-in: a config
   file must not be able to take `mod+k` from the palette.

   Because a launcher id is unknown until `actions.yml` has been read — which is
   after `settings.yaml` has — the override loader no longer drops bindings for
   ids it does not recognise. Dropping them would have erased a user's launcher
   bindings from `settings.yaml` on their next unrelated rebind.

5. **A launcher is opened by id, and the core resolves what it runs.** The
   frontend sends `launcher: "lazygit"` to `/api/terminal/popup/open`; the
   command and any pinned directory come from the catalog at that moment. This
   grants no authority the surface did not already have — that endpoint takes an
   arbitrary command line by design — but it keeps one answer to what a launcher
   runs, so a frontend holding a stale catalog cannot run a command the user has
   since changed. It also means an agent on the loopback API can open a launcher
   by name.

6. **Invoking a launcher replaces what the panel is holding.** One pop-up is
   open at a time (ADR ephemeral-popup-terminals), so a launch that differs from the live one — a
   different launcher, or a shell for a different session — opens in its place
   and closes it. Invoking what is already on screen hides it, so the chord that
   opens a launcher can also put it away; the pop-up's own toggle is unchanged
   for anyone who never configures one. A launcher's chord fires while a
   terminal has focus, which ADR ephemeral-popup-terminals allows only for the pop-up's shortcut — a
   launcher's is one, for the same reason.

## Consequences

- **actions.yml holds two kinds of thing, and the split is the file's schema
  rather than a set of exemptions.** Adding an action type is unchanged by any
  of this; adding a field to a launcher touches `Launcher` and nothing else.
- **The action envelope stays honest.** `HasTarget`, `HeadlessCapable`,
  `validateActions` and the executor-map bijection test are back to describing
  actions only, with no "except launchers" branch in any of them.
- **Settings ▸ Launchers is its own section**, because the action editor's form
  — targets, applies-to, inputs, per-type templates — is not a launcher's form.
  `ListActions` still answers for both lists, so there is one read, one wake on
  `actions:updated`, and one parse error to report.
- **A launcher cannot be a flow terminal**, and needs no rule to say so: a flow
  `action` node resolves ids against the action catalog, which a launcher is not
  in.
- **Deleting a launcher needs no usage preflight**, unlike deleting an action.
  Nothing references one but a keybinding, and a binding for an id that is gone
  simply stops resolving.
- **Replacing a live pop-up can end a shell someone was using.** That is the
  cost of one panel and predictable invocation, and it is the trade ADR ephemeral-popup-terminals
  already makes on every other path — nothing in a pop-up survives.
- **The icon vocabulary is a third curated set** (`icons.ValidLauncher`,
  mirrored in `launcherIcons.ts`), separate from the feed set because these name
  programs and tasks rather than kinds of feed item. It is kept in sync by hand,
  the same as the feed set it sits beside.
