---
name: cli-smoke
description: Run hand-driven smoke tests of the hive CLI and TUI in a throwaway Docker stack (a client and two servers), and break the servers on purpose to find failure points. Use when asked to smoke test, try a CLI or remote change end to end, reproduce a remote failure, or look for what breaks under a stuck daemon, a slow network, or a dead server.
---

# Smoke testing in a throwaway stack

`test/smoke/` runs hive on a client and on `server-a` and `server-b`, on a private Docker network with no route out. Run anything you like in it: nothing from the host is mounted except generated keys and the client config. Use it instead of running hive, tmux, or a probe script on the host.

This is for exploring. When you find a bug, add a test for it in `test/remote/` (see "Remote Server Tests" in `AGENTS.md`).

## Commands

```bash
mise run cli:smoke:up                                     # start, or replace the stack with this branch's binary
mise run cli:smoke:exec -- client hive session list       # a command as the hive user
mise run cli:smoke:exec -- server-a hive session list     # the server's own view
mise run cli:smoke:root -- server-a ps -eo pid,stat,args  # a command as root
mise run cli:smoke:down                                   # remove it all
```

`up` wipes the stack, so sessions do not survive it. `test/smoke/run.sh` takes the same arguments as the tasks and leaves out the mise output, which keeps `--json` output clean to parse. Each worktree gets its own stack.

On the client, `/home/hive/repo.git` is a bare repository and `/home/hive/work` is its clone, and both exist on the servers too. Use `--background` when you create a session from a command:

```bash
run=test/smoke/run.sh
$run exec client hive --target-server server-a session create --json --background --remote /home/hive/repo.git demo
$run exec client hive session show --json server-a:<id>
$run exec server-a hive session show --json <id>
```

Check a result from the server's own CLI, not only from the client that asked for it.

The agent in every session is `fake-claude`. To set its status, write a snapshot name to `/tmp/fake-claude/<basename of the session path>` on that machine. The names are the files in `internal/domain/terminal/assess/testdata/claude/`.

## The TUI

The TUI runs in tmux on the client:

```bash
mise run cli:smoke:tui -- start
mise run cli:smoke:tui -- keys j Enter    # tmux send-keys names
mise run cli:smoke:tui -- show            # the screen as text
mise run cli:smoke:tui -- stop
```

Give it a few seconds after `start` and after a key. Its log is `/tmp/hive.log` on the client.

## Faults

```bash
mise run cli:smoke:fault -- <name> <server> [args]
mise run cli:smoke:fault -- clear <server>           # undo every fault on that server
```

| Fault | What happens | Args |
|---|---|---|
| `stuck-daemon` | ssh connects, the daemon never answers | |
| `kill-daemon` | the daemon dies; the next connection starts a new one | |
| `slow-git` | every git command on the server takes longer | seconds, default 12 |
| `shell-noise` | the server's shell prints text before hive starts | |
| `network` | tc netem on the server's interface | netem args, default `delay 300ms loss 5%` |
| `stop` | the server is off, so connections fail | |
| `freeze` | the server hangs, so connections get no answer and no error | |

`network` needs the `sch_netem` kernel module on the Docker host. netem delays ARP replies too, so with a delay over about 3s the first connection fails with `No route to host`; connect once to fill the cache. An ssh connection takes about 12 round trips, so a `delay 2s` connection takes about 25s. For a fault that is not listed, use `root`: a wrapper in `/usr/local/bin` slows or breaks any command the daemon runs (`/usr/local/bin` comes first in its `PATH`), and a `commands:` rule in `~/.config/hive/config.yaml` on the server holds a create at a known point. Restart the daemon after you change that config.

## What to look for

Run each action from the client under each fault, and record:

- **Does it end?** Put `timeout 60` in front of the command. A hang is a bug.
- **Does the error help?** It should name the server and say what the user can do next.
- **Is stdout clean?** With `--json`, stdout is for JSON.
- **What state is left?** Compare the server's session list, `~/.local/share/hive/repos/`, `tmux ls`, and `hive doctor` before and after.
- **Does a retry work?** Clear the fault and run the same command again.

## Traps in a probe

These made probes look like they passed or failed for the wrong reason:

- **Ctrl-C:** a background job in a non-interactive shell ignores SIGINT, so `cmd & kill -INT $!` does not test it. Use `timeout -s INT 3 hive ...`, which exits 124 whatever hive does, and judge by the state left behind.
- **`delete --force`** skips the risk check, so `slow-git` does not slow it.
- **Timing:** a create can end before your fault lands. Make the slow step long, and confirm the fault landed while the command was still running.
- **`mise` output:** use `test/smoke/run.sh` when you parse stdout.
