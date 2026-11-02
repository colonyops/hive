---
icon: lucide/server
description: Run sessions on other machines over ssh and manage them from the hive CLI and TUI.
---

# Remote servers

A server is another machine that runs hive. The CLI and the TUI on your machine reach it over ssh and manage its sessions next to your local ones. The sessions, their clones, and their tmux sessions stay on the server. Remote servers are experimental.

## Set up a server

On the server:

1. Install hive, tmux, and your agent. In the server's `config.yaml`, list your repositories under `workspaces:` and set any `agents:` and `rules:` you want. A session on a server uses the server's own config: its agent profiles, and the `windows` and setup commands of its rules.
2. Optional: run `hive daemon install` so the hive daemon starts again after a restart. Without it, the first connection starts the daemon.

On your machine:

1. Make sure `ssh <host>` logs in without a prompt. hive runs ssh with `BatchMode=yes`, so it cannot ask for a password or a new host key. Use a key, and connect once by hand to accept the host key.
2. Add the server to `config.yaml`:

```yaml
servers:
  - name: box            # how hive names the server; not "local", no ":"
    ssh: dev@box.lan     # an ssh destination or a Host alias
    ssh_config: /home/me/.ssh/box     # optional ssh config file; use a full path, because ssh does not expand ~ here
    binary: ~/.local/bin/hive         # optional, when hive is not on the PATH of a plain ssh command
    tmux: ~/.local/bin/tmux           # optional, the same for tmux
```

3. Run `hive servers ls`. Each server shows its state, hive version, and session count. `hive servers probe <name>` times each step and reports a missing agent or a daemon that `hive daemon install` did not start.

## Work with server sessions

| Command | What it does |
| --- | --- |
| `hive session list` | Lists local sessions and the sessions of every server, with a SERVER column |
| `hive new --server box --remote <url> <name>` | Creates a session on `box` with the server's rules, then attaches. With `--background` or no terminal, it prints the attach command instead |
| `hive session attach box:<id>` | Opens the session's terminal, and starts it first when its tmux session ended. When all of the session's commands finished, there is no terminal to open, and hive says so |
| `hive session show\|update\|delete\|recycle box:<id>` | Acts on the session on `box` |

Every session command except `hive session info` takes `--server <name>` (or `HIVE_SERVER`), and then a bare id names a session on that server. `--server local` means this machine.

- The server runs the clone, so `--remote` must be a URL the server can reach. Without `--remote`, hive uses your checkout's origin, and refuses an origin that is a path on your machine.
- `--source` is refused, so the `copy:` files in the server's rules are not copied.
- An attach runs `ssh -t <host> -- tmux attach-session`. Inside tmux, it opens a new window so you keep the current one.

In the TUI, each server is a group after your local repositories, even with no sessions. When the tree groups by session group instead of repository, a server's sessions sit in those groups, without the server's state. Rename, tags, group, recycle, delete, and open work on its sessions, and the tree shows their branch, changes, and preview from the server. Press the new-session key (<kbd>n</kbd> by default) on a server's group or session to create a session there, from the repositories and agents the server lists. It starts in the background, so open it from the tree. A command with `sh:` or `windows:` runs on your machine, so it is refused for a server's session.

## Server states

| State | Meaning | Fix |
| --- | --- | --- |
| connecting | The first check has not finished | |
| connected | The server answered | |
| offline | ssh or the daemon did not answer | Read the ssh error that hive shows. Check that `ssh <host>` works without a prompt and that hive is on the server's PATH, or set `binary:`. |
| incompatible | The server's hive speaks another protocol version | Install the same hive version on both machines |

An offline server does not stop the rest: `hive session list` prints a warning and lists the other sessions, and the TUI shows the server's group with its error and keeps the rows it already listed. A server that hangs can delay a list by up to 5 seconds. `hive session list --server <name>` fails when that server is offline.

Hive Desktop reads the same `servers:` list. See [Servers in Code](../../desktop/code/servers.md).
