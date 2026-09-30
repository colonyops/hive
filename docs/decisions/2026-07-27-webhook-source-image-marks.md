# Webhook source image marks: content-addressed PNG in the data dir, hash in the flow

- **Status:** accepted
- **Date:** 2026-07-27

## Context

A `sources.webhook` node's feed mark was a glyph *name* from the curated
`internal/app/icons` allow-list, rendered as a build-time Lucide component.
Issue #91 asked to let a webhook source show an uploaded **image** — the
sending service's logo — as its feed and detail-pane mark instead.

Two in-repo precedents pointed different ways. The GitHub avatar consumes a
remote URL rendered in an `<img>`. ADR profile-images, landed the same day, built the
first user-supplied image store (`internal/app/profileimg`): a profile avatar
is uploaded, normalized to a square PNG under the data dir, and referenced by
content hash from the flow — and it explicitly rejected a base64 blob inline in
the flow YAML.

The deciding difference is where the reference lives. A profile's avatar is a
flow-level attribute *outside* the node/wire round-trip, so `SetProfileImage`
owns it and `FlowStore.Save` preserves it. A webhook node's mark is ordinary
**node config** — it round-trips through the graph editor exactly like the
glyph `icon`. That rules the profileimg "separate setter owns the reference"
shape unnecessary here, but raises a new question: the bytes must be stored
*before* the graph save that records them.

## Decision

**Follow the profileimg pattern — upload, normalize, store in the data dir,
reference by content hash — with the reference carried as normal node config
and the store keyed by content rather than by id.**

- **Storage is `internal/app/sourcemark`** — a leaf `Store` over
  `<StateDir>/assets/webhookmarks/`. `Set(raw)` decodes (PNG/JPEG/GIF/WebP),
  guards size and dimensions, normalizes, re-encodes canonical PNG, and returns
  the stored bytes' content hash; the file is `<hash>.png`. **Content-addressed,
  not id-keyed**, because the mark is uploaded before the graph save that
  records it: keying by content lets the upload stay a pure bytes → hash
  function with no node identity to thread, and identical logos share one file.

- **Normalization fits, it does not crop.** The image is scaled to fit within a
  128px square and centered on a transparent canvas, so a wide logo is
  letterboxed intact. profileimg center-crops because a face fills a circular
  avatar; a logo's edges must survive, so this is the one deliberate divergence
  from that store.

- **The flow records the hash as ordinary config.** `webhook.Config.Image` is a
  new `image:` field beside `icon`, round-tripped through `GetFlow`/`SaveFlow`
  like every other source-node field — so, unlike the profile avatar, it needs
  no preserve-on-save seam. `Validate` checks only that the reference is a
  well-formed hash (or empty); a reference whose file is missing reads as no
  mark, exactly as `icon`'s empty value does.

- **Serving to the desktop feed is a data URL, not a route.**
  `WebhookService.SetMarkImage` normalizes an upload and returns the hash plus
  the stored PNG as a data URL for immediate preview; `WebhookService.MarkImages`
  resolves a set of hashes to data URLs. The feed builds a node-id → data-URL
  map for the active flow the same way it builds `sourceIcons`, and `SourceMark`
  renders an `<img>` — with an `@error` fallback to the glyph — instead of the
  Lucide component when a node has an image.

- **The agent HTTP API mirrors the profile avatar.** The desktop editor sets a
  mark in two steps (store, then a graph save that carries the hash), because it
  owns the unsaved graph. An agent has no such graph, so the agent API
  (`internal/adapter/httpapi`) exposes a **one-call, node-scoped** endpoint that
  does both: `PUT /api/flows/{flowId}/nodes/{nodeId}/image` takes the raw image
  bytes, normalizes and stores them, sets the node's `image`, and saves the flow
  — the direct parallel to `PUT /api/profiles/{id}/image`. `GET` returns the
  128×128 PNG (404 when unset) and `DELETE` clears it. It is backed by
  `FlowsService.SetNodeImage`/`ClearNodeImage`/`NodeImage`, which mutate one
  node's image through a connector-neutral `FlowStore.SetSourceImage` (the flow
  package recognises the image-mark capability structurally, so it never names
  the webhook connector). A node that is not a webhook source is a 400. The same
  core methods become an MCP tool later. Node ids are discovered by the means
  that already exist (reading the flow); the API does not yet enumerate a flow's
  graph.

The remote-URL avatar model and a `data:` URI stored inline in the flow YAML
were both weighed and declined: the former phones home from the webview and has
no normalization; the latter is the base64-in-config blob ADR profile-images already
rejected.

## Consequences

- The app gains its **second** user-supplied asset store, again deliberately
  narrow — one content-addressed PNG store, read on demand. It is not merged
  with `profileimg`: the two differ in keying (content vs. flow id) and in
  normalization (fit vs. crop), and ADR profile-images already set the "narrow, not a
  general asset framework" posture.
- The mark is **app-local state, not dotfiles config**: it does not travel with
  a `flows/` directory synced to a machine without its data dir. A hash with no
  file falls back to the glyph, never an error — the same tolerance the
  letter-chip fallback relies on.
- **Orphans are tolerated.** A replaced or removed mark leaves its old
  `<hash>.png` on disk; there is no ref-count or GC. The blobs are tiny and
  harmless, matching ADR profile-images's stance that orphaned asset data is never a
  reason to fail an edit. A sweep can be added if it ever matters.
- **Webhook-only, for now.** The `icons` glyph allow-list stays and is the
  fallback. Extending image marks to the feed terminal's `icon` is a follow-up,
  not built here. *(Superseded in part: `sources.exec` gained the same mark in
  #289. The capability is structural — a connector config implementing
  `MarkImage`/`SetMarkImage` has one — so extending it needed no new decision,
  only moving the upload RPCs off `WebhookService` onto `FlowsService`.)*
