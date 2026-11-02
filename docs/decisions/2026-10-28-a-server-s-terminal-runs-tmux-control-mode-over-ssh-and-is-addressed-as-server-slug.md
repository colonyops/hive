# A server's terminal runs tmux control mode over ssh and is addressed as server:slug

- **Status:** proposed
- **Date:** 2026-10-28

## Context

Hive Desktop renders a session's terminal through a tmux control-mode client
that it runs on this machine (ADR terminal-transport). A session on a server
(#738) has its tmux session on that server. The frontend and the HTTP control
plane address a terminal by its slug alone.

## Decision

- **tmux runs over ssh.** `tmuxcc.Runner` builds every tmux command a
  `tmuxcc.Manager` runs. `LocalRunner` is the existing local exec.
  `SSHRunner` runs `ssh <host> -- tmux ...` with the ssh options of the server
  fleet and no pty. Control mode, the one-shot commands, and the stdin of
  `load-buffer` all work over the pipes ssh gives.
- **One manager for each server.** Desktop builds a server's manager on its
  first terminal call, from the server fleet, and stops it when a config
  reload changes or removes the server.
- **ssh shares one connection for each server.** `ControlMaster=auto` with
  `ControlPersist=60` under `/tmp/hive-<uid>/ssh`, so the one-shot commands of
  a sidebar sweep do not each pay for a handshake. The directory is in `/tmp`
  because a unix socket path must fit in 104 bytes.
- **The target is `<server>:<slug>`.** Every terminal call and the stream take
  the same string. tmux allows no `:` in a session name, so a bare slug is
  always local, and the HTTP API and the frontend stay keyed by one string.
- **The frontend attaches again after an ssh drop.** For a server's terminal
  it does not read the window list, which cannot tell a terminated session
  from an unreachable server. It attaches again after 1s, 2s, then every 5s,
  12 times at most. The attach answers `not_found` when the session is gone.

## Consequences

- Desktop does not have the server's rules, so `Start` asks the daemon to open
  the tmux session (`sessionapi.API.StartTmuxSession`) and then attaches.
- Images are pasted as local file paths (ADR
  terminal-image-input-is-delivered-as-local-file-references), which a server
  cannot read. The frontend refuses an image for a server's terminal.
- A pop-up terminal runs on this machine, so it refuses a server's terminal
  as its directory.
- A server's tmux runs from the PATH of a non-interactive ssh command, which
  often lacks a package manager's directory. `tmux` under `servers:` names its
  path, as `binary` does for hive. We did not export a wider PATH in the remote
  command, because that syntax differs between login shells.
- Desktop cannot tell whether a server's pane is at a prompt: the check reads
  this machine's process table. Closing one always asks.
- A sweep omits the windows of a server that does not answer, so one server
  offline does not fail the sweep of the others.
