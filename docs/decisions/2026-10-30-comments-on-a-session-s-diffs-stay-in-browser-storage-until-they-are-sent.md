# Comments on a session's diffs stay in browser storage until they are sent

- **Status:** accepted
- **Date:** 2026-10-30

## Context

A comment on a Drive document is a row in `hive.db`, keyed by the document and its
hash, because a document changes rarely and its comments must outlive a rewrite.
A comment on a session's diff points at a line number in a file that an agent is
editing while the user reads, and the review is meant to go to that agent within
minutes. `review_comments` has no file path or diff side, so storing these there
needs a migration in the store both programs share.

## Decision

1. **`stores/useChangesReview` keeps them in browser storage, per session, until
   they are sent.** A relaunch keeps them. Nothing else reads them: the CLI has no
   view of a session's diff comments.

2. **Each comment carries its own quote.** The rows it covered are copied when it is
   made, so the feedback still says what the user meant after the lines move. The
   card shows a warning dot when the file's version differs.

3. **Go owns the feedback text.** The review goes to `ReviewService.SendChanges`
   whole, and `review.DiffFeedback` formats it, as `review.Feedback` does for a
   document. Delivery is the document path, limited to the panes of that one session.

## Consequences

- Reviews are per machine and per browser profile, and clearing site data loses them.
- Deleting a session from Hive Desktop drops its comments, after a warning and an
  offer to copy them. A session deleted any other way, or by a prune, leaves them in
  storage, where nothing shows them.
- The last three sent reviews per session stay in storage too, because tmux accepting
  a paste does not mean the agent read it.
- Moving these to `hive.db` later needs only the store to change, plus a migration
  that adds the path and sides to `review_comments`.
