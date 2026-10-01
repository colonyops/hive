package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/coder/websocket"
	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
	"github.com/colonyops/hive/cmd/desktop/internal/app/ptyterm"
)

// The ptyterm data plane, shared by every caller that addresses a terminal by
// id (ADR ptyterm-terminals-are-caller-addressed) — today the pop-up, and a workspace session once phase 5 lands.
// One socket carries exactly one terminal — the id is in the query string, not
// in every frame — so output is a byte payload behind a single tag and
// xterm.js writes it without a decode step.
//
//	server -> client
//	  0x00 Output [0x00][raw bytes]
//	  0x01 Exit   [0x01][JSON {reason}]
//	client -> server
//	  0x10 Input  [0x10][raw bytes]
const (
	// PTYStreamPath is the data plane's mount path, under TerminalPathPrefix so
	// the bearer token and CORS policy already guarding the terminal surface
	// cover it (ADR ephemeral-popup-terminals point 4).
	PTYStreamPath = "/api/terminal/pty/stream"

	// ptyWireVersion is this wire's own, deliberately not terminalWireVersion:
	// the two sockets carry different frames and change on their own schedules,
	// and sharing the constant made a tmux-wire change refuse every pop-up
	// handshake. It pins POPUP_WIRE_VERSION in
	// cmd/desktop/frontend/src/lib/popupTerminalClient.ts; move the two together.
	ptyWireVersion = "1"

	popupFrameOutput byte = 0x00
	popupFrameExit   byte = 0x01
	popupFrameInput  byte = 0x10
)

// PTYStreamHandler returns the mount path and the raw handler for the ptyterm
// data plane. Like the tmux stream it is mounted on its own rather than
// joining the operations table: the errchain cannot frame a hijacked socket.
func PTYStreamHandler(core *app.App, token string, origins []string, log zerolog.Logger) (string, http.Handler) {
	return PTYStreamPath, &popupTerminalStream{core: core, token: token, origins: origins, log: log}
}

type popupTerminalStream struct {
	core    *app.App
	token   string
	origins []string
	log     zerolog.Logger
}

type popupExitPayload struct {
	Reason string `json:"reason"`
}

func (h *popupTerminalStream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	if query.Get("v") != ptyWireVersion {
		http.Error(w, "unsupported terminal wire version", http.StatusBadRequest)
		return
	}
	if origin := r.Header.Get("Origin"); origin != "" && !originAllowed(h.origins, origin) {
		h.log.Debug().Str("origin", origin).Strs("allowed", h.origins).Msg("popup terminal stream origin rejected")
		http.Error(w, "origin is not allowed", http.StatusForbidden)
		return
	}

	presented, fromSubprotocol := streamToken(r)
	if h.token == "" || presented != h.token {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	id := query.Get("id")
	events, unsubscribe, err := h.core.PopupTerminals.Subscribe(r.Context(), id)
	if err != nil {
		status := statusForKind(app.KindOf(err))
		http.Error(w, http.StatusText(status), status)
		return
	}

	opts := &websocket.AcceptOptions{
		InsecureSkipVerify: true,
		CompressionMode:    websocket.CompressionDisabled,
	}
	if fromSubprotocol {
		opts.Subprotocols = []string{presented}
	}
	conn, err := websocket.Accept(w, r, opts)
	if err != nil {
		unsubscribe()
		h.log.Debug().Err(err).Str("terminal", id).Msg("popup terminal websocket upgrade failed")
		return
	}
	defer func() { _ = conn.CloseNow() }()
	conn.SetReadLimit(maxInputFrameBytes)

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	writeDone := make(chan struct{})
	go func() {
		defer close(writeDone)
		h.writePump(ctx, conn, events)
	}()

	h.readPump(ctx, conn, id)
	cancel()
	unsubscribe()
	<-writeDone
}

func (h *popupTerminalStream) writePump(ctx context.Context, conn *websocket.Conn, events <-chan ptyterm.Event) {
	for ev := range events {
		frame, ok := encodePopupEvent(ev)
		if !ok {
			continue
		}
		writeCtx, cancel := context.WithTimeout(ctx, streamWriteTimeout)
		err := conn.Write(writeCtx, websocket.MessageBinary, frame)
		cancel()
		if err != nil {
			return
		}
	}
	_ = conn.Close(websocket.StatusNormalClosure, "terminal ended")
}

func (h *popupTerminalStream) readPump(ctx context.Context, conn *websocket.Conn, id string) {
	for {
		kind, data, err := conn.Read(ctx)
		if err != nil {
			return
		}
		if kind != websocket.MessageBinary {
			_ = conn.Close(websocket.StatusUnsupportedData, "binary frames only")
			return
		}
		payload, err := decodePopupInputFrame(data)
		if err != nil {
			_ = conn.Close(websocket.StatusInvalidFramePayloadData, "malformed input frame")
			return
		}
		if err := h.core.PopupTerminals.Write(ctx, id, payload); err != nil {
			h.log.Debug().Err(err).Str("terminal", id).Msg("popup terminal input rejected")
		}
	}
}

func encodePopupEvent(ev ptyterm.Event) ([]byte, bool) {
	switch v := ev.(type) {
	case ptyterm.Output:
		return append([]byte{popupFrameOutput}, v.Data...), true
	case ptyterm.Exited:
		return encodeJSONFrame(popupFrameExit, popupExitPayload{Reason: v.Reason})
	default:
		return nil, false
	}
}

func decodePopupInputFrame(frame []byte) ([]byte, error) {
	if len(frame) == 0 || frame[0] != popupFrameInput {
		return nil, errors.New("not an input frame")
	}
	return frame[1:], nil
}
