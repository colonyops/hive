# Webhook source

A **webhook source** node turns anything that can send an HTTP request into a flow input. The desktop runs a local listener on `127.0.0.1`; JSON POSTed to `http://127.0.0.1:<port>/hooks/<path>` becomes messages on this node's output, exactly like a GitHub source's poll would produce them.

The port is picked at random the first time Hive starts and then kept, so it differs per machine. Settings → Integrations → Webhooks shows this install's full endpoint URL, changes the port, or turns the listener off entirely.

## Fields

- `path` — the endpoint under `/hooks/`: slug segments separated by `/`, e.g. `ci-alerts` or `ci/deploys`. Several nodes (even across flows) may share a path — each enabled one receives every request.
- `secret` — optional shared secret. When set, requests must carry the same value in the `X-Hive-Secret` header or they are rejected with 401.
- `icon` — optional glyph, from the curated feed icon set, shown on this source's items. Empty uses the default webhook glyph.
- `image` — optional uploaded image (a service logo) shown as this source's mark instead of the glyph. It is a content hash of a normalized PNG kept in the app data dir, set through the editor's image picker; it is presentation only and never affects ingest. Empty falls back to `icon`.

## Delivery contract

- POST only, JSON body only (any shape — object, array, or scalar), capped at 1 MiB. Accepted deliveries return `202`.
- Item identity: a top-level `"id"` (string or number) is the stable key — re-posting the same id updates the same inbox item. Without an `id`, the body's content hash is the key, so exact duplicate deliveries deduplicate and any changed body is a new item.
- A top-level `"title"` and `"url"` are promoted so the item renders in feeds; everything else stays in the opaque `msg.Payload` for downstream nodes, decoded against the canonical item contract (docs/decisions/0008) wherever it renders.
- A top-level `"kind"` is the item's type label and what actions target with `applies_to`. Omit it and the item is kind `Item` — still automatable (`applies_to: [Item]`), just not distinguishable from other untyped deliveries.
- A top-level `"state"` drives lifecycle: `resolved`, `closed`, and `done` (case-insensitive) system-archive the item with the state as the archive reason; any other or absent state keeps it active. A later delivery whose state leaves one of those terminal values resurfaces the item. A stateless payload behaves exactly as before — manual triage only.

## Rendering and transformation

Feeds render an item from what was ingested. A payload carrying the canonical item contract's fields (`id`, `kind`, `repo`, `title`, `url`, …; docs/decisions/0008) renders like a first-party item; anything else still ingests fine but renders minimally (title + link). To reshape a payload, put a `function` node downstream of this one; the node editor shows the last captured delivery, flags payloads missing the render-critical fields, and offers a copyable LLM prompt for writing that function. A function node must only change `msg.Payload` — `msg.Key` and `msg.Topic` are how feed membership resolves.

## Behavior

The listener binds localhost only and resolves routes live from the current flows, so adding or editing webhook nodes needs no restart. Deliveries ingest immediately (no poll delay).
