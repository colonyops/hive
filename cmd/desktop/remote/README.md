# Remote Hive fixture

This fixture runs a real Hive Desktop backend and CLI in a persistent Linux
container. SSH is its only published port. It seeds `remote-demo` with two shell
windows; it never imports host repositories or agent credentials.

From the repository root:

```sh
mise run desktop:remote:up
mise run desktop:remote:tunnel     # keep running in a second terminal
mise run desktop:remote:connection
mise run desktop:dev:prepare
mise run desktop:dev              # native local Desktop, separate terminal
```

In Desktop, open **Code → Remote (preview)**. Use
`http://127.0.0.1:19001` and the `token` from
`.hive-remote/connection.json`. The connection file is private; do not
commit or share it. After a backend restart, retrieve it again. The private SSH
key stays in this worktree's `.hive-remote/` directory. Host keys are
retrieved through Docker and pinned in that directory's `known_hosts`.

Select `remote-demo`, switch between its windows, type in its shell and resize
the app. Stop the tunnel to exercise connection loss, restart it, and click
**Reconnect**. Closing the remote view detaches; it does not kill the shells.
Local Code, inbox and Chats continue using the local installation.

```sh
mise run desktop:remote:shell     # an interactive SSH session as hive
mise run desktop:remote:down      # retains volumes; up starts them again
```

The fixture's home and SSH host keys live in named Docker volumes scoped to
this checkout. Rebuilding retains them. Tmux processes survive client/tunnel
loss, but not a container restart; startup restores the demo session’s terminals. The fixture intentionally uses Bash, not a paid agent.
Install/authenticate a CLI inside the container if testing actual agent usage.

`HIVE_REMOTE_SSH_PORT` changes the host SSH port (default `19222`).
`HIVE_REMOTE_LOCAL_PORT` changes the tunnel's local port (default `19001`).
Set these consistently across tasks when running several fixtures. None of the
HTTP ports is published. Inside the container, the terminal API listens on
`127.0.0.1:19001` and the Wails browser UI on `127.0.0.1:8080`.

## Automated smoke test

```sh
mise run desktop:remote:test
```

This builds the fixture and runs a second container as the client. The test
checks token rejection, session discovery, terminal input, window discovery,
tunnel loss/reconnect, and session survival after detaching. Browser tooling
and tmux testing stay inside Docker. The remote volume remains reusable after
the test.

## Other SSH hosts

Build the backend with `go build -tags server,terminals -o hive-desktop ./cmd/desktop`
after building the frontend. Configure its normal Hive data/config directories,
set `HIVE_DESKTOP_HTTP_PORT` to a stable loopback port and
`HIVE_DESKTOP_CONNECTION_FILE` to a file in an existing private directory. Start
only one Desktop backend against that installation. Keep the Wails server bound
to loopback too.

Forward that API port with OpenSSH, for example:

```sh
ssh -N -T -o ExitOnForwardFailure=yes -L 127.0.0.1:19001:127.0.0.1:19001 my-host
```

Retrieve the connection file through SSH and enter its token in Desktop. Your
normal SSH host-key verification, keys and Tailscale routing apply. Keep the
client and backend on compatible builds. SSH lifecycle management and remote
installation/upgrades are manual in this first slice.
