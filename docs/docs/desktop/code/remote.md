---
icon: lucide/network
description: Attach to existing Hive sessions on another machine through SSH.
---

# Remote sessions

**Code → Remote (preview)** connects to existing Hive Code sessions on another
machine. Select a session to view its tmux windows and panes, type, paste text,
and resize the terminal. Local Code, Inbox, and Chats still use this machine.

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

4. Retrieve the JSON file through SSH. In **Code → Remote (preview)**, enter
   `http://127.0.0.1:19001` and its `token` value.
5. Select an existing session. The command palette also offers **Go to remote
   Code** and the connected remote session names.

Only loopback HTTP addresses are accepted; SSH carries the connection to the
other machine. The token stays in memory and is cleared by Disconnect or closing
Desktop. Keep the backend's listeners on loopback and run one backend per
installation.

## Reconnect

An interrupted connection leaves the agent running remotely. Restore the tunnel
and click **Reconnect** to repaint the terminal from tmux. If the backend itself
restarts, retrieve its new token and connect again. Failed session refreshes keep
the last known list.

Remote session creation, Inbox, Chats, image/file transfer, editor integration,
and automatic tunnel management are not included yet. Create sessions using
Hive on the remote machine. **New session** in Desktop switches to Local.

For a reusable Docker host and local Desktop setup, see the repository's
[remote fixture instructions](https://github.com/colonyops/hive/tree/main/cmd/desktop/remote).
