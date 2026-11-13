package httpapi

import (
	"context"
	"net/http"
	"strings"

	"github.com/coder/websocket"
	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
)

// PaneStreamPath carries one tmux pane to a renderer outside the webview: a
// native terminal surface's bridge process. Binary messages both ways are raw
// terminal bytes: the pane's output out, typed input in. While the socket is
// open the pane's output leaves the session stream (app.PaneTap).
const PaneStreamPath = "/api/terminal/pane-stream"

// paneRepaint asks for a fresh snapshot: a bridge sends it as a text message
// once its surface has resized.
const paneRepaint = "repaint"

// paneClear empties the renderer's screen and scrollback before a snapshot.
// It is not a full reset: modes the program set, such as bracketed paste,
// are not in the snapshot and must survive it.
const paneClear = "\x1b[0m\x1b[r\x1b[?6l\x1b[H\x1b[2J\x1b[3J"

// PaneStreamHandler returns the mount prefix and handler of the per-pane
// stream. The bridge is a local process, not a page: it sends the token as a
// bearer header and no Origin.
func PaneStreamHandler(core *app.App, token string, log zerolog.Logger) (string, http.Handler) {
	return PaneStreamPath, &paneStream{core: core, token: token, log: log}
}

type paneStream struct {
	core  *app.App
	token string
	log   zerolog.Logger
}

func (h *paneStream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Origin") != "" {
		http.Error(w, "the pane stream is not for pages", http.StatusForbidden)
		return
	}
	presented, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if h.token == "" || presented != h.token {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	slug, pane := r.URL.Query().Get("slug"), r.URL.Query().Get("pane")
	tap, err := h.core.Terminals.TapPane(r.Context(), slug, pane)
	if err != nil {
		status := statusForKind(app.KindOf(err))
		http.Error(w, err.Error(), status)
		return
	}
	defer tap.Close()

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{CompressionMode: websocket.CompressionDisabled})
	if err != nil {
		h.log.Debug().Err(err).Str("session", slug).Str("pane", pane).Msg("pane stream upgrade failed")
		return
	}
	defer func() { _ = conn.CloseNow() }()
	conn.SetReadLimit(maxPasteBytes)

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	repaint := make(chan struct{}, 1)
	go h.readInput(ctx, cancel, conn, slug, pane, repaint)
	h.writeOutput(ctx, conn, tap, slug, pane, repaint)
}

// writeOutput is the only writer, so a resync never interleaves with output.
func (h *paneStream) writeOutput(ctx context.Context, conn *websocket.Conn, tap *app.PaneTap, slug, pane string, repaint <-chan struct{}) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-tap.Done():
			_ = conn.Close(websocket.StatusGoingAway, "replaced")
			return
		case <-repaint:
			// The surface resized after tmux did: what it drew is reflowed
			// text the program no longer agrees with, so it starts over from
			// tmux's screen.
			h.resync(ctx, conn, tap, slug, pane)
		case p := <-tap.C:
			if tap.Overflowed() {
				h.resync(ctx, conn, tap, slug, pane)
				continue
			}
			if err := h.write(ctx, conn, p); err != nil {
				return
			}
		}
	}
}

// resync drops what the tap queued and starts the renderer over from a fresh
// snapshot, which carries the screen and scrollback the dropped bytes drew.
// Output queued before the repaint was for the screen the snapshot replaces.
func (h *paneStream) resync(ctx context.Context, conn *websocket.Conn, tap *app.PaneTap, slug, pane string) {
	for len(tap.C) > 0 {
		<-tap.C
	}
	if err := h.write(ctx, conn, []byte(paneClear)); err != nil {
		return
	}
	if err := h.core.Terminals.RepaintPane(ctx, slug, pane); err != nil {
		h.log.Warn().Err(err).Str("session", slug).Str("pane", pane).Msg("repainting a native pane")
	}
}

func (h *paneStream) write(ctx context.Context, conn *websocket.Conn, p []byte) error {
	writeCtx, cancel := context.WithTimeout(ctx, streamWriteTimeout)
	defer cancel()
	return conn.Write(writeCtx, websocket.MessageBinary, p)
}

func (h *paneStream) readInput(ctx context.Context, cancel context.CancelFunc, conn *websocket.Conn, slug, pane string, repaint chan<- struct{}) {
	defer cancel()
	for {
		kind, p, err := conn.Read(ctx)
		if err != nil {
			return
		}
		if kind == websocket.MessageText {
			if string(p) == paneRepaint {
				select {
				case repaint <- struct{}{}:
				default:
				}
			}
			continue
		}
		if len(p) == 0 {
			continue
		}
		if err := h.core.Terminals.Write(ctx, slug, pane, p); err != nil {
			h.log.Debug().Err(err).Str("session", slug).Str("pane", pane).Msg("pane input not delivered")
		}
	}
}
