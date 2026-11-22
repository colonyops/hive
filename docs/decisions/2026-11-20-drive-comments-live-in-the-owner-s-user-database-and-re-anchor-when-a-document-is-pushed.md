# Drive comments live in the owner's user database and re-anchor when a document is pushed

- **Status:** accepted
- **Date:** 2026-11-20

## Context

People who open a pushed plan in the drive server's web app need to comment
on a sentence and see who else is reading. The owner's agent needs to read
those comments from the CLI and answer them. The document's text changes
only by push from a local file, so a comment must survive a new version.
The server is one container with no other services
([ADR](2026-10-15-hive-drive-server-is-a-multi-tenant-program-with-one-sqlite-database-per-user.md)).

## Decision

- **Storage.** Threads and comments live in the document owner's
  `users/<id>.db`, beside the document, and a document delete cascades to
  them. They do not count toward the owner's quota, because other people
  write them. Per-document caps bound them instead.
- **The link rule, amended.** The multi-tenant ADR made a share link the only
  path from one user's document to another person. A link is now also the
  only path in: any signed-in account with push scope can write comments
  through it, and every id a link call names must belong to the link's one
  document. Anonymous readers can read threads and appear in presence, but
  cannot write.
- **Anchoring.** An anchor is a source line range, front matter counted,
  plus the quoted text. After each push the server moves anchors with a line
  diff, then searches for a changed range's quote with case and punctuation
  ignored, nearest the old position. An anchor with no match is outdated and
  keeps its quote; a later push brings it back when the quote returns. HTML
  documents take general threads only.
- **Re-anchoring runs after the upsert,** outside its transaction, so a large
  diff does not hold the user file's one connection. A per-path lock keeps
  two pushes to one document in order. Each thread records the document
  version it was checked against, so a push moves exactly the threads on the
  version it replaced, and a thread is created only against the version the
  client rendered.
- **Presence and signals** run through one in-memory hub per process. A
  server-sent event stream carries only signals and presence; content always
  comes from REST, and every stream open re-reads. Presence carries line
  ranges, never text, and a heartbeat must carry the secret its stream issued.

## Consequences

A second server replica would need a shared bus for the hub; until then the
server stays one process. The per-IP stream cap and rate limits depend on
`trusted_proxies` behind a proxy. A quote that spans a link's URL in the
source does not match and goes outdated. Every existing share link accepts
comments from any account that can sign in to the server.
