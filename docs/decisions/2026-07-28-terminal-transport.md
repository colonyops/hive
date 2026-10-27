# Terminal transport: REST control plane on httpapi, one binary WebSocket per session

- **Status:** accepted
- **Date:** 2026-07-28

## Context

Terminal mode attaches a tmux control-mode client (`tmux -C attach -t <slug>`)
per Hive session and renders its windows as tabs. That is two traffic shapes:
infrequent request/response operations (attach, start, kill, resize, window
operations) and a continuous bidirectional byte stream whose per-byte cost
and latency the person typing feels. The Wails bridge serializes payloads as
JSON, so raw bytes would be base64'd on every frame, and it has no
backpressure.

## Decision

1. **Control plane is REST on the `httpapi` adapter; data plane is one binary
   WebSocket per attached session**, both on the shared loopback server (ADR
   agent-http-api). The operations are `POST /api/terminal/...` keyed by
   session slug, written as errchain handlers (ADR http-handler-conventions)
   in the operations table. The stream is `GET /api/terminal/stream`, mounted
   as a raw `http.Handler` by its own `App.MountAPI` call, because the JSON
   errchain cannot frame a hijacked socket. Frames are `type byte + payload`:
   output and input carry raw bytes behind length-prefixed ids, control frames
   carry small JSON with stable string kinds. No base64 and no sequence
   numbers (WebSocket delivery is ordered).

2. **The terminal endpoints require a bearer token.** A terminal is command
   execution as the user, so loopback trust stops here. `cmd/desktop/main.go`
   mints the token per run (`httpapi.MintTerminalToken`, crypto/rand,
   base64url) and hands it to `httpapi` and the wailsui `TerminalService`. The
   core holds no token, base URL or stream path; the endpoint
   (`httpBaseURL`, `wsURL`) is composed in `wailsui` from the live bind.
   Browsers cannot set headers on a WebSocket handshake, so the stream accepts
   the token in `?token=` or `Sec-WebSocket-Protocol` and rejects before
   upgrading. The CORS allowlist (`wails://localhost` plus the dev Vite
   origin) is composed in `main.go`. The token is never logged or persisted.

3. **Availability follows the loopback server.** With `http.enabled` false
   there is no transport, and `TerminalService.Available` reports unavailable
   with a reason, the same channel that reports a missing tmux or tmux older
   than 3.2. No fallback transport exists.

4. **The reader always drains tmux's stdout and never enables pause mode.**
   Command `%end`/`%error` replies share that pipe with notifications, so a
   blocked reader deadlocks a pending command. The per-session broker buffer
   is bounded by bytes instead; what happens on overflow is ADR
   terminal-overflow-resyncs-the-view-instead-of-ending-the-stream.
   `%pause`/`%continue` are still parsed so an externally enabled pause mode
   cannot break framing.

5. **First paint may duplicate a few bytes.** Attach captures each pane's
   screen and replays live `%output` buffered from attach start once the
   capture reply resolves, discarding what arrived before the capture was
   sent. Output landing between tmux's snapshot and its reply can appear
   twice; tmux offers no marker to close that window, and duplicated bytes
   beat a missing region. What a snapshot contains is ADR
   terminal-first-paint-carries-scrollback.

## Consequences

- Nothing on the terminal data path crosses the Wails bridge. Wails carries
  the shell: mode toggle, session picker, availability and endpoint bootstrap.
- The framing is hand-written on both sides, with no codegen and no drift
  gate. The URL carries `?v=<wire version>` (currently 2, since ADR
  pane-splits-render-one-emulator-per-tmux-pane-over-the-window-s-layout made
  client frames name a pane) and the server rejects other versions before
  upgrading, so a stale webview fails at the handshake instead of misparsing.
- A local process holding the token gets shell execution. Any surface that
  hands the token out is a new decision.
