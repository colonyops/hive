# Terminal transport: REST control plane on httpapi, one binary WebSocket per session

- **Status:** accepted
- **Date:** 2026-07-28

## Context

Terminal mode attaches a tmux control-mode client (`tmux -C attach -t <slug>`)
per Hive session and renders its windows as tabs. That is two traffic shapes at
once: infrequent request/response operations (attach, detach, resize, window
new/close/rename/select) and a continuous bidirectional byte stream — pane
output out, keystrokes in — whose per-byte cost and latency are visible to the
person typing. The Wails bridge serialises event payloads as JSON, so raw
terminal bytes would have to be base64'd on every frame, and the frontend has no
way to signal backpressure.

The app already binds one loopback HTTP server (ADR agent-http-api) hosting the webhook
listener and the agent API, mounted through `App.MountAPI`. Everything on it is
deliberately unauthenticated: the loopback bind is the boundary.

## Decision

1. **Control plane is REST on the existing `httpapi` adapter; data plane is one
   binary WebSocket per attached session.** The operations are `POST
   /api/terminal/…` keyed by session slug, written as ordinary errchain handlers
   (ADR http-handler-conventions) in the operations table (ADR self-describing-agent-api), so they inherit input
   extraction, `Kind` → status mapping, the route index and the OpenAPI
   document. The stream is `GET /api/terminal/stream?slug=<slug>&v=1`, mounted
   as a raw `http.Handler` at its own prefix by a second `App.MountAPI` call —
   ServeMux longest-match keeps it out of the JSON errchain, which cannot frame
   a hijacked socket. Frames are `type byte + payload`: output and input carry
   **raw bytes** behind length-prefixed window/pane ids, control frames carry
   small JSON with stable string kinds. No base64, and no sequence numbers,
   because WebSocket delivery is ordered. No new server process and no second
   port.

2. **The terminal endpoints require a bearer token; their siblings deliberately
   do not.** A terminal is arbitrary command execution as the user; the sibling
   routes mutate app state (flows, profiles, triage) but none of them execs, so
   the loopback-trust model of ADR agent-http-api stops here rather than being revised
   everywhere. The token is minted per run in the composition root
   (`desktop/main.go`, crypto/rand, base64url) and handed to `httpapi.New` and
   the wailsui `TerminalService`. The core holds none of it: `app.TerminalsService`
   has no token, base URL or stream path, and the endpoint DTO
   (`httpBaseURL`, `wsURL`) is composed in `wailsui` from the live loopback bind.
   Browsers cannot set headers on a WebSocket handshake, so the stream accepts
   the token in `?token=` or `Sec-WebSocket-Protocol` and rejects before
   upgrading. The CORS allowlist (`wails://localhost` plus the dev Vite origin)
   is composed in `main.go` for the same reason the token is; auth is a token,
   not a cookie, so the allowlist need not be credentialed. The token is never
   logged.

3. **Availability follows the loopback server.** `http.enabled` false means no
   listener, `MountAPI` returning false, and no terminal transport. Terminal
   mode then reports unavailable with a reason through the Wails
   `TerminalService.Available` — the same channel that reports a missing tmux,
   tmux `< 3.2`, and the `-tags server` build. No fallback transport is built
   for that configuration.

4. **Flow control is a byte-bounded fan-out buffer whose overflow is fatal;
   there is no tmux pause mode in v1.** The reader goroutine must always drain
   tmux's stdout — command `%end`/`%error` replies share that pipe with
   notifications, so a blocked dispatch deadlocks a pending command. Because we
   always drain, tmux's own send-buffer age never grows and `refresh-client -f
   pause-after` could not protect our broker anyway. Instead the per-session
   broker buffer is bounded by bytes and overflow kills the client, drops the
   slug, emits an `EXITED(overflow)` lifecycle frame and closes the socket; the
   frontend's reconnect affordance re-attaches, and a fresh attach re-runs first
   paint, which is the resync. A terminal byte stream cannot drop-oldest without
   corrupting emulator state, and a partial resync would need the scrollback and
   cursor restore v1 does not have. `%pause`/`%continue` are still parsed so an
   externally enabled pause mode cannot break framing; we never enable it.

   **Superseded in part by [ADR terminal-overflow-resyncs-the-view-instead-of-ending-the-stream](2026-08-05-terminal-overflow-resyncs-the-view-instead-of-ending-the-stream.md):**
   crossing the bound now drops the backlog, marks the stream `degraded` and
   repaints in place; the teardown is what a resync that will not hold falls
   back to. The bound, the always-drain rule and the absence of `pause-after`
   are unchanged, and so is the `EXITED(overflow)` frame on the fatal path.

5. **First paint may duplicate a few bytes, and that is accepted.** Attach
   captures each window's visible screen (`capture-pane -pe -J`) and replays
   live `%output` buffered from attach start once that window's capture reply
   resolves, discarding what arrived before the capture command was *sent*.
   Output landing between tmux's snapshot and its reply can therefore appear
   twice. Closing the window would need a marker protocol tmux does not offer;
   duplicated bytes at attach are preferable to a missing region.

   **Superseded in part by [ADR terminal-first-paint-carries-scrollback](2026-07-30-terminal-first-paint-carries-scrollback.md):**
   what a snapshot *contains* is now bounded scrollback, a full-height screen
   and a cursor restore. The accepted duplication and the buffer-and-replay
   ordering are unchanged.

## Consequences

- The app has a **second driving transport with a wire format of its own**.
  Wails stays the shell — mode toggle, session picker, availability and endpoint
  bootstrap — and nothing on the terminal data path crosses the Wails bridge.
  The adapter rules are unchanged: both adapters call `app.TerminalsService`,
  which returns `*app.Error` values, and each maps `Kind` exactly once.
- **The framing is hand-written on both sides.** There is no codegen and no
  drift gate, so a wire change is two edits. The URL carries `?v=1` and the
  server rejects other versions before upgrading, so a stale webview against a
  new binary fails at the handshake instead of misparsing frames.
- **The `-tags server` build stubs the subsystem to unavailable.** The headless
  and e2e lanes compile with no tmux dependency and assert the unavailable
  state; live attach round-trips are proven by Go integration tests against real
  tmux, which CI installs.
- **A local process holding the token gets shell execution.** The token is
  per-run, in memory only, never logged and never persisted, and the loopback
  bind remains the outer boundary. Any future surface that hands the token out
  is a new decision.
- Deferred without changing the transport: pane splits (v1 renders each window's
  active pane, and the output frame already carries the pane id) and any
  pause-mode based partial resync. Full history, alternate-screen handling and
  cursor restore on attach were the other three, and ADR terminal-first-paint-carries-scrollback built them.
  ADR pane-splits-render-one-emulator-per-tmux-pane-over-the-window-s-layout built the splits, and moved the wire to v2:
  client frames name a pane rather than a window.
