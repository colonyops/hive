# tmux is spawned in the resolved environment, because a login shell is not an interactive one

- **Status:** proposed; amends [ADR subprocess-environment](2026-07-30-subprocess-environment.md)'s last
  consequence and [ADR the-scratch-terminal-is-a-tmux-session-the-desktop-owns](2026-08-03-the-scratch-terminal-is-a-tmux-session-the-desktop-owns.md) §4
- **Date:** 2026-08-04

## Context

ADR subprocess-environment resolved one PATH for every command the app spawns on the user's
behalf, and its consequences named `tmuxcc`'s control client as the deliberate
exception: it did not need the PATH, because ADR tmux-discovery hands it an absolute
binary and tmux's own panes start login shells.

The second half of that is false for the case ADR agent-workspace-sessions-are-tmux-sessions introduced. An agent
workspace session is created with `new-session … -- $SHELL -l -c <launch line>`,
and a login shell running `-c` is not an interactive one: zsh reads `.zshenv`,
`.zprofile` and `.zlogin`, and never reads `.zshrc` — which is where a user's
PATH additions overwhelmingly live. `execenv` already knew this; its probe is
`$SHELL -ilc` precisely because "the PATH a terminal shows is as often set in an
interactive startup file as in a login one".

So when the app is launched from the Dock — launchd's
`/usr/bin:/bin:/usr/sbin:/sbin` — or relaunched by a Homebrew operation from its
sanitized shim PATH, the tmux client inherited that PATH, the pane inherited the
client, and the login shell recovered nothing. The agent binary was not found,
the shell exited, the tmux session was gone inside 100ms, and `awaitEarlyExit`
turned that into HTTP 200 with a notice and an empty terminal id. Clicking a
session did nothing at all, and nothing was written to the log.

`ptyterm` had taken the resolved environment through an `Environ` hook since
ADR subprocess-environment. `tmuxcc` had a bare `os.Environ()`. That asymmetry was the whole bug.

## Decision

1. **`tmuxcc.Manager` takes an `Environ` hook, and it is `ptyterm`'s hook
   written a second time rather than one extracted from it.** Same signature,
   same nil-means-this-process's-own default, same wiring to `execenv.Resolver`
   at the composition root. ADR ephemeral-popup-terminals keeps the two backends siblings with no
   shared interface: a `TerminalBackend` holding one method they happen to agree
   on would be a seam designed around a coincidence.

2. **It reaches every tmux the manager execs — the control client and the
   one-shots alike — because a pane inherits the environment of the *client*
   that created it.** Setting `Cmd.Env` on the `new-session` command client is
   sufficient and was verified against an already-running tmux server; there is
   no `new-session -e` plumbing and none is wanted. This is also why the
   one-shot path matters more than the attach path: `NewSession` is the command
   whose environment an agent actually runs in.

3. **The resolved environment is the floor, not a replacement.** `-l` still
   runs, so a login startup file may still set a PATH that wins. What changes is
   what a shell that says nothing leaves behind: the app's own resolved PATH
   rather than the launcher's.

4. **`tmuxcc.Commander` takes the same hook**, so hive's pane-capture status
   detection runs with the environment control-mode attaches do rather than
   quietly keeping the inherited one.

5. **A session that dies before it can be attached to says so on screen.** The
   launch response is the only place its notice exists — the session listing
   reports none, because it never launched anything — so the Agents pane renders
   that notice instead of returning to idle. A 200 that renders as nothing
   happening is indistinguishable from a dead click, which is how this shipped.

## Consequences

- A tmux session the app creates now starts agents the way the user's own
  terminal would, from the Dock and from a Homebrew relaunch. Version-manager
  shims come with it, for the reason ADR subprocess-environment gives.
- ADR subprocess-environment's exception list loses its only entry: everything the app spawns on
  the user's behalf now goes through `execenv`.
- ADR the-scratch-terminal-is-a-tmux-session-the-desktop-owns §4's "this package holds no environment policy, and should not grow
  one" no longer holds. Its conclusion about the *scratch* terminal does: that
  session's shell is interactive, so the user's startup files still decide its
  PATH, and the resolved environment is only the floor under them.
- The first tmux operation of a run may pay the login-shell probe, which is
  bounded at five seconds and shared with hooks and shell actions — the resolver
  probes once per run and remembers the answer either way. A probe that fails
  degrades to what the app could reach before.
- ~~Only PATH is adopted from the probe, since that is `execenv.Environ`'s
  contract; nothing here widens the set of variables a subprocess inherits.~~
  `Environ`'s contract widened in
  [ADR a-subprocess-inherits-the-whole-shell-environment-not-just-its-path](2026-08-07-a-subprocess-inherits-the-whole-shell-environment-not-just-its-path.md),
  and tmux took the wider set with every other consumer.
- A tmux session created before this app touched the server keeps whatever
  environment it was created with. The environment reaches a session through the
  client that creates it and there is no way to retrofit a live one, so a
  session that predates a fixed PATH is fixed by killing and starting it.
