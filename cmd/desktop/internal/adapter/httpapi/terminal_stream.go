package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
	"github.com/colonyops/hive/cmd/desktop/internal/app/tmuxcc"
)

// The data-plane wire. Output and Input carry raw bytes behind length-prefixed
// ids so xterm.js writes them without a decode step; control frames carry small
// JSON whose kinds are the stable strings tmuxcc already emits. Client frames
// name a pane, never a window: the keystrokes that follow a click into a pane
// go out while the select-pane it caused is still in flight.
//
//	server -> client
//	  0x00 Output      [0x00][winLen u8][windowId][paneLen u8][paneId][raw bytes]
//	  0x01 WindowEvent [0x01][JSON {kind, windowId, name, active, activePane, width, height, zoomed, layout}]
//	  0x02 Lifecycle   [0x02][JSON {kind, windowId, message}]
//	client -> server
//	  0x10 Input       [0x10][paneLen u8][paneId][raw bytes]
//	  0x11 PasteChunk  [0x11][paneLen u8][paneId][raw bytes]
//	  0x12 PasteCommit [0x12][paneLen u8][paneId]
const (
	frameOutput      byte = 0x00
	frameWindowEvent byte = 0x01
	frameLifecycle   byte = 0x02
	frameInput       byte = 0x10
	framePasteChunk  byte = 0x11
	framePasteCommit byte = 0x12
)

const (
	// terminalWireVersion is carried as ?v= and checked before the upgrade, so a
	// stale webview fails the handshake instead of misparsing frames.
	terminalWireVersion = "2"
	// maxInputFrameBytes caps one client->server frame whole, id included.
	maxInputFrameBytes = 4 << 10
	// maxPasteBytes caps a paste reassembled from its chunk frames. It is far
	// past anything a person pastes into a terminal, and it exists so a client
	// cannot grow this connection's buffer without bound by never committing.
	maxPasteBytes = 1 << 20
	// streamWriteTimeout bounds one frame's write. Without it a client that
	// stops reading pins this connection and the buffer behind it for the life
	// of the process; dropping it instead leaves the tmux session attached and
	// the frontend free to reconnect.
	streamWriteTimeout = 10 * time.Second
)

// TerminalStreamHandler returns the mount prefix and the raw handler for the
// data plane. It is mounted by its own App.MountAPI call rather than joining
// the operations table: the errchain cannot frame a hijacked socket (ADR terminal-transport).
func TerminalStreamHandler(core *app.App, token string, origins []string, log zerolog.Logger) (string, http.Handler) {
	return TerminalStreamPath, &terminalStream{core: core, token: token, origins: origins, log: log}
}

type terminalStream struct {
	core    *app.App
	token   string
	origins []string
	log     zerolog.Logger
}

func (h *terminalStream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	if query.Get("v") != terminalWireVersion {
		http.Error(w, "unsupported terminal wire version", http.StatusBadRequest)
		return
	}
	if origin := r.Header.Get("Origin"); origin != "" && !originAllowed(h.origins, origin) {
		h.log.Debug().Str("origin", origin).Strs("allowed", h.origins).Msg("terminal stream origin rejected")
		http.Error(w, "origin is not allowed", http.StatusForbidden)
		return
	}

	// Browsers cannot set headers on a WebSocket handshake, so the token rides
	// the query string or the subprotocol list, and is checked before upgrading.
	presented, fromSubprotocol := streamToken(r)
	if h.token == "" || presented != h.token {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	slug := query.Get("slug")
	events, unsubscribe, err := h.core.Terminals.Subscribe(r.Context(), slug)
	if err != nil {
		status := statusForKind(app.KindOf(err))
		http.Error(w, http.StatusText(status), status)
		return
	}

	opts := &websocket.AcceptOptions{
		// The origin was matched against the allowlist above; coder's own check
		// is host-based and would reject the wails:// scheme the webview sends.
		InsecureSkipVerify: true,
		CompressionMode:    websocket.CompressionDisabled,
	}
	if fromSubprotocol {
		opts.Subprotocols = []string{presented}
	}
	conn, err := websocket.Accept(w, r, opts)
	if err != nil {
		unsubscribe()
		h.log.Debug().Err(err).Str("session", slug).Msg("terminal websocket upgrade failed")
		return
	}
	defer func() { _ = conn.CloseNow() }()
	conn.SetReadLimit(maxInputFrameBytes)

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	writeDone := make(chan struct{})
	go func() {
		defer close(writeDone)
		h.writePump(ctx, conn, slug, events)
	}()

	h.readPump(ctx, conn, slug)
	cancel()
	unsubscribe()
	<-writeDone
}

// writePump forwards the session's events until the subscription channel
// closes — the manager stopped, the client died, or another socket replaced
// this subscriber — and then closes the socket, which is what unblocks the read
// pump.
func (h *terminalStream) writePump(ctx context.Context, conn *websocket.Conn, slug string, events <-chan tmuxcc.Event) {
	for ev := range events {
		frame, ok := encodeEvent(ev)
		if !ok {
			continue
		}
		writeCtx, cancel := context.WithTimeout(ctx, streamWriteTimeout)
		err := conn.Write(writeCtx, websocket.MessageBinary, frame)
		cancel()
		if err != nil {
			return
		}
		if out, isOutput := ev.(tmuxcc.Output); isOutput {
			h.core.Terminals.ObserveFrameLatency(ctx, time.Since(out.At))
		}
	}
	_ = conn.Close(websocket.StatusNormalClosure, "terminal session ended")
}

// readPump turns input frames into pane writes and paste frames into pane
// pastes. A rejected keystroke — an unknown pane, a tmux command failure — is
// logged and the stream lives on; only a broken frame or a non-binary message
// closes it.
//
// A paste is held until its commit rather than written through, because the
// frame cap bounds one socket message and says nothing about how much text a
// person pastes: the pane has to receive it as one paste or tmux cannot bracket
// it. The pending map is this goroutine's alone — nothing else reads the socket.
func (h *terminalStream) readPump(ctx context.Context, conn *websocket.Conn, slug string) {
	pending := map[string][]byte{}
	for {
		kind, data, err := conn.Read(ctx)
		if err != nil {
			return
		}
		if kind != websocket.MessageBinary {
			_ = conn.Close(websocket.StatusUnsupportedData, "binary frames only")
			return
		}
		frame, err := decodeClientFrame(data)
		if err != nil {
			_ = conn.Close(websocket.StatusInvalidFramePayloadData, "malformed input frame")
			return
		}
		pane := frame.paneID
		switch frame.kind {
		case frameInput:
			if err := h.core.Terminals.Write(ctx, slug, pane, frame.data); err != nil {
				h.log.Debug().Err(err).Str("session", slug).Str("pane", pane).Msg("terminal input rejected")
			}
		case framePasteChunk:
			if len(pending[pane])+len(frame.data) > maxPasteBytes {
				delete(pending, pane)
				h.log.Debug().Str("session", slug).Str("pane", pane).Msg("terminal paste dropped: over the size cap")
				continue
			}
			pending[pane] = append(pending[pane], frame.data...)
		case framePasteCommit:
			text := pending[pane]
			delete(pending, pane)
			if err := h.core.Terminals.Paste(ctx, slug, pane, text); err != nil {
				h.log.Debug().Err(err).Str("session", slug).Str("pane", pane).Msg("terminal paste rejected")
			}
		}
	}
}

// streamToken reads the bearer token from ?token= or, failing that, from the
// subprotocol list. It reports which, because a subprotocol offer has to be
// echoed for the browser to accept the handshake.
func streamToken(r *http.Request) (token string, fromSubprotocol bool) {
	if q := r.URL.Query().Get("token"); q != "" {
		return q, false
	}
	for _, header := range r.Header.Values("Sec-WebSocket-Protocol") {
		for offered := range strings.SplitSeq(header, ",") {
			if offered = strings.TrimSpace(offered); offered != "" {
				return offered, true
			}
		}
	}
	return "", false
}

// windowEventPayload carries a full snapshot so one reconcile event can update
// layout, dimensions, and metadata atomically.
type windowEventPayload struct {
	Kind string `json:"kind"`
	terminalWindow
}

type lifecyclePayload struct {
	Kind     string `json:"kind"`
	WindowID string `json:"windowId"`
	Message  string `json:"message"`
}

func encodeEvent(ev tmuxcc.Event) ([]byte, bool) {
	switch v := ev.(type) {
	case tmuxcc.Output:
		return encodeOutputFrame(v.WindowID, v.PaneID, v.Data), true
	case tmuxcc.WindowChanged:
		return encodeJSONFrame(frameWindowEvent, windowEventPayload{Kind: string(v.Kind), terminalWindow: toTerminalWindow(v.Window)})
	case tmuxcc.LifecycleChanged:
		return encodeJSONFrame(frameLifecycle, lifecyclePayload{
			Kind:     string(v.Kind),
			WindowID: v.WindowID,
			Message:  v.Message,
		})
	default:
		return nil, false
	}
}

func encodeOutputFrame(windowID, paneID string, data []byte) []byte {
	frame := make([]byte, 0, 3+len(windowID)+len(paneID)+len(data))
	frame = append(frame, frameOutput)
	frame = appendID(frame, windowID)
	frame = appendID(frame, paneID)
	return append(frame, data...)
}

func encodeInputFrame(paneID string, data []byte) []byte {
	frame := make([]byte, 0, 2+len(paneID)+len(data))
	frame = append(frame, frameInput)
	frame = appendID(frame, paneID)
	return append(frame, data...)
}

func encodeJSONFrame(kind byte, payload any) ([]byte, bool) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, false
	}
	return append([]byte{kind}, body...), true
}

// appendID writes a length-prefixed ascii id. tmux ids are "@12"/"%34", so the
// uint8 length is never in danger of truncating one.
func appendID(dst []byte, id string) []byte {
	if len(id) > 0xff {
		id = id[:0xff]
	}
	return append(append(dst, byte(len(id))), id...)
}

func decodeOutputFrame(frame []byte) (windowID, paneID string, data []byte, err error) {
	if len(frame) == 0 || frame[0] != frameOutput {
		return "", "", nil, errors.New("not an output frame")
	}
	windowID, rest, err := readID(frame[1:])
	if err != nil {
		return "", "", nil, err
	}
	paneID, data, err = readID(rest)
	if err != nil {
		return "", "", nil, err
	}
	return windowID, paneID, data, nil
}

type clientFrame struct {
	kind   byte
	paneID string
	data   []byte
}

// decodeClientFrame reads one client -> server frame. An unknown kind is
// malformed rather than ignored, so a client speaking a wire this server does
// not is told rather than silently half-heard.
func decodeClientFrame(frame []byte) (clientFrame, error) {
	if len(frame) == 0 {
		return clientFrame{}, errors.New("empty client frame")
	}
	switch frame[0] {
	case frameInput, framePasteChunk, framePasteCommit:
	default:
		return clientFrame{}, fmt.Errorf("unknown client frame kind 0x%02x", frame[0])
	}
	paneID, data, err := readID(frame[1:])
	if err != nil {
		return clientFrame{}, err
	}
	if paneID == "" {
		return clientFrame{}, errors.New("client frame carries no pane id")
	}
	return clientFrame{kind: frame[0], paneID: paneID, data: data}, nil
}

func readID(src []byte) (id string, rest []byte, err error) {
	if len(src) == 0 {
		return "", nil, errors.New("frame ended before its id length")
	}
	size := int(src[0])
	if len(src) < 1+size {
		return "", nil, fmt.Errorf("frame declares a %d byte id but carries %d", size, len(src)-1)
	}
	return string(src[1 : 1+size]), src[1+size:], nil
}
