---
icon: lucide/message-square
description: Comment on pushed documents in the drive server's web app, see who is reading, and answer threads from the CLI.
---

# Comments

A pushed markdown document takes comment threads in the drive server's web app. A thread is on a selection of text, or on the whole document. HTML documents take whole-document threads only.

## Add a comment

Select text in the document, or use the handle in the margin beside a block, and type your comment in the box that opens. Open **Comments** in the header to see the threads beside the text they are on. A comment on the whole document goes in the box at the bottom of that column.

## Who can comment

- The owner comments on the document page.
- Anyone who can sign in to the server comments through the document's [share link](sharing.md). A share link page shows **Sign in to comment** to a reader who is not signed in.
- A reader who is not signed in sees every thread with the authors' names, but no email addresses, and cannot write.
- An API token with `read` scope cannot comment.

The owner and a thread's author can resolve or reopen it. Only a comment's author can edit it. Its author or the owner can delete it.

Comments show your display name, or the part of your email before `@`. Set a display name from the account menu.

## When the document changes

Threads follow their text when you push a new version. A thread on text the new version removed moves to **Outdated** and keeps its quote. If a later push puts the text back, the thread returns to it.

## Presence

The document header shows who has the page open, with the lines each person has selected. A reader who is not signed in gets a generated name, which they can change. Presence carries line numbers only, never the selected text. An account with the page open in more than one tab shows once.

## Read and answer from the CLI

```bash
hive drive comments plans/ship.md
hive drive comments reply plans/ship.md 5d0e77b1 "Changed to 48h."
hive drive comments resolve plans/ship.md 5d0e77b1
hive drive comments reopen plans/ship.md 5d0e77b1
```

- The list shows open and outdated threads, each with its last 8 id characters, its lines, its quote, and every reply. `--all` adds resolved threads.
- `--json` prints one thread per line, for agents.
- A thread is named by its id or any unique end of it.
- `reply`, `resolve`, and `reopen` need a token with `push` scope. A reply written with a token shows an agent badge with the token's name.
- The command picks the space and server the way [push](cli.md#choose-a-space) does.

## Limits

| Limit | Value |
| --- | --- |
| Threads per document | 500 |
| Comments per thread | 200 |
| Comment text per document | 2 MiB |
| One comment | 8 KiB |
| Comment changes per account | 30 a minute |

Comments do not count toward the owner's storage quota.
