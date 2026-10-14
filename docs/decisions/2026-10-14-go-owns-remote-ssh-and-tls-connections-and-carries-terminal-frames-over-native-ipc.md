# Go owns remote SSH and TLS connections and carries terminal frames over native IPC

- **Status:** accepted
- **Date:** 2026-10-14

## Context

Remote credentials must stay outside the webview. Remote traffic needs TLS, including loopback hops. Native webviews cannot pin a private certificate through browser HTTP or WebSocket APIs. The local HTTP listener also serves agent MCP clients, which do not share the remote certificate trust.

## Decision

Go owns remote connections. OpenSSH reads a private connection file through an existing trusted host configuration. That file supplies the current bearer token and public certificate. Go verifies HTTPS/WSS with that certificate as its trust root and checks the endpoint hostname. OpenSSH stdio forwarding carries TLS bytes without a local TCP listener.

A dedicated, loopback-only TLS listener serves the remote terminal API. App owns its lifecycle. Its certificate and key persist in the state directory; its private connection file changes on each server start. The existing local MCP listener remains separate.

Native Wails calls carry remote requests and terminal frame batches. Frames use base64 inside bounded request/response batches, not application events. Input calls remain ordered. Disconnect cancels requests and streams and closes SSH processes. A rejected token causes one SSH refresh and retry; transport errors do not replay mutations.

## Consequences

Packaged native operation has no webview-to-remote TCP hop or remote token in JavaScript. The development and headless browser harness still uses its ordinary Wails HTTP origin and does not provide this native IPC property.

Existing local terminal connections retain their HTTP/WebSocket implementation behind the same frontend stream interface. Remote IPC adds encoding overhead. The Go output queue and frontend input queue are bounded; tests cover ordering and teardown. Hosts need a trusted SSH host key and noninteractive credentials. Certificates expire after one year; operators must renew the identity and reconnect. There is no TLS verification bypass or fallback to remote HTTP.
