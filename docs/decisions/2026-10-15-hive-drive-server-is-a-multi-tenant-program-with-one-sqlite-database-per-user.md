# Hive Drive Server is a multi-tenant program with one SQLite database per user

- **Status:** proposed
- **Date:** 2026-10-15

## Context

Hive Drive documents live on one machine. People who want to read, share, or
push them from elsewhere need a server they can deploy as one container,
with no second auth system and no extra database service. Several people
may use one server, and a public server (a `*` policy) lets anyone with an
inbox store data on it. A document must never reach a user who does not own
it, except through a share link its owner made.

## Decision

- **Program boundary.** The server is `cmd/drive-server`, a fourth program in
  the module. It builds `hive-drive-server`, ships as the
  `ghcr.io/colonyops/hive-drive-server` image, and carries the shared
  version. It imports the engine layers below `cmd/` and no other program;
  `drive-server-no-program-deps` enforces that.
- **Tenancy.** Every document has exactly one owner. A share link is the
  only path from one user's document to another person, and it names one
  document.
- **Storage.** `control.db` holds accounts, sessions, tokens, sign-in codes,
  the mail outbox, and share links. `users/<id>.db` holds one user's
  documents. Every primary key is a UUIDv7. A UUIDv7 is never a secret;
  slugs, tokens, and codes are separate random values.

## Consequences

Isolation is structural. A handler holds one user's handle and cannot read
another user's rows, so a missing `WHERE user_id = ?` cannot leak data. One
shared database would force every query to filter on the user, and one
missed filter would expose a tenant. The same layout gives each user a
separate writer, deletes a user by deleting a file, and computes quota in
the user's own file.

The cost is operational. The server keeps an LRU of open user handles and
migrates each user file on first open after an upgrade. A share link lives
in `control.db` and its document in a user file. A delete removes the
links first, then the document. A link whose document is missing resolves
to 404. Backup is a directory, taken with `VACUUM INTO` or a stopped
container, never a copy of a live WAL file. A cross-user isolation test suite guards the
assumption that the store opens the right file.

The HTML frame was reviewed against hostile documents in three browser engines;
see [SECURITY-REVIEW.md](../../cmd/drive-server/SECURITY-REVIEW.md).
