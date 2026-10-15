---
icon: lucide/network
description: Attach to existing Hive sessions on another machine through SSH.
---

# Remote sessions

For a machine without a GUI, use the
[Ubuntu server installation guide](../getting-started/ubuntu-server.md).

**Settings ▸ Remote** connects to existing Hive Code sessions on another
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

Remote Code session creation, inbox triage, Drive, image/file transfer, editor
integration, and automatic tunnel management are not included yet. Create Code
sessions using Hive on the remote machine. **New session** in Code switches to
Local.

For a reusable Docker host and local Desktop setup, see the repository's
[remote fixture instructions](https://github.com/colonyops/hive/tree/main/cmd/desktop/remote).
