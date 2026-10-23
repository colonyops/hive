# devserver agent-facing control API

- **Status:** accepted
- **Date:** 2026-07-26

## Context

The devserver (ADR devserver-github-proxy) is a singleton shared by every worktree, and agents need to
drive lifecycle workflows through it. A scenario an agent needs is known only at test time, so it cannot
be checked-in config that requires a restart, and a restart would evict every other worktree's overlays.
Webhook targets cannot be committed either, because a desktop instance's webhook port is chosen per
install (ADR local-webhook-listener).

## Decision

The `/_ctl/` control API is a first-class agent surface, not only the dashboard's backend.

1. **Scenarios are composed at runtime, never configured.** `POST /_ctl/scenario` takes an inline
   `{steps: [{repo, num, action|set, wait}]}` body, validates every step up front (a bad step is an error
   response, not a background failure), and runs it in the background under a generated name
   (`scenario-1`). Running scenarios appear in `/_ctl/state`; a finished one disappears, which is how a
   poller learns it is done. There is no config section for scenarios.
2. **Webhook targets may be given inline.** `POST /_ctl/webhooks/push` accepts `target` as a configured
   name (a JSON string) or a `{url, secret}` object. Inline URLs must be loopback: devserver POSTs a
   secret-bearing body there, and the desktop listener binds loopback only. Inline targets are not
   remembered.
3. **`/_ctl/state` is the discovery surface**, including the `actions` list derived from the map the server
   dispatches on.

The `desktop-devserver` skill (`.agents/skills/desktop-devserver`) teaches an agent to read the state, pick
an observed item, and drive a lever. The action vocabulary stays GitHub's own (no `approve`).

## Consequences

- A scenario lives only for its run; there is no committed catalogue of sequences.
- Overlay and scenario state are in-memory and global across worktrees. A restart drops every running
  scenario.
- An agent can make devserver POST to any loopback URL, a deliberate widening bounded to loopback.
- The dashboard's scenario panel is a read-only view of running scenarios.
