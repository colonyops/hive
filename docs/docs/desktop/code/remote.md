---
icon: lucide/network
description: Attach to existing Hive sessions on another machine through SSH.
---

# Remote sessions

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

1. Start the remote backend with its own Hive data/config directories and a
   fixed `HIVE_DESKTOP_HTTP_PORT`, for example `19001`.
2. Set `HIVE_DESKTOP_CONNECTION_FILE` to a file in an existing private directory.
   Hive writes a mode-0600 JSON file containing its address and per-run token.
3. Forward the terminal API through SSH:

   ```sh
   ssh -N -T -o ExitOnForwardFailure=yes -L 127.0.0.1:19001:127.0.0.1:19001 my-host
   ```

4. Retrieve the JSON file through SSH. Open **Settings ▸ Remote**, add a
   connection name and color, and enter `http://127.0.0.1:19001` and its `token`
   value. Click **Connect** to open remote Code.
5. Select an existing session. The command palette also offers **Go to remote
   Code** and the connected remote session names.

Names, tunnel addresses, and colors are saved on this Desktop. Each connection
has its own color, shown in Inbox, Code, and Chats. Configure a saved connection
in **Settings ▸ Remote** to reconnect or switch machines; one remote can be
connected at a time.

Only loopback HTTP addresses are accepted; SSH carries the connection to the
other machine. The token stays in memory and is cleared by Disconnect or closing
Desktop. Keep the backend's listeners on loopback and run one backend per
installation.

## Reconnect

An interrupted connection leaves the agent running remotely. Restore the tunnel
and click **Reconnect** to repaint the terminal from tmux. If the backend itself
restarts, retrieve its new token and connect again. Failed session refreshes keep
the last known list.

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
