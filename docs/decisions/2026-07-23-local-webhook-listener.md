# Local webhook listener for generic pipeline ingress

- **Status:** accepted
- **Date:** 2026-07-23

## Context

Systems without a first-party source (CI, monitoring, home-grown tooling, Zapier/n8n bridges) need a way to
push arbitrary JSON into flows. Webhooks are push-shaped and fit the poll producer badly: a buffer drained
per tick adds up to a poll interval of latency and distorts the producer's authoritative-snapshot-per-tick
contract.

## Decision

A `net/http` listener (`cmd/desktop/internal/app/sources/webhook.Listener`) bound to loopback serves
`POST /hooks/<path>` for every enabled `sources.webhook` flow node.

1. **Routes resolve per request** from the current push instances, so node edits need no restart and the
   listener holds no route state. Several nodes may declare the same path; each receives the delivery.
2. **Deliveries bypass the producer** and call `IngestObservation` directly under the node's topic
   (`source:<flowId>/<nodeId>`), `source_kind: "webhook"`, `source_scope: <nodeId>`. After a write the
   listener appends the topic's full unarchived item set as an authoritative snapshot, so membership replay
   treats webhook sources like polled ones.
3. **Identity is payload-derived.** A top-level `id` (string or number) is the stable key; otherwise the
   body's SHA-256, so exact duplicates dedupe. `title` and `url` are promoted for feed rendering; the
   payload is otherwise opaque, and reshaping is downstream `function`-node work.
4. **Configuration** lives in the `http` section of `settings.yaml` (ADR desktop-configuration): enabled
   by default, loopback host, port `0` by default. On the first successful bind with port `0` the app
   persists the OS-assigned port, so endpoint URLs pasted into senders stay valid across restarts. Mock
   modes start the listener only with an explicit `HIVE_DESKTOP_HTTP_PORT`, so parallel e2e lanes never
   collide. Changes take effect on restart.
5. **Auth is an optional per-node shared secret** (`X-Hive-Secret`, constant-time compare), optional
   because the bind is loopback-only.
6. **The last request body per topic** is kept in the `webhook_capture` table to power the node editor's
   payload preview and feed-shape hint.

## Consequences

- A bind failure is logged and kept for the settings pane to show; the app runs on.
- There is no documentable fixed webhook port. Instructions read it from settings or the node editor.
- Non-loopback exposure is out of scope; users front it themselves (tailscale, ssh, reverse proxy).
- One delivery costs one snapshot append over the topic's unarchived items. Revisit if a topic accumulates
  very large active sets.
- Feed rendering comes from what was ingested, not from downstream `function` transforms. Senders that want
  rich rows send feed-item-shaped JSON (ADR canonical-item-contract).
