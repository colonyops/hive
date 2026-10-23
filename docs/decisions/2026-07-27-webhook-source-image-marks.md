# Webhook source image marks: content-addressed PNG in the data dir, hash in the flow

- **Status:** accepted
- **Date:** 2026-07-27

## Context

A source node's feed mark was a glyph name from the curated `icons`
allow-list. Issue #91 asked for an uploaded image (the sending service's logo)
instead. ADR profile-images had already built an upload, normalize, store and
hash-reference pattern for avatars. The difference here is that a mark is
ordinary node config that round-trips through the graph editor, so its bytes
must be stored before the graph save that records them.

## Decision

**Follow the profile-images pattern, with the reference carried as node config
and the store keyed by content rather than by id.**

- `cmd/desktop/internal/app/sourcemark` is a leaf `Store` over
  `<StateDir>/assets/webhookmarks/`. `Set(raw)` decodes (PNG, JPEG, GIF,
  WebP), guards size and dimensions, normalizes, and writes `<hash>.png`.
  Content addressing keeps the upload a pure bytes-to-hash function with no
  node identity to thread, and identical logos share one file.
- **Normalization fits, it does not crop.** The image is scaled to fit a 128px
  square and centered on a transparent canvas, so a wide logo stays intact.
- **The capability is structural.** A source config that implements
  `MarkImage()` / `SetMarkImage()` has an image mark (today `webhook`, `exec`,
  `rss`), stored as an `image:` field beside `icon`. `Validate` checks only
  that the hash is well-formed. A hash with no file reads as no mark and falls
  back to the glyph.
- **Serving is a data URL.** The Wails `FlowsService.SetMarkImage` stores an
  upload and returns the hash plus a data URL for preview; `MarkImages`
  resolves hashes to data URLs. `SourceMark.vue` renders an `<img>` with a
  fallback to the glyph.
- **Agents set a mark in one call.** The editor stores and then saves the
  graph, because it owns the unsaved graph. An agent has none, so
  `FlowsService.SetNodeImage` / `ClearNodeImage` / `NodeImage` store the bytes
  and set the node's `image` through `FlowStore.SetSourceImage`, which never
  names a connector. The MCP server exposes them as `set_node_image`,
  `clear_node_image` and `get_node_image`.

## Consequences

- `sourcemark` and `profileimg` stay separate stores: they differ in keying
  (content vs. flow id) and normalization (fit vs. crop).
- A mark is app-local state and does not travel with a synced `flows/`
  directory.
- Orphans are tolerated. A replaced mark leaves its old `<hash>.png`; there is
  no ref-count or GC.
