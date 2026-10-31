---
icon: lucide/server
description: Show, create, and open sessions on other machines in the Code area.
---

# Servers

Code lists the sessions of each server under `servers:` in the hive `config.yaml` with your local sessions. A server is another machine that runs hive and that Hive Desktop reaches over ssh. Remote servers are experimental.

To set up a server and add it to `config.yaml`, follow [Remote servers](../../cli/getting-started/remote-servers.md). Hive Desktop and the hive CLI read the same list.

## Sessions on a server

A server's session sits in its repository's group, after your local sessions, with a server icon in that server's color. Hover the icon for the server's name. Each repository header has a colored dot for every server with a session in it. When a server session is open, the bar above its terminal names the server.

To create a session on a server, pick it in the Server field of the New Session dialog, or run **New session on** and the server name from the command palette. Both offer connected servers only. The dialog then lists that server's repositories and agents.

On a server's session you can:

- open its terminal, which runs over ssh
- start it when its terminal stopped; the server opens the session's tmux windows and agent from its own `rules:`
- rename it, edit its tags, recycle it, and delete it
- see its branch, changes, and pull request in the status bar

Open in editor, Show in Finder, agent windows, and configured actions run on your machine, so Code does not offer them for a server's session. Code refuses an image pasted into its terminal, and a pop-up terminal in its directory, for the same reason. A create on a server does not take an agent workspace, and the dialog refuses a prompt above 32 KiB.

## Server states

**Settings ▸ Servers** checks each server when it opens, and again when you press **Check again**.

| State | Meaning | Fix |
| --- | --- | --- |
| Connected | The server answered | |
| Offline | ssh or the daemon did not answer | Read the ssh error on the server's row, or on its banner in Code. Check that `ssh <host>` logs in without a prompt. |
| Incompatible | The server's hive speaks another protocol version | Install the same hive version on both machines |

An offline or incompatible server shows as a banner above the list, with **Retry**. Hover the banner's text for the error. Sessions that Code already listed stay in place, struck through and without a menu, until the server answers again. After a restart of Hive Desktop, Code lists them only when the server answers. Your local sessions do not wait for it, but a server that hangs can hold back other servers' sessions for up to 5 seconds.
