# Profile images: normalized PNG in the data dir, hash-referenced from the flow

- **Status:** accepted
- **Date:** 2026-07-27

## Context

A profile is a flow, and the rail showed only a letter chip derived from its
name, which stops distinguishing profiles past a couple. A profile needs an
uploaded avatar. The choices were where the bytes live (inline base64 in the
flow YAML, a config-dir sibling file, or the app data dir) and how the
frontend gets them.

## Decision

**Store an uploaded image, normalized to a small square PNG, in the app data
dir, and reference it by content hash from the flow YAML.**

- `cmd/desktop/internal/app/profileimg` is a leaf `Store` over
  `<StateDir>/assets/profiles/`, one `<flow-id>.png` per profile. It owns
  normalization: decode (PNG, JPEG, GIF, WebP), center-crop to a square,
  downscale to 128px with `x/image/draw`, re-encode as PNG, and return the
  content hash. Every setter (Wails, MCP) gets the same stored shape.
- The flow YAML records only the hash (`image:`). A config synced without its
  data dir has a reference but no file, which reads as no image and falls
  back to the letter chip.
- Only `FlowsService.SetProfileImage` and `ClearProfileImage` change the
  reference. The graph editor's save does not carry `image:`, so
  `FlowStore.Save` keeps whatever image the stored flow declares.
- The Wails adapter serves the image inline as a `data:image/png;base64,` URL
  on `FlowSummary.Image`. No asset route or custom scheme handler.
- `FlowsService.Delete` removes the stored image best-effort; a leftover file
  never fails the delete.

## Consequences

- An avatar is app-local state, not dotfiles config. It survives restart,
  rename (the flow id is stable) and enable/disable, but does not travel with
  a synced `flows/` directory. That is the cost of keeping flow files free of
  binary blobs.
- Making avatars portable means inlining them in the flow YAML or moving them
  beside it in the config dir, and needs a new decision.
