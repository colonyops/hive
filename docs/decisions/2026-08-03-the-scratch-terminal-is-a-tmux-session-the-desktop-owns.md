# The scratch terminal is a tmux session the desktop owns, pinned above the repositories

- **Status:** proposed; §4's closing claim amended by [ADR tmux-runs-in-the-resolved-environment](2026-08-04-tmux-runs-in-the-resolved-environment.md)
- **Date:** 2026-08-03

## Context

Every terminal in the app belongs to something. A tmux terminal belongs to a
hive session — the slug is the session's, its windows are hive's spawn
configuration, and its working directory is the session's checkout (ADR terminal-start-is-an-offered-action).
A pop-up terminal belongs to a moment: it dies with the app, has no name, no
tabs, and never appears in the tree (ADR ephemeral-popup-terminals). There is nowhere to just work:
no shell that outlives the app, keeps as many tabs as the user wants, and is
tied to no piece of tracked work.

Nothing about the terminal machinery is in the way. `tmuxcc` is keyed by a slug
and does not care where the slug came from; the pool, the window sweep, the tab
handling and the tree all read a session row. Only two things stand in the way,
and both are hive's: a session cannot be created without a repository
(`sessions_service.go` rejects an empty one), and the desktop does not choose a
working directory — `ResolveSpawn(rules, remote)` does, from user config keyed
by the remote a scratch session would not have.

## Decision

1. **The scratch terminal is a tmux session this app creates, with no hive
   session behind it.** Not a remote-less hive session: that would put a row in
   `hive.db` that the CLI and the supervisor list as work, and marking it as
   not-work means editing vendored code (which we do not do). Not a fourth
   terminal concept either — it is an ordinary tmux session, so attach, the
   window sweep, tabs, reorder, kill, the pool and the tree serve it unchanged.
   What the desktop owns is exactly one thing: what creating it means.

2. **`Scratch` is a reserved slug, chosen so hive cannot mint it.** Hive's
   `Slugify` lowercases before it replaces, so no session a user can name
   reaches a capital letter. A session called "Scratch" slugs to `scratch` and
   passes this one by. The two products share tmux's one session namespace and
   the slug is the only address either uses (ADR session-rename-keeps-slug-and-tmux-in-step), so a slug hive can
   produce would eventually be produced.

3. **Starting it opens one window in the user's home directory — as the
   *session's* working directory, not the window's.** tmux opens a window with
   no directory of its own in the session's, so every tab added later starts
   there too and nothing has to remember where the session began. Spawn rules
   are user config keyed by a remote; a scratch session has none, and inventing
   a default rule would mean a config error the user did not cause the first
   time they clicked.

4. **The session runs an interactive login shell, which is what makes its PATH
   the user's.** A desktop launch inherits the launcher's environment (ADR subprocess-environment),
   and a create is what may start the tmux server every later shell inherits from
   — so an app-resolved environment looked necessary. It is not: `tmuxcc`'s
   create execs `$SHELL -l` (the same empty-command create an agent workspace
   session uses), and tmux's own default for a window with no command is a login
   shell too, so every tab reads the user's startup files and resolves its own
   PATH. ~~This package holds no environment policy, and should not grow one.~~
   *ADR tmux-runs-in-the-resolved-environment grew it one: an agent's `-c` launch line runs in a login shell that
   is **not** interactive and reads no `.zshrc`, so `tmuxcc` now spawns tmux in
   the resolved environment. Nothing above changes — the scratch terminal's
   shell is interactive, its startup files still decide its PATH, and the
   resolved environment is only the floor under them.*

5. **It is a pinned section of the session tree, headed like a repository and
   listing its tabs where a repository lists its sessions.** The tree is
   group → session → window everywhere, so the scratch terminal is a group of
   one, prepended rather than sorted in — being a row of the same shape is what
   makes the pool, the sweep, the keyboard walk and the deletion watcher carry it
   without learning a second kind of row. What it does **not** draw is that row:
   a Scratch row under a Terminals heading names the same thing twice and buys a
   level of nesting for a group that will only ever hold one session, so the
   heading is the section and the tabs sit one level in, in the box a session row
   would have. `Scratch` survives only as the tmux name `tmux ls` shows. The
   heading is a row rather than a button, because it carries the section's own
   controls and a button cannot contain one.

6. **`+` on that heading is the whole affordance, and creating the session is
   what it means while tmux holds nothing.** The session it creates *is* the
   first tab, so there is no separate start to have missed. What decides between
   the two is the server's answer — `started` from the same probe-then-create
   call — rather than what the tree has swept, and this is scratch-only: `+` on a
   hive session must never start one, because starting runs its agent (ADR terminal-start-is-an-offered-action).
   An empty section still draws one row, offering the same start, so the keyboard
   has somewhere to land and the mouse is not left guessing at the `+`.

7. **Its liveness comes from the window sweep, and its row offers only the
   terminal's own lifecycle.** Hive's status projection is keyed by session id
   and has nothing to say about it, so the sweep — one tmux call for every slug
   in it — answers whether tmux is holding it, and the scratch slug rides that
   call whether or not "always show windows" is on. Rename, recycle, delete and
   session details address a hive record; configured actions render over one
   (ADR actions-target-terminal-sessions-and-windows). None are offered, rather than offered and failed.

## Consequences

- Prune is unaffected: it acts on hive's recycled and corrupted sessions, and
  the scratch terminal is in no listing. The tree's prunable count is read off
  the listing rather than as the remainder of the tree, which is what keeps the
  pinned row from being counted as one.
- The pool may still evict it. That costs a re-attach and a repaint, not the
  terminal: the tmux session outlives every attach, which is the whole reason
  this is tmux's rather than `ptyterm`'s. Exempting it would be a special case
  buying nothing.
- Killing it is offered and confirmed, and it comes back empty. There is no
  "reset" beyond that, and no persistence of what was in its tabs — tmux holds
  the session, and when tmux loses it (a reboot, `kill-server`), the row offers
  a start like any other.
- There is exactly one. Tabs are the multiplicity, which is what the tree
  already renders and what tmux already manages; a second scratch session would
  need a name, and naming it is the work a hive session is for.
- Adding a window no longer requires an attach anywhere: a slug with no control
  client is served by a one-shot, and its `-c` is spelled `#{session_path}`
  because tmux resolves an unset start-directory against the client running the
  command — which for a one-shot is this process, so the window would otherwise
  open in the app's working directory. This is what the pinned row's first press
  needs, and hive sessions get it for free.
- The reserved slug is a contract across two products. A test pins that hive's
  own slugify cannot produce it; a change to `Slugify` upstream that admits
  capitals breaks that test rather than the app.
