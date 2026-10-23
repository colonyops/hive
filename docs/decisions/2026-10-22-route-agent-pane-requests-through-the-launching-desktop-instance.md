# Route agent pane requests through the launching desktop instance

- **Status:** Accepted
- **Date:** 2026-10-22

## Context

Installed and development apps can share Drive files while serving different
MCP addresses. A successful write therefore cannot prove that a pane request
reached the app displaying the agent.

## Decision

Pass the launching app's current MCP addresses as process arguments for
supported Code agents. Keep these overrides out of global configuration and
persisted session metadata.

Correlate each pane request with an acknowledgment from the app-level UI
listener. Return the UI outcome and responding instance URL, with a bounded
wait when the UI is unavailable. Never broadcast pane requests between apps.
