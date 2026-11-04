# Agent reviews of a session's diff live in hive.db and are sent with the user's comments

- **Status:** accepted
- **Date:** 2026-11-04

## Context

An agent can review a Code session's changes and the user wants to read that
review inline in the Diff view. The agent can be the session's own, a Hive chat,
or one with only the hive CLI, and the desktop need not be running when it
writes. The user's own diff comments stay in browser storage
(ADR comments-on-a-session-s-diffs-stay-in-browser-storage-until-they-are-sent),
which no other process can write.

## Decision

1. **Agent reviews are rows in `hive.db`**: `diff_reviews`, one per session and
   author, and `diff_review_comments`. A new review from an author replaces that
   author's earlier one. The rows cascade from `sessions`. They are new tables,
   not `review_comments`, because a document review is keyed by a path and a
   content hash and a diff has neither.

2. **Comments anchor to the branch comparison against the default base**, the
   pull request view. `diffreview.Service` refuses a file outside that diff and
   lines that are not rows of it, and quotes the rows itself with the same
   parser the desktop uses. The desktop marks a comment stale when its rows no
   longer read as quoted.

3. **The user only dismisses.** An agent comment the user does not dismiss goes
   out with the user's own comments when they press Send, its author and
   severity leading its text, and its row is deleted once the paste succeeds.
   There is no step that copies an agent comment into the user's review: the
   user disagrees by dismissing and adds to it by writing a comment of their own.

4. **One input type, two writers.** `internal/ops/reviews` declares
   `diff_review.put`, `.get` and `.clear` on the drive toolset, which the drive
   MCP server mounts and `hive dt` follows. `hive review diff` writes the same
   input straight to `hive.db`, so it needs no desktop.

## Consequences

- The desktop learns of every write through a `hivewatch` probe, its own as well
  as the CLI's, so a new review shows up within the probe interval rather than at
  once. A dismiss removes the card in the view before the probe sees it.
- A dismissed or sent comment is gone from `hive.db`; `diff_review_get` shows an
  agent only what is still waiting.
- An agent comment cannot be edited. A user who wants different wording dismisses
  it and writes their own.
- A comparison other than the branch against the default base shows no agent
  comment inline, only a note on the Overview.
- The CLI and TUI can read reviews (`hive review diff show`), but the TUI does
  not draw them.
