---
icon: lucide/server
description: Show, create, and open sessions on other machines in the Code area.
---

# Servers

Code lists the sessions of each server under `servers:` in the hive `config.yaml` with your local sessions. A server is another machine that runs hive and that Hive Desktop reaches over ssh. Remote servers are experimental.

To set up a server and add it to `config.yaml`, follow [Remote servers](../../cli/getting-started/remote-servers.md). Hive Desktop and the hive CLI read the same list.

## Sessions on a server

A server's session sits in its repository's group, after your local sessions, with a server icon in that server's color. Hover the icon for the server's name. Each repository header has a colored dot for every server with a session in it. When a server session is open, the bar above its terminal names the server.

To create a session on a server, pick it in the Server field of the New Session dialog, or run **New session on** and the server name from the command palette. The dialog then lists that server's repositories and agents.

On a server's session you can:

- open its terminal, which runs over ssh
- start it when its terminal stopped; the server starts the agent with its own rules
- rename it, edit its tags, recycle it, and delete it
- see its branch, changes, and pull request in the status bar

Open in editor, Show in Finder, agent windows, and configured actions run on your machine, so Code does not offer them for a server's session. A create on a server does not take an agent workspace.

## Server states

**Settings ▸ Servers** shows how each server answered its last check.

| State | Meaning | Fix |
| --- | --- | --- |
| Connected | The server answered | |
| Offline | ssh or the daemon did not answer | Read the ssh error on the server's banner in Code. Check that `ssh <host>` logs in without a prompt. |
| Incompatible | The server's hive speaks another protocol version | Install the same hive version on both machines |

An offline server shows as a banner above the list, with **Retry**. Hover it for the ssh error. Its sessions stay in place, struck through, until it answers again. It does not slow down the rest of the list.
