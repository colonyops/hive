# Remote Hive fixture

The persistent Linux fixture runs Hive Desktop, the CLI, tmux, a test inbox,
and shell and Codex workspaces. SSH is its only published port. It imports no
host repositories. Agent credentials are imported only by an explicit command.

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

## Test Codex with your ChatGPT subscription

The fixture image includes Codex CLI 0.161.0. Rebuild the fixture, then explicitly
copy your local ChatGPT login into its persistent home:

```sh
mise run desktop:remote:up
mise run desktop:remote:codex-auth
```

The import reads `$CODEX_HOME/auth.json` (default `~/.codex/auth.json`). Set
`HIVE_REMOTE_CODEX_AUTH_FILE` to use another file. Sign in locally with
`codex login` first if the file does not exist; a login stored only in the OS
keyring must first be made available through Codex's file credential storage.
The importer accepts ChatGPT credentials, not API keys, and verifies the login
method with `codex login status` inside the container.

Credentials travel through Docker exec stdin into `/home/hive/.codex/auth.json`,
owned by `hive` with mode `0600`. They never enter the Docker image, build
arguments, command arguments, or this checkout. The container has its own
writable copy so Codex can refresh it; host configuration, history, and secrets
are not mounted. Re-run the import if the copied login stops working, or use
`codex login --device-auth` in `desktop:remote:shell` for a separate login.
See [Codex authentication](https://learn.chatgpt.com/docs/auth#fallback-authenticate-locally-and-copy-your-auth-cache).

Reconnect Desktop after rebuilding. In **Code**, choose a remote repository in
**New session**, then choose **codex** as the agent. An existing session's
**New window** menu also offers Codex. In **Chats**, select **Remote Codex** and
create a chat. Both request workspace-write sandboxing and on-request approvals;
complete Codex's first-use prompts in the terminal. Docker's default seccomp
policy blocks the user/mount namespaces needed by Codex's shell sandbox on the
tested Docker host. Authentication and model responses work, but shell tools
remain blocked until a compatible container security policy is configured.
These tasks do not disable either sandbox or make the container privileged. Shell remains the default
agent so automated smoke tests never spend your subscription allowance.

The credential copy survives `down` and rebuilds, just like the rest of the
remote home. Remove that copy without changing your host login with:

```sh
mise run desktop:remote:codex-logout
```

## Tests

```sh
mise run desktop:remote:test
```

A second Docker container acts as the client. It checks SSH credential retrieval,
HTTPS discovery, terminal input, resize, windows, SSH stream loss/reconnect,
inbox reads, and remote chat creation. It also checks that the browser does not
contact the remote endpoint. Browser and tmux tests stay inside Docker.

```sh
mise run desktop:remote:test-unattended
```

The unattended test connects a local Desktop, disconnects it, then stops both
the local backend process and browser. While they are stopped, the remote's
real scheduler launches a chat and an incoming webhook launches an inbox worker.
A deterministic Python command stands in for the AI agent: it posts result
items and marks the incoming item done. The test checks the scheduler history,
worker receipts, and archived inbox item before it restarts Desktop. It then
reconnects through SSH/TLS and checks that both result items appear in the UI.

Each run uses a unique workspace, flow, and item IDs, and deletes its workspace
and flow afterward. No AI login, host tmux, direct database reads, or forced
scheduler ticks are needed. This covers incoming webhook notifications and
inbox processing; it does not assert OS notification banners or AI reasoning.

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
