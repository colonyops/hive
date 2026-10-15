---
icon: lucide/server
description: Install Hive on an Ubuntu server without a GUI and connect through SSH.
---

# Ubuntu server

Run inbox flows, scheduled chats, and coding sessions on your server while
Hive Desktop connects from your laptop. The server needs no X11, Wayland,
WebKit, or desktop keychain. This setup is experimental and targets Ubuntu
24.04 on amd64 or arm64.

## Build the installation bundle

Server bundles are currently built from source; there is no published server
download or automatic server update channel yet. On a development machine,
follow [Build from source](build-from-source.md) to install the build tools,
then run from the repository root:

```sh
HIVE_SERVER_ARCH=amd64 mise run desktop:server:package
```

Use `arm64` for an ARM server. This creates
`cmd/desktop/bin/server-linux-amd64.tar.gz` and its `.sha256` file. The bundle
contains the server, matching Hive CLI, installer, and service units. Go and
Node are needed on the build machine only.

Copy both files to the server with `scp`. Check the checksum before extraction:

```sh
sha256sum -c server-linux-amd64.tar.gz.sha256
mkdir -p hive-server-install
tar -xzf server-linux-amd64.tar.gz -C hive-server-install
```

The checksum detects transfer damage; obtain the bundle and checksum from a
trusted build. `REVISION` identifies the source commit.

## Prepare the server account

Use a regular Linux account with SSH access. It will own the repositories,
agent subscriptions, configuration, and running jobs. Install runtime packages:

```sh
sudo apt update
sudo apt install git tmux openssh-server ca-certificates curl dbus-user-session
```

Log in over SSH as that account. Install your agent CLI and complete its login
using that CLI's headless login instructions. Test it from this account before
creating schedules. Set Git identity and repository SSH credentials as needed.
Desktop does not forward your laptop's AI login or SSH agent to the server.

## Install and start

Run the installer without sudo:

```sh
./hive-server-install/install.sh
sudo loginctl enable-linger "$(id -un)"
systemctl --user status hive-server.service
```

The installer creates two user services. `hive-server.service` runs Hive;
`hive-server-tmux.service` owns the terminals. Enabling
[lingering](https://www.freedesktop.org/software/systemd/man/latest/loginctl.html#enable-linger%20%5BUSER%E2%80%A6%5D)
starts the user manager at boot and keeps it running after logout.

Use `install.sh --no-start` to install files before the user service manager
is available. Once logged in normally, run:

```sh
systemctl --user daemon-reload
systemctl --user enable --now hive-server.service
```

| Location | Contents |
| --- | --- |
| `~/.local/share/hive-server/bin/` | Server and matching CLI |
| `~/.local/share/hive-server/data/` | Inbox, chat records, TLS identity, and logs |
| `~/.local/share/hive-server/engine/` | Shared engine state |
| `~/.config/hive-server/config.yaml` | Engine settings |
| `~/.config/hive-server/desktop/` | Flows, workspaces, actions, and desktop settings |
| `~/.config/hive-server/environment` | Private service credentials and overrides |
| `~/.local/share/hive-server/connection.json` | Private SSH connection credential |

The server roots are separate from a regular Desktop installation. Do not point
two running backends at the same data directory. To use the bundled CLI with
the server's engine and tmux socket, run:

```sh
~/.local/share/hive-server/launch.sh cli session list
```

## Configure credentials and a workspace

Edit `~/.config/hive-server/environment` for service environment overrides.
It uses systemd `KEY=value` syntax: no `export`, shell commands, or `$HOME`
expansion. Keep mode `0600`. For one GitHub identity, set `HIVE_GITHUB_TOKEN`
there. Other connectors use their provider-specific overrides, such as
`HIVE_GRAFANA_TOKEN`. These override the provider's stored credentials and do
not support distinct tokens per account. Agent CLI login remains separate.

The service includes `~/.local/bin` on PATH. For an agent installed elsewhere,
use an absolute command in its workspace or an absolute PATH in the environment
file. Restart both services after changing environment variables needed by
agents; this ends running terminal processes:

```sh
systemctl --user restart hive-server-tmux.service
systemctl --user start hive-server.service
```

For an initial test that needs no AI login, create
`~/.config/hive-server/desktop/workspaces/shell/agent-workspace.yaml`:

```yaml
version: 5
name: Server shell
command: bash --noprofile --norc -i
```

Create its parent directory first. Then configure your real
[agent workspaces](../chats/agent-workspaces.md) and
[inbox flows](../inbox/flows.md) in the same configuration root.
Scheduled and inbox-triggered chats require a command that accepts the quoted
prompt, as described in the workspace documentation. Work runs on the server
with the service account's permissions and subscriptions.

## Connect Desktop

Verify SSH works noninteractively and trust the host key through your normal
SSH setup. In **Settings ▸ Remote**, enter the SSH host or config alias and:

```text
~/.local/share/hive-server/connection.json
```

Choose a name and color, then connect. Desktop fetches the token and public
certificate over SSH and uses HTTPS/WSS through SSH forwarding. Keep all Hive
ports on loopback. Default ports are `19003` for the remote TLS API, `19001`
for local MCP/webhooks, and `19000` for the local Wails browser server.
The latter two are local HTTP interfaces; do not publish them.

SSH must allow commands and forwarding to `127.0.0.1:19003`. Tailscale can
provide the route to SSH. No public Hive API port is required. See
[Remote connections](../code/remote.md) for client requirements.

Incoming notifications and scheduled work continue while Desktop is closed.
Native OS notification banners are unavailable on the server. Route alerts
through an external notification command or service if you need a phone or
desktop alert while disconnected. External webhook senders also need a route
to the loopback webhook listener, such as a separately configured relay; SSH
Desktop access alone does not expose an inbound public webhook endpoint.

## Operate and update

```sh
journalctl --user -u hive-server.service -u hive-server-tmux.service -f
curl --fail http://127.0.0.1:19001/api/status
systemctl --user restart hive-server.service
```

A Hive restart keeps the separate tmux daemon and live chats running. A tmux
restart or machine reboot ends live processes; chat records and inbox state
remain. Scheduled work follows each schedule's `on_missed` policy. Existing
interactive sessions can be resumed after reconnecting; they do not all start
automatically at boot.

To update, back up configuration and data, verify and extract a new bundle,
and rerun its installer. It preserves existing configuration and credentials,
replaces the binaries and unit files, and restarts only Hive. Put service
customizations in `systemctl --user edit hive-server.service` drop-ins, since
the installer replaces the supplied unit. Server auto-updates are disabled.

The self-signed TLS certificate lasts one year. To renew it, stop Hive, move
`~/.local/share/hive-server/data/desktop/remote-tls/identity.pem` to a private
backup, and start Hive again. Reconnect through SSH to obtain the new public
certificate. The bearer token rotates at backend startup; Desktop refreshes it
through SSH when the old token is rejected. Never share `connection.json`.

For a consistent backup, stop Hive before copying its configuration, engine,
and desktop data. Include workspace repositories and agent credentials if
required by your recovery plan. Database migrations can make binary-only
rollback unsafe; restore the matching data backup when reverting versions.

To stop all background work and terminals:

```sh
systemctl --user disable --now hive-server.service
systemctl --user stop hive-server-tmux.service
```

Remove the two unit files under `~/.config/systemd/user/`, then run
`systemctl --user daemon-reload` to uninstall the services. Configuration and
data remain until you choose to remove them. Do not disable account lingering
if other user services rely on it.

## Troubleshooting and verification

- **Cannot connect to the user bus:** SSH into the account normally and check
  `dbus-user-session` and `loginctl show-user "$USER" -p Linger`. Do not run
  `systemctl --user` through a root shell.
- **Address in use:** change the ports in the private environment file and
  restart Hive. The exported connection file follows the TLS listener address.
- **Agent not found or not signed in:** run it as the service account, check
  its absolute command/PATH, and inspect the service logs.
- **TLS failure:** check the server clock, certificate expiry, and connection
  file path. Renew the identity as above; do not disable certificate checks.

`mise run desktop:server:test` builds the bundle and tests installation on
Ubuntu 24.04 in Docker without a GUI or keychain. It validates the service
units and checks inbox and live-chat survival across a backend restart.
It does not boot systemd in Docker. On the home server, also log out and
reconnect, then reboot once and verify the enabled service, persisted inbox,
and schedule catch-up. The unattended test is documented with the
[remote fixture](https://github.com/colonyops/hive/tree/main/cmd/desktop/remote).
