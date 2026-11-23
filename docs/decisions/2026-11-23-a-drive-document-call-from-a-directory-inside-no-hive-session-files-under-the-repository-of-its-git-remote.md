# A Drive document call from a directory inside no hive session files under the repository of its git remote

- **Status:** accepted
- **Date:** 2026-11-23

## Context

ADR [agents-outside-every-session-share-one-global-canvas-owner](2026-10-04-agents-outside-every-session-share-one-global-canvas-owner.md) kept a working directory inside no hive session `not_found`, because the desktop resolved callers before the hive engine started, and a session's agent would otherwise file documents where its Drive tab never looks. The engine now starts before the MCP servers mount. The document operations became engine operations (#795), so `hive drive put` runs in-process, and its caller is often the current directory of a plain clone. ADR [engine-operations-run-from-one-handler-table-that-the-desktop-mounts-and-the-hive-cli-calls-in-process](2026-11-22-engine-operations-run-from-one-handler-table-that-the-desktop-mounts-and-the-hive-cli-calls-in-process.md) makes such a path global with that directory, which for Drive would mean the shared space.

## Decision

**A `document_*` call whose caller is a directory inside no hive session files under the repository space of that directory's git remote**, the space the hive sessions of that repository use and the one `hive drive ls` shows there. A directory with no remote that names an owner and repository stays `not_found`. It never falls back to the shared space; only the explicit caller `global` writes there.

`internal/hive/engineops` applies the rule. `Engine.DriveCaller` does not, so tab and diff review calls from such a directory stay `not_found`: they act on a session, and there is none.

## Consequences

- A write that was `not_found` now succeeds, in the same space a session of that repository writes to.
- Such a write has no author, like a `global` write, so it marks no chat or session as unseen.
