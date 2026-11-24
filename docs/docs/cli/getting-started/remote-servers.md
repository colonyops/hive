---
icon: lucide/server
description: Run sessions on other machines over ssh and manage them from the hive CLI.
---

# Remote servers

!!! warning "Experimental"
    Remote servers are experimental. The `servers:` config and how server sessions work may change.

A server is another machine that runs hive. The CLI on your machine reaches it over ssh and manages its sessions next to your local ones. The sessions, their clones, and their tmux sessions stay on the server.

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
| `hive session tags update box:<id> --add <tag> --remove <tag>` | Adds and then removes tags on the session on `box` |
| `hive session start box:<id>` | Starts the agent of a session on `box` that has no running terminal, without attaching |

Every session command except `hive session info` takes `--server <name>` (or `HIVE_SERVER`), and then a bare id names a session on that server. `--server local` means this machine.

- The server runs the clone, so `--remote` must be a URL the server can reach. Without `--remote`, hive uses your checkout's origin, and refuses an origin that is a path on your machine.
- `--source` is refused, so the `copy:` files in the server's rules are not copied.
- An attach runs `ssh -t <host> -- tmux attach-session`. Inside tmux, it opens a new window so you keep the current one.

## Tasks, messages, todos and documents on a server

The `hive hc`, `hive msg`, `hive todo` and `hive drive` commands that run catalog operations also take `--server <name>` (or `HIVE_SERVER`). They then run on the server's own hive.db and Drive:

```sh
hive hc next --server box <epic-id>
hive drive list --server box reports
```

- A `--caller box:<session id>` picks the server too, and acts as that session there, as `hive hc next --assign` needs.
- A caller that is a path on your machine, such as the working directory, acts as `global` on the server.
- `hive msg sub`, `hive msg inbox` and the hand-written `hive drive` commands stay on this machine.
- A server whose hive does not serve these commands answers `unavailable` with both hive versions. Install the same hive version on both machines.

The TUI lists only the sessions on its own machine. To use it with a server's sessions, open a second terminal, run `ssh <host>`, and run `hive` there.

## Server states

| State | Meaning | Fix |
| --- | --- | --- |
| connecting | The first check has not finished | |
| connected | The server answered | |
| offline | ssh or the daemon did not answer | Read the ssh error that hive shows. Check that `ssh <host>` works without a prompt and that hive is on the server's PATH, or set `binary:`. |
| incompatible | The server's hive speaks another protocol version | Install the same hive version on both machines |

An offline server does not stop the rest: `hive session list` prints a warning and lists the other sessions. When stdout is not a terminal, `hive session list` and `hive ls` print one JSON line per session instead of the table, as `--json` does. A server that hangs can delay a list by up to 10 seconds. `hive session list --server <name>` fails when that server is offline.

Hive Desktop reads the same `servers:` list. See [Servers in Code](../../desktop/code/servers.md).
