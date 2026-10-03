# Pure shared domain packages are imported directly; services and stores stay behind the seam

- **Status:** superseded by [both-programs-run-on-one-layered-hive-engine](2026-10-03-both-programs-run-on-one-layered-hive-engine.md)
- **Date:** 2026-10-02

> **Superseded (2026-10-03):** the seam is gone. `app` imports every shared
> package by layer, and shared types appear in `app` signatures.

## Context

`cmd/desktop/internal/app` reached every shared package through
`dispatch/hive_adapters.go`, and depguard denied `internal/core` outright. The
seam exists to keep the CLI's services, stores, and types out of desktop
signatures, so a shared-side change breaks one adapter instead of the app.

That works for an adapter that converts something. For a pure function such as
`session.Slugify`, the adapter forwarded the call with the same signature, so a
change broke every caller anyway. Since the repositories merged into one
module, the desktop and the CLI must also agree on rules such as session
naming. A separate copy of a rule behind the seam is a bug, not isolation.

## Decision

`app` imports a shared package directly when it is a pure domain primitive:
no I/O, no configuration, no store, and no imports from `internal/` other than
other primitives. Today those are `internal/core/session` and
`internal/core/validate`. The depguard rule `app-shared-via-seam` still denies
`internal/core` and allows those two paths in lax mode, where the longer match
wins, so a new core package is denied until someone allows it.

A primitive's functions, constants, and sentinel errors may be used anywhere in
`app`. Its types still convert at the seam: `session.Session` does not appear in
an `app` signature.

## Consequences

The session-name wrappers in `hive_adapters.go` and the translated
`ErrDuplicateSessionName` are gone. Adding a primitive means adding it to the
depguard allow list in the same PR that first needs it. The wider package
structure, which still mixes primitives with CLI features under
`internal/core`, is tracked in #575.
