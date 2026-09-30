# Local webhook listener for generic pipeline ingress

- **Status:** superseded by [desktop-configuration](2026-07-25-desktop-configuration.md)
- **Date:** 2026-07-23

ADR desktop-configuration replaces the listener's configuration, default, and port-allocation
decisions. The ingress and routing decisions below remain as historical context.

## Context

The pipeline's only source is `github-source`, polled by the Go producer. A generic ingress was needed so systems without a first-party source (CI, monitoring, home-grown tooling, Zapier/n8n bridges) can push arbitrary JSON into flows. The desktop app had no general-purpose HTTP listener — the only HTTP surface is Wails' own asset server plus marker-gated `/_e2e/*` test middleware — and webhooks are push-shaped, which fits the poll producer badly (a buffer drained per tick would add up to a full poll interval of latency and distort the "authoritative snapshot per tick" contract).

## Decision

A dedicated `net/http` server (`pipeline.WebhookListener`) bound to **127.0.0.1 only**, serving `POST /hooks/<path>` for every enabled `webhook-source` flow node:

1. **Routes resolve per request from the current flow set** (the same late-binding posture as the producer's per-tick `SourceLister`), so node edits need no restart and the listener holds no route state. Several nodes may declare the same path; each receives the delivery.
2. **Deliveries bypass the producer** and call `IngestObservation` directly — the same production source boundary the smoke fixture uses — under topic `source:<flowId>/<nodeId>`, `source_kind: "webhook"`, `source_scope: <nodeId>`. After a write the listener appends the topic's complete unarchived item set as the authoritative snapshot, so startup/deploy membership replay treats webhook sources exactly like polled ones.
3. **Identity is payload-derived:** a top-level `id` (string/number) is the stable key; otherwise the body's SHA-256, so exact duplicate deliveries dedupe. `title`/`url` are promoted for feed rendering; the payload is otherwise opaque per the `Msg` contract — reshaping stays downstream `function`-node work.
4. **Port** comes from `settings.yaml`'s `webhook_port`, overridable by `HIVE_DESKTOP_WEBHOOK_PORT`. There is no shipped default port: on first run one is drawn at random from 20000–32767 and persisted, so two Hive installs do not collide by construction and the endpoint URLs users paste into sending systems stay valid. That window sits above the crowded registered ports and below both Linux's (32768) and macOS's (49152) ephemeral floors, so a persisted port is never handed to an outbound connection while Hive is closed; a short reserved list skips registered services inside it, and candidates are bind-tested before being persisted. In mock modes the listener starts only with an explicit env port so parallel e2e servers never collide. Non-localhost exposure is out of scope; users who need remote senders can front it themselves (tailscale/ssh/reverse proxy).
5. **Enable/disable is a startup-time setting** (`webhook_enabled`, default on). The listener binds a port and serves flow-declared routes, so a live toggle would mean tearing down in-flight deliveries for no benefit; the settings pane reports the pending restart instead. Both controls live on the Integrations page next to GitHub, since a webhook endpoint is a data source in the same sense.
6. **Auth is a per-node shared secret** (`X-Hive-Secret`, constant-time compare) — optional, since the bind is loopback-only.
7. The last request body per topic is kept in a `webhook_capture` table (one row per topic) to power the node editor's payload preview, its non-blocking feed-shape hint, and the copyable LLM transform prompt.

## Consequences

- The desktop now opens a localhost TCP port in live mode unless `webhook_enabled` is false. A bind failure logs, is retained for the settings pane to surface, and the app runs on; the port is never fatal.
- A per-install random port means no documentable "the Hive webhook port": every instruction has to read the port out of settings or the node editor's endpoint row. Both already render the live URL, so this costs documentation clarity rather than usability.
- Webhook items ingest with zero poll latency, and one delivery costs one snapshot append over the topic's unarchived items — acceptable at webhook volumes, revisit if a topic accumulates very large active sets.
- Feed rendering of an item comes from what was ingested (title/url promotion plus payload fields), not from downstream `function` transforms — transforms affect routing, filtering, and action payloads. Senders that want rich rows send feed-item-shaped JSON.
- A future e2e lane can exercise the listener by claiming a port through `HIVE_DESKTOP_WEBHOOK_PORT`.
