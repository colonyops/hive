---
icon: lucide/link
description: Open a Code session, a chat, an inbox item, or a Drive document in Hive Desktop from a hive:// link.
---

# Links

!!! note "Experimental"
    The link format can change.

Hive Desktop registers the `hive://` URL scheme. Opening a `hive://` link from a browser, a script, or another app raises the Hive window and goes to the place the link names. Hive starts first if it is not running.

A link only navigates. It never creates a session, runs a command, or changes data.

## Link formats

| Opens | Link |
| --- | --- |
| A tmux session in Code | `hive://code/<slug>` |
| A window in that session | `hive://code/<slug>?window=@3` |
| A Chats workspace | `hive://chats/<workspace>` |
| A chat | `hive://chats/<workspace>/<chat-id>` |
| A profile's inbox | `hive://inbox/<profile>` |
| A feed | `hive://inbox/<profile>?feed=<feed-id>` |
| An inbox item | `hive://inbox/<profile>/<item-id>` |
| A Drive document | `hive://drive/<space-key>/<path>` |
| A Drive space | `hive://drive/<space-key>` |
| A canvas | `hive://canvas/<name>?owner=<owner>` |

- `<slug>` is the tmux session name, so a link can open a session that another tool started, as long as it runs on the same tmux server as Hive.
- A space key is one of three forms:
    - `shared` for the Shared space, as in `hive://drive/shared/notes.md`.
    - `workspaces/<name>` for a Chats workspace, as in `hive://drive/workspaces/hive/canvases/release.canvas.json`.
    - `<owner>/<repo>` for a repository, as in `hive://drive/colonyops/hive/plans/2026-10-11-drive.md`.
- The path after the key is the document's path inside the space. A link to a space with no path opens that space.
- Add `canvas=<name>` to a Code or chat link to open the Drive pane beside it on that canvas.
- The older canvas link still opens the canvas. Its owner is a Chats workspace, a repository as `owner/repo`, or `@global`.
- Escape special characters in each part, so a canvas named `release plan` is `hive://canvas/release%20plan?owner=colonyops%2Fhive`.

## Open a link from the CLI

`hive desktop open` (or `hive dt open`) checks a link and opens it. `hive desktop --help` lists the formats above.

```bash
hive dt open 'hive://code/my-session?window=@2'
```

## A link does nothing

Both sides log to the shared `hive.log` (see [Report a problem](../getting-started/troubleshooting.md#report-a-problem) for its path). Search it for `hive://`:

- `opening hive:// link` and `opener accepted hive:// link` come from the CLI. `opener failed` means the OS has no handler for `hive://`, so install Hive Desktop or reinstall it to register the scheme.
- `received hive:// link` means Hive Desktop got the link, and `frontend took hive:// link` means the window started routing to it. `ignoring hive:// link` names the part that did not parse.
- On Linux, `single-instance lock off` means a link starts a second Hive instead of reaching the running one.

!!! note "Installed builds only"
    The packaged macOS app and the Linux install script register the scheme. A build run with `mise run desktop:dev` does not receive links.
