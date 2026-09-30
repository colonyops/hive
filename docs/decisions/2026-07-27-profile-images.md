# Profile images: normalized PNG in the data dir, hash-referenced from the flow

- **Status:** accepted
- **Date:** 2026-07-27

## Context

A profile *is* a flow, and the flow document carried no visual attributes — the
rail derived a single-letter chip from the name. Once a user has more than a
couple of profiles the letters stop distinguishing them, so a profile needs an
avatar shown in the sidebar rail.

Two things had to be decided before implementing: **scope** (a curated preset
icon reusing the `icons` allow-list, versus a real uploaded image) and
**storage** (a base64 blob inline in the flow YAML, the image as a config-dir
sibling file like `<id>.ui.yaml`, or the image in the app data dir referenced
from the flow). There was also no mechanism in the app for storing or serving a
user-supplied image at all.

## Decision

**Support a real uploaded image, normalized to a small square PNG, stored in the
app data dir and referenced by content hash from the flow YAML.**

- **Storage is `internal/app/profileimg`** — a leaf `Store` over
  `<StateDir>/assets/profiles/`, one `<flow-id>.png` per profile. It owns
  normalization: decode (PNG/JPEG/GIF/WebP via `image.Decode` + `x/image`),
  center-crop to a square, downscale to 128px with `x/image/draw`, re-encode
  canonical PNG, and return the stored bytes' content hash. Normalization lives
  in the core, not a caller, so a future HTTP or MCP setter produces the same
  stored shape — the Wails settings view is just the first setter.

- **The flow YAML records only a hash.** `Flow.Image` (a new `image:` key,
  read by the strict decoder and written by `SaveFlow`) is the content hash; the
  bytes live under the data dir. This keeps `flows/*.yaml` small and text — a
  base64 blob inline was the thing to avoid — while still recording, in config,
  that a profile has an avatar. A config synced to a fresh machine without its
  data dir carries the reference but no file; that reads as **no image** and the
  rail falls back to the letter chip, never an error.

- **The image is owned by `SetImage`, not the graph editor.** The editor's
  `SaveFlow` round-trips only `{id, name, enabled, nodes, wires}` (see
  `wireFlow.ts`), so a graph save would otherwise drop the `image:` key.
  `FlowStore.Save` therefore preserves whatever image the loaded flow already
  declares, and the reference changes only through
  `FlowsService.SetProfileImage` / `ClearProfileImage`.

- **Serving is a data URL, not a new asset route.** The stored PNG is tiny, so
  `FlowSummary.Image` carries it inline as a `data:image/png;base64,…` URL,
  built in the Wails adapter (a transport encoding, so it stops at the adapter).
  The rail stays a pure prop render — `<img>` when set, letter chip otherwise —
  with no second fetch and no custom Wails scheme handler.

- **Cleanup rides the existing delete cascade.** `FlowsService.Delete` removes
  the stored image best-effort after the flow files and before purging durable
  rows; a leftover avatar is orphaned data, never a reason to fail the delete.

The cheaper preset-icon-plus-color option was declined for now: the ask was an
image, and the icon allow-list pattern remains available if a non-image glyph is
wanted later.

## Consequences

- The app gains its first user-supplied asset store and its first data-URL
  serving seam. Both are deliberately narrow — one square PNG per flow id, read
  on demand — and neither is a general asset framework.
- `golang.org/x/image` is a new dependency, pulled in for high-quality
  downscaling and WebP decode. The server build stays CGO-free (`x/image` is
  pure Go).
- A profile's avatar is app-local state, not dotfiles config: it survives
  restart, rename (the flow id is stable), and enable/disable, and is removed on
  delete, but does **not** travel with a dotfiles-managed `flows/` directory.
  That is the accepted cost of keeping the flow file free of binary blobs; the
  letter-chip fallback makes a missing file harmless.
- Reversing course to make avatars portable means either inlining them in the
  flow YAML or moving them to a config-dir sibling file, and belongs in a
  superseding decision.
