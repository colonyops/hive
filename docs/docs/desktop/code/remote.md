---
icon: lucide/network
description: Create and attach to Hive sessions on another machine through SSH.
---

# Remote sessions

For a machine without a GUI, use the
[Ubuntu server installation guide](../getting-started/ubuntu-server.md).

**Settings ▸ Remote** connects to Hive Code sessions on another
machine. Select a session to view its tmux windows and panes, type, paste text,
and resize the terminal. Connected remote inboxes, Code sessions, and chat
workspaces appear alongside local entries in the same navigation menus. A narrow
edge in your chosen color marks remote entries; hover to see the connection name.

The remote machine runs Hive, tmux, and your agent CLI. The CLI uses the account
and credentials configured on that machine.

## Connect

This preview requires a source-built remote backend with the `server,terminals`
build tags. The ordinary `desktop:serve` build keeps terminals disabled.

1. Start the remote backend with its own Hive data/config directories. Set
   `HIVE_DESKTOP_CONNECTION_FILE` to a file in an existing private directory,
   such as `~/.config/hive/remote/connection.json`.
2. Hive starts a separate HTTPS API on loopback and writes its address, token,
   and public certificate to that private file. For a fixed port, set
   `HIVE_DESKTOP_REMOTE_ADDRESS=127.0.0.1:19003`.
3. Configure and verify SSH access from your laptop. Desktop uses your SSH
   config and agent, with strict host-key checks and no password prompts.
4. Open **Settings ▸ Remote**. Enter an SSH host alias, the remote connection
   file path, a name, and a color. Click **Connect**.
5. Select a remote entry in Inbox, Code, or Chats.

Go retrieves the credentials over SSH and verifies HTTPS/WSS with the exported
certificate. SSH carries encrypted TLS traffic without a local forwarded port.
Remote tokens stay in Go memory and never enter the webview. Native Desktop
uses IPC for the remote terminal display. Development and headless browser
builds still use their local Wails HTTP origin.

Profiles persist on your laptop; only one remote connection is active at a
time. Existing manual tunnel profiles need a new SSH profile. Keep client and
remote builds compatible. The remote agent uses credentials on its own machine.

## Reconnect

Disconnect leaves remote agents running. Restore SSH access and click
**Reconnect** to attach again. Hive retrieves a fresh token automatically when
an API request reports that the old token expired. A network failure does not
repeat an action that may already have run.

The TLS identity persists under the remote state directory in
`remote-tls/identity.pem`. Its certificate expires after one year. To renew it,
stop the remote backend, remove that identity file, restart, and reconnect from
Settings. The new certificate is retrieved through verified SSH. Never disable
certificate verification to work around a connection failure.

## Remote inboxes and chats

Choose a remote inbox in the Inbox rail to read its feeds. This preview
shows up to 200 active items per feed and does not change their read or archived
state.

In Chats, choose a remote workspace, enter a chat name, and click
**New chat**. The workspace's configured command runs on the remote machine.
Select a previous remote chat to reopen it. Closing the view leaves the remote
process running. The agent uses the account signed in on that machine.

## Code repositories and sessions

Code lists remote repositories alongside local repositories, with the connection
color marking the remote groups. Expand a repository to see its sessions and
the agent and terminal windows under each session. Select a window to attach.
The session's **New window** menu offers a terminal and the remote machine's
configured agents.

**New session** opens the same form for both machines. The repository picker
shows **Local** or the remote connection name beside each choice. The same
repository can appear twice, once for each machine. Selecting a remote choice
uses that machine's agent profiles and creates the checkout there. The form
waits for the remote creation job and shows any failure without discarding your
input. The repository's **+** button preselects that repository and machine.

The session status bar shows the remote checkout's branch, changes, and pull
request using the remote machine's forge credentials. It follows the same
status-bar visibility setting as local sessions.

If a terminal loses its connection, **Reconnect server** reconnects the entire
remote installation and reattaches its open terminals. Your selected inbox,
workspace, chat, Code session, and terminal window are preserved. Remote tmux
processes keep running while Desktop is disconnected.

Inbox triage, Drive, image/file transfer, and editor integration remain local-only.
Only one remote connection can be active at a time.

For a reusable Docker host and local Desktop setup, see the repository's
[remote fixture instructions](https://github.com/colonyops/hive/tree/main/cmd/desktop/remote).

Clicking **Connect** saves the connection profile before attempting to reach the
server. A failed attempt leaves the profile available for later use; only host
settings are saved, not the remote token.

Hive checks the connected server every ten seconds. A failed check or lost
terminal stream shows a **Disconnected** banner with **Reconnect server**,
even while viewing another area. This monitors the Hive transport, not the
internal connections of an agent running inside tmux. An agent's own server
error can require restarting that agent while the Hive connection remains live.
