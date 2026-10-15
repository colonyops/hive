# Remote Hive fixture

The persistent Linux fixture runs Hive Desktop, the CLI, tmux, a test inbox,
and a shell workspace. SSH is its only published port. It imports no host
repositories or agent credentials.

```sh
mise run desktop:remote:up
mise run desktop:remote:ssh-config
```

Add the printed block to `~/.ssh/config`. It references the private key and
Docker-verified host key in this checkout's `.hive-remote/` directory. Open
Desktop **Settings ▸ Remote** and enter:

- SSH host: the `Host` alias printed above.
- Connection file: `/home/hive/connection.json`.
- Name and color: your choice.

Click **Connect**. Go retrieves the token and public certificate and opens
HTTPS/WSS through SSH stdio forwarding. No manual tunnel or pasted token is
needed. The fixture permits forwarding only to its TLS API at `127.0.0.1:19003`.
Local MCP and health checks use `127.0.0.1:19001`; that port is not SSH-forwardable.

Select `remote-demo` to use its two shell windows. In Chats, select **Remote
test shell** and create a chat. These shells use no AI account. For an agent
workspace, configure its command and sign in on the remote host.

```sh
mise run desktop:remote:shell
mise run desktop:remote:down
```

Named volumes retain the remote home, TLS identity, and SSH host keys across
rebuilds. Tmux survives client disconnects, but not container restarts. Startup
restores the demo terminals. `HIVE_REMOTE_SSH_PORT` changes the published SSH
port from `19222`; regenerate the config block after a change.

## Tests

```sh
mise run desktop:remote:test
```

A second Docker container acts as the client. It checks SSH credential retrieval,
HTTPS discovery, terminal input, resize, windows, SSH stream loss/reconnect,
inbox reads, and remote chat creation. It also checks that the browser does not
contact the remote endpoint. Browser and tmux tests stay inside Docker.

## Other hosts

Build the remote backend with `server,terminals`. Set its normal Hive data and
config directories and `HIVE_DESKTOP_CONNECTION_FILE` to a file in a private
existing directory. Optionally set `HIVE_DESKTOP_REMOTE_ADDRESS=127.0.0.1:19003`
for a fixed TLS port. Run one backend per installation. Keep the Wails browser
server on loopback too.

Desktop uses the normal OpenSSH config, agent, and host-key verification.
Trust the host through your normal SSH setup first. Password prompts and new
host-key prompts are not supported inside Desktop. Restricted SSH accounts
must allow `cat` on the connection file and TCP forwarding to the TLS port.

`desktop:remote:connection` retrieves a private local copy for diagnostics.
`desktop:remote:tunnel` provides a diagnostic TLS port forward; it is not needed
by Desktop. Never share or commit connection files.
