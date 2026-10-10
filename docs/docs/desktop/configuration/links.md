---
icon: lucide/link
description: Open a Code session, a chat, an inbox item, or a canvas in Hive Desktop from a hive:// link.
---

# Links

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
| A canvas | `hive://canvas/<name>?owner=<owner>` |

- `<slug>` is the tmux session name, so a link can open a session that another tool started, as long as it runs on the same tmux server as Hive.
- Add `canvas=<name>` to a Code or chat link to open the canvas pane beside it.
- A canvas owner is a Chats workspace, a repository as `owner/repo`, or `@global`.
- Escape special characters in each part, so a canvas named `release plan` is `hive://canvas/release%20plan?owner=colonyops%2Fhive`.

## Build links with the hive CLI

`hive desktop url` prints a link and `hive desktop open` opens one. `hive dt` is the short form.

```bash
hive dt url code                          # the hive session for this directory
hive dt url code my-session --window @2   # any tmux session, on one window
hive dt url chat research 42 --canvas notes
hive dt url inbox work 7
hive dt url canvas plan                   # this repository's canvas
hive dt url canvas plan --owner @global

hive dt open code                         # the same arguments, opened now
hive dt open 'hive://inbox/work/7'        # or any link
```

Without a slug, `code` uses the hive session that holds the current directory. Without `--owner`, `canvas` uses that session's repository.

!!! note "Installed builds only"
    The packaged macOS app and the Linux install script register the scheme. A build run with `mise run desktop:dev` does not receive links.
