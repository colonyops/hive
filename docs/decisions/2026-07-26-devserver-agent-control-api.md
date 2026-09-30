# devserver agent-facing control API

- **Status:** accepted (point 3, `/_ctl/help`, superseded by ADR http-handler-conventions — the
  endpoint was dropped; `/_ctl/state` carries the action vocabulary)
- **Date:** 2026-07-26

## Context

ADR devserver-github-proxy built `cmd/devserver` with a JSON control API under `/_ctl/` and a
dashboard as a thin renderer over it. The surface was shaped for a human
clicking buttons: single mutations (`/_ctl/action`, `/_ctl/overlay`) and
multi-step **scenarios defined in `devserver.yaml`** and triggered by name
(`/_ctl/scenarios/{name}/run`).

Driving lifecycle workflows from an agent exposed two gaps in that shape:

1. **Scenarios were static.** A scenario is a sequence of mutations with waits —
   review requested, pause, merged. Defining one meant editing the checked-in
   config and restarting the singleton proxy that every worktree shares (ADR
   0017). An agent running a scenario for a specific test cannot do either: the
   sequence it needs is known only at test time, and a restart would evict every
   other worktree's overlay state. In practice the shipped scenarios were never
   used — they were placeholders pointed at a specific PR number.

2. **Webhook pushes had no reachable target.** The pusher only delivers to a
   target named in config, but a real target is a running desktop instance whose
   webhook port is drawn at random per install (ADR local-webhook-listener), so no target can be
   committed. There was no way to push an event to an instance from a call.

## Decision

The control API is a first-class agent surface, not only the dashboard's
backend. Three changes:

1. **Scenarios are composed at runtime, not configured.** `POST /_ctl/scenario`
   takes an inline `{steps: [{repo, num, action|set, wait}]}` body, validates it
   up front (a bad step is a `400` the caller reads, not a background failure it
   cannot), and runs it in the background under a generated name (`scenario-1`).
   Each step is either a quick action from the existing vocabulary or a raw
   mutation set, with an optional wait. In-flight scenarios appear in
   `/_ctl/state`, and a finished one disappears — that absence is how a poller
   learns the run is done.

   The `scenarios:` config section, the `Scenario` type, and the
   `/_ctl/scenarios/{name}/run` route are **removed**. This reverses the part of
   ADR devserver-github-proxy that made scenarios a config feature. There is exactly one way to run
   a scenario, and it is the runtime one.

2. **Webhook targets may be given inline.** `POST /_ctl/webhooks/push` accepts
   `target` as either a configured name (a JSON string) or a `{url, secret}`
   object. Inline targets must be loopback, matching the fail-closed posture ADR
   0017 set for the proxy itself: devserver POSTs a secret-bearing body to the
   target, and the desktop's listener binds loopback only, so a non-loopback
   target is either a mistake or an attempt to make dev tooling reach off the
   machine. Inline targets are not remembered.

3. **`GET /_ctl/help` is a self-describing contract.** It returns the endpoints,
   the action vocabulary, and the mutation fields. The action list is derived
   from the same map the server dispatches on, so it cannot drift from what is
   actually accepted. `/_ctl/state` remains the live companion — this is the
   fixed shape, that is the current state.

A repository skill, `.agents/skills/devserver`, teaches an agent to read the
state, pick an observed item, and drive the right lever for a test.

The action vocabulary itself is unchanged and stays GitHub's own (ADR devserver-github-proxy,
point 5): no `approve` action, because the desktop never reads review state.
The new surface exposes the existing levers to an agent; it does not widen them.

## Consequences

- A scenario is no longer reproducible from config — it lives only for the run.
  That is the point: scenarios are per-test, composed by whoever runs them. The
  cost is that there is no committed catalogue of example sequences; the skill
  and `/_ctl/help` document the shape instead.
- Overlay and scenario state stay in-memory and global across worktrees, exactly
  as ADR devserver-github-proxy described. Runtime scenarios do not change that — a restart still
  returns to the config's declared overlays and drops every running scenario.
- An agent can now make devserver POST to any loopback URL. That is a deliberate
  widening bounded to loopback: the pusher already delivered secret-bearing
  bodies to config targets, and inline targets extend that to the only address a
  desktop instance can be reached at.
- The dashboard's scenario panel is now a read-only view of running scenarios;
  it no longer offers buttons to launch configured ones, because there are none.
