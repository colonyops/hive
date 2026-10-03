package httpapi

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"

	"github.com/hay-kot/criterio"
	"github.com/hay-kot/httpkit/server"
	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
	"github.com/colonyops/hive/internal/platform/tmux/control"
	"github.com/colonyops/hive/internal/web/extractors"
)

const (
	// TerminalPathPrefix is the control plane's path space. Everything under it
	// authenticates and answers CORS; nothing else on this API does.
	TerminalPathPrefix = "/api/terminal/"
	// TerminalStreamPath is where the data-plane WebSocket is mounted. It sits
	// under the prefix above but is not an operation: ServeMux's longest match
	// routes it to its own raw handler instead of through the errchain table.
	TerminalStreamPath = "/api/terminal/stream"

	terminalTokenBytes = 32
	maxTerminalName    = 200
)

// MintTerminalToken returns a fresh per-run bearer token for the terminal
// surface. It is minted in the composition root, held in memory, never logged
// and never persisted (ADR terminal-transport).
func MintTerminalToken() (string, error) {
	raw := make([]byte, terminalTokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("mint terminal token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// terminalWindow carries tmux's size for the window, which is the size the
// frontend must render it at — the cols/rows it asked for are only a vote —
// and its pane layout, which is where each pane's emulator goes inside it.
type terminalWindow struct {
	WindowID   string `json:"windowId"`
	Name       string `json:"name"`
	Active     bool   `json:"active"`
	ActivePane string `json:"activePane"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	// Zoomed says the active pane is drawn over the whole window; Layout still
	// records where it goes back to.
	Zoomed bool            `json:"zoomed"`
	Layout *terminalLayout `json:"layout,omitempty"`
}

type terminalLayout struct {
	PaneID string `json:"paneId,omitempty"`
	// Split is "leftright" for cells side by side (tmux's split-window -h) or
	// "topbottom" for stacked cells (-v); absent on a leaf.
	Split  string           `json:"split,omitempty"`
	X      int              `json:"x"`
	Y      int              `json:"y"`
	Width  int              `json:"width"`
	Height int              `json:"height"`
	Cells  []terminalLayout `json:"cells,omitempty"`
}

func toTerminalLayout(layout tmuxcc.Layout) terminalLayout {
	out := terminalLayout{
		PaneID: layout.Pane,
		Split:  string(layout.Split),
		X:      layout.X,
		Y:      layout.Y,
		Width:  layout.Width,
		Height: layout.Height,
	}
	for _, cell := range layout.Cells {
		out.Cells = append(out.Cells, toTerminalLayout(cell))
	}
	return out
}

type terminalSlugRequest struct {
	Slug string `json:"slug"`
}

type terminalAgentWindowRequest struct {
	Slug  string `json:"slug"`
	Agent string `json:"agent"`
}

func (b terminalAgentWindowRequest) Validate() error {
	return criterio.ValidateStruct(
		criterio.Run("slug", b.Slug, criterio.Required),
		criterio.Run("agent", b.Agent, criterio.Required),
	)
}

// terminalSlugsRequest names the sessions a sweep wants windows for. No
// Validate: an empty set is a sidebar with nothing in it, which is an empty
// answer rather than a bad request.
type terminalSlugsRequest struct {
	Slugs []string `json:"slugs"`
}

func (b terminalSlugRequest) Validate() error {
	return criterio.Run("slug", b.Slug, criterio.Required)
}

type terminalSizeRequest struct {
	Slug string `json:"slug"`
	Cols int    `json:"cols"`
	Rows int    `json:"rows"`
}

func (b terminalSizeRequest) Validate() error {
	return criterio.ValidateStruct(
		criterio.Run("slug", b.Slug, criterio.Required),
		criterio.Run("cols", b.Cols, criterio.Positive[int]()),
		criterio.Run("rows", b.Rows, criterio.Positive[int]()),
	)
}

// terminalAttachRequest is a size request that also accepts 0x0: a webview that
// has not measured its pane yet attaches unsized rather than voting a
// placeholder tmux would obey, which would resize the session — and every other
// client attached to it — to a size nothing asked for.
type terminalAttachRequest terminalSizeRequest

func (b terminalAttachRequest) Validate() error {
	if b.Cols == 0 && b.Rows == 0 {
		return criterio.Run("slug", b.Slug, criterio.Required)
	}
	return terminalSizeRequest(b).Validate()
}

type terminalWindowRequest struct {
	Slug     string `json:"slug"`
	WindowID string `json:"windowId"`
}

func (b terminalWindowRequest) Validate() error {
	return criterio.ValidateStruct(
		criterio.Run("slug", b.Slug, criterio.Required),
		criterio.Run("windowId", b.WindowID, criterio.Required),
	)
}

// terminalMoveRequest carries a destination index rather than a neighbour: a
// tab strip means "this window ends up here", and which tmux insertion expresses
// that is the core's to work out against the order it can see.
type terminalMoveRequest struct {
	Slug     string `json:"slug"`
	WindowID string `json:"windowId"`
	Position int    `json:"position"`
}

func (b terminalMoveRequest) Validate() error {
	return criterio.ValidateStruct(
		criterio.Run("slug", b.Slug, criterio.Required),
		criterio.Run("windowId", b.WindowID, criterio.Required),
		criterio.Run("position", b.Position, criterio.Min(0)),
	)
}

type terminalRenameRequest struct {
	Slug     string `json:"slug"`
	WindowID string `json:"windowId"`
	Name     string `json:"name"`
}

func (b terminalRenameRequest) Validate() error {
	return criterio.ValidateStruct(
		criterio.Run("slug", b.Slug, criterio.Required),
		criterio.Run("windowId", b.WindowID, criterio.Required),
		criterio.Run("name", b.Name, criterio.Required, criterio.StrMax(maxTerminalName)),
	)
}

type terminalPaneRequest struct {
	Slug   string `json:"slug"`
	PaneID string `json:"paneId"`
}

func (b terminalPaneRequest) Validate() error {
	return criterio.ValidateStruct(
		criterio.Run("slug", b.Slug, criterio.Required),
		criterio.Run("paneId", b.PaneID, criterio.Required),
	)
}

// terminalSplitRequest is a pane and which way to split it, in tmux's words:
// horizontal puts the new pane to the right, vertical below.
type terminalSplitRequest struct {
	Slug      string `json:"slug"`
	PaneID    string `json:"paneId"`
	Direction string `json:"direction"`
}

func (b terminalSplitRequest) Validate() error {
	return criterio.ValidateStruct(
		criterio.Run("slug", b.Slug, criterio.Required),
		criterio.Run("paneId", b.PaneID, criterio.Required),
		criterio.Run("direction", b.Direction, criterio.Required, criterio.OneOf(string(tmuxcc.SplitHorizontal), string(tmuxcc.SplitVertical))),
	)
}

// terminalSelectPaneRequest is a pane and, optionally, a direction: with one,
// the pane's neighbour that way is selected rather than the pane itself.
type terminalSelectPaneRequest struct {
	Slug      string `json:"slug"`
	PaneID    string `json:"paneId"`
	Direction string `json:"direction"`
}

func (b terminalSelectPaneRequest) Validate() error {
	return criterio.ValidateStruct(
		criterio.Run("slug", b.Slug, criterio.Required),
		criterio.Run("paneId", b.PaneID, criterio.Required),
		criterio.Run("direction", b.Direction, criterio.OneOf(
			string(tmuxcc.PaneSelf), string(tmuxcc.PaneLeft), string(tmuxcc.PaneRight), string(tmuxcc.PaneUp), string(tmuxcc.PaneDown))),
	)
}

// Width and Height are validated by tmuxcc, which permits one zero axis,
// rejects both zero, and enforces the 1..1000 bound.
type terminalResizePaneRequest struct {
	Slug   string `json:"slug"`
	PaneID string `json:"paneId"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

func (b terminalResizePaneRequest) Validate() error {
	return criterio.ValidateStruct(
		criterio.Run("slug", b.Slug, criterio.Required),
		criterio.Run("paneId", b.PaneID, criterio.Required),
	)
}

type terminalSplitResponse struct {
	PaneID string `json:"paneId"`
}

type terminalAttachResponse struct {
	Windows []terminalWindow `json:"windows"`
}

type terminalWindowsResponse struct {
	Windows []terminalWindow `json:"windows"`
}

// terminalSessionWindowsResponse keys window sets by slug. A slug tmux has no
// session for is absent rather than present-and-empty; the two mean the same
// thing to a caller and only one of them needs representing.
type terminalSessionWindowsResponse struct {
	Sessions map[string][]terminalWindow `json:"sessions"`
}

// terminalStartResponse reports whether this call is what spawned the session,
// so a caller can tell "I started it" from "it was already running".
type terminalStartResponse struct {
	Started bool `json:"started"`
}

// terminalKillResponse reports whether there was a session to kill.
type terminalKillResponse struct {
	Killed bool `json:"killed"`
}

type terminalNewWindowResponse struct {
	WindowID string `json:"windowId"`
}

func toTerminalWindow(win tmuxcc.Window) terminalWindow {
	out := terminalWindow{
		WindowID:   win.ID,
		Name:       win.Name,
		Active:     win.Active,
		ActivePane: win.ActivePane,
		Width:      win.Width,
		Height:     win.Height,
		Zoomed:     win.Zoomed,
	}
	// A zero-sized root is a layout tmux has not reported yet, sent as absent
	// rather than as an empty tree.
	if win.Layout.Width != 0 || win.Layout.Height != 0 {
		layout := toTerminalLayout(win.Layout)
		out.Layout = &layout
	}
	return out
}

func toTerminalWindows(windows []tmuxcc.Window) []terminalWindow {
	out := make([]terminalWindow, 0, len(windows))
	for _, win := range windows {
		out = append(out, toTerminalWindow(win))
	}
	return out
}

// TerminalAttach opens the control client for a session slug and returns its
// windows.
func (ctrl *Controller) TerminalAttach(w http.ResponseWriter, r *http.Request) error {
	body, err := terminalBody[terminalAttachRequest](ctrl, w, r)
	if err != nil {
		return err
	}
	windows, err := ctrl.core.Terminals.Attach(r.Context(), body.Slug, body.Cols, body.Rows)
	if err != nil {
		return err
	}
	return server.JSON(w, http.StatusOK, terminalAttachResponse{Windows: toTerminalWindows(windows)})
}

// TerminalStart spawns the tmux session behind a slug so it can be attached to.
func (ctrl *Controller) TerminalStart(w http.ResponseWriter, r *http.Request) error {
	body, err := terminalBody[terminalSlugRequest](ctrl, w, r)
	if err != nil {
		return err
	}
	started, err := ctrl.core.Terminals.Start(r.Context(), body.Slug)
	if err != nil {
		return err
	}
	return server.JSON(w, http.StatusOK, terminalStartResponse{Started: started})
}

// TerminalKill kills the tmux session behind a slug, leaving the hive session
// itself alone.
func (ctrl *Controller) TerminalKill(w http.ResponseWriter, r *http.Request) error {
	body, err := terminalBody[terminalSlugRequest](ctrl, w, r)
	if err != nil {
		return err
	}
	killed, err := ctrl.core.Terminals.Kill(r.Context(), body.Slug)
	if err != nil {
		return err
	}
	return server.JSON(w, http.StatusOK, terminalKillResponse{Killed: killed})
}

// TerminalListWindows answers several sessions' window sets without attaching,
// so the sidebar can show windows for sessions this webview is not attached to.
// It takes the whole set rather than one slug because the caller is a sweep:
// per-slug, each unattached session cost two tmux spawns and the sidebar
// spawned twice as many processes as it had rows, all at once.
func (ctrl *Controller) TerminalListWindows(w http.ResponseWriter, r *http.Request) error {
	body, err := terminalBody[terminalSlugsRequest](ctrl, w, r)
	if err != nil {
		return err
	}
	windows, err := ctrl.core.Terminals.ListAllWindows(r.Context(), body.Slugs)
	if err != nil {
		return err
	}
	sessions := make(map[string][]terminalWindow, len(windows))
	for slug, set := range windows {
		sessions[slug] = toTerminalWindows(set)
	}
	return server.JSON(w, http.StatusOK, terminalSessionWindowsResponse{Sessions: sessions})
}

// TerminalResize renegotiates the control client's size.
func (ctrl *Controller) TerminalResize(w http.ResponseWriter, r *http.Request) error {
	body, err := terminalBody[terminalSizeRequest](ctrl, w, r)
	if err != nil {
		return err
	}
	if err := ctrl.core.Terminals.Resize(r.Context(), body.Slug, body.Cols, body.Rows); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// TerminalNewWindow creates a window and returns its id.
func (ctrl *Controller) TerminalNewWindow(w http.ResponseWriter, r *http.Request) error {
	body, err := terminalBody[terminalSlugRequest](ctrl, w, r)
	if err != nil {
		return err
	}
	id, err := ctrl.core.Terminals.NewWindow(r.Context(), body.Slug)
	if err != nil {
		return err
	}
	return server.JSON(w, http.StatusOK, terminalNewWindowResponse{WindowID: id})
}

func (ctrl *Controller) TerminalNewAgentWindow(w http.ResponseWriter, r *http.Request) error {
	body, err := terminalBody[terminalAgentWindowRequest](ctrl, w, r)
	if err != nil {
		return err
	}
	id, err := ctrl.core.Sessions.NewAgentWindow(r.Context(), body.Slug, body.Agent)
	if err != nil {
		return err
	}
	return server.JSON(w, http.StatusOK, terminalNewWindowResponse{WindowID: id})
}

// TerminalCloseWindow kills one window of an attached session.
func (ctrl *Controller) TerminalCloseWindow(w http.ResponseWriter, r *http.Request) error {
	body, err := terminalBody[terminalWindowRequest](ctrl, w, r)
	if err != nil {
		return err
	}
	if err := ctrl.core.Terminals.CloseWindow(r.Context(), body.Slug, body.WindowID); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// terminalForegroundResponse says what a window is running. command is the
// foreground process's name and is empty when nothing is running or the name
// could not be read, so running is what a caller branches on.
type terminalForegroundResponse struct {
	Running bool   `json:"running"`
	Command string `json:"command"`
}

// TerminalWindowForeground reports whether closing a window would kill work.
func (ctrl *Controller) TerminalWindowForeground(w http.ResponseWriter, r *http.Request) error {
	body, err := terminalBody[terminalWindowRequest](ctrl, w, r)
	if err != nil {
		return err
	}
	foreground, err := ctrl.core.Terminals.WindowForeground(r.Context(), body.Slug, body.WindowID)
	if err != nil {
		return err
	}
	return server.JSON(w, http.StatusOK, terminalForegroundResponse{Running: foreground.Running, Command: foreground.Command})
}

// TerminalRenameWindow renames one window of an attached session.
func (ctrl *Controller) TerminalRenameWindow(w http.ResponseWriter, r *http.Request) error {
	body, err := terminalBody[terminalRenameRequest](ctrl, w, r)
	if err != nil {
		return err
	}
	if err := ctrl.core.Terminals.RenameWindow(r.Context(), body.Slug, body.WindowID, body.Name); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// TerminalMoveWindow moves one window of an attached session to a position in
// its window order.
func (ctrl *Controller) TerminalMoveWindow(w http.ResponseWriter, r *http.Request) error {
	body, err := terminalBody[terminalMoveRequest](ctrl, w, r)
	if err != nil {
		return err
	}
	windows, err := ctrl.core.Terminals.MoveWindow(r.Context(), body.Slug, body.WindowID, body.Position)
	if err != nil {
		return err
	}
	return server.JSON(w, http.StatusOK, terminalWindowsResponse{Windows: toTerminalWindows(windows)})
}

// TerminalSelectWindow makes one window the session's active window.
func (ctrl *Controller) TerminalSelectWindow(w http.ResponseWriter, r *http.Request) error {
	body, err := terminalBody[terminalWindowRequest](ctrl, w, r)
	if err != nil {
		return err
	}
	if err := ctrl.core.Terminals.SelectWindow(r.Context(), body.Slug, body.WindowID); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// TerminalSplitPane splits one pane of an attached session and returns the
// new pane's id.
func (ctrl *Controller) TerminalSplitPane(w http.ResponseWriter, r *http.Request) error {
	body, err := terminalBody[terminalSplitRequest](ctrl, w, r)
	if err != nil {
		return err
	}
	id, err := ctrl.core.Terminals.SplitPane(r.Context(), body.Slug, body.PaneID, tmuxcc.SplitDirection(body.Direction))
	if err != nil {
		return err
	}
	return server.JSON(w, http.StatusOK, terminalSplitResponse{PaneID: id})
}

// TerminalSelectPane makes one pane, or its neighbour in a direction, the
// window's active pane.
func (ctrl *Controller) TerminalSelectPane(w http.ResponseWriter, r *http.Request) error {
	body, err := terminalBody[terminalSelectPaneRequest](ctrl, w, r)
	if err != nil {
		return err
	}
	if err := ctrl.core.Terminals.SelectPane(r.Context(), body.Slug, body.PaneID, tmuxcc.PaneDirection(body.Direction)); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// TerminalClosePane kills one pane of an attached session.
func (ctrl *Controller) TerminalClosePane(w http.ResponseWriter, r *http.Request) error {
	body, err := terminalBody[terminalPaneRequest](ctrl, w, r)
	if err != nil {
		return err
	}
	if err := ctrl.core.Terminals.ClosePane(r.Context(), body.Slug, body.PaneID); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// TerminalPaneForeground reports whether closing a pane would kill work.
func (ctrl *Controller) TerminalPaneForeground(w http.ResponseWriter, r *http.Request) error {
	body, err := terminalBody[terminalPaneRequest](ctrl, w, r)
	if err != nil {
		return err
	}
	foreground, err := ctrl.core.Terminals.PaneForeground(r.Context(), body.Slug, body.PaneID)
	if err != nil {
		return err
	}
	return server.JSON(w, http.StatusOK, terminalForegroundResponse{Running: foreground.Running, Command: foreground.Command})
}

// TerminalResizePane sets a pane's size in cells.
func (ctrl *Controller) TerminalResizePane(w http.ResponseWriter, r *http.Request) error {
	body, err := terminalBody[terminalResizePaneRequest](ctrl, w, r)
	if err != nil {
		return err
	}
	if err := ctrl.core.Terminals.ResizePane(r.Context(), body.Slug, body.PaneID, body.Width, body.Height); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// TerminalZoomPane toggles a pane between filling its window and its place in
// the layout.
func (ctrl *Controller) TerminalZoomPane(w http.ResponseWriter, r *http.Request) error {
	body, err := terminalBody[terminalPaneRequest](ctrl, w, r)
	if err != nil {
		return err
	}
	if err := ctrl.core.Terminals.ZoomPane(r.Context(), body.Slug, body.PaneID); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// TerminalDetach closes the control client, leaving the tmux session running.
func (ctrl *Controller) TerminalDetach(w http.ResponseWriter, r *http.Request) error {
	body, err := terminalBody[terminalSlugRequest](ctrl, w, r)
	if err != nil {
		return err
	}
	if err := ctrl.core.Terminals.Detach(r.Context(), body.Slug); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// terminalBody enforces the bearer token before decoding, so an unauthenticated
// caller learns nothing about the body shape.
func terminalBody[T any](ctrl *Controller, w http.ResponseWriter, r *http.Request) (T, error) {
	var zero T
	if err := requireTerminalToken(r, ctrl.terminalToken); err != nil {
		return zero, err
	}
	return extractors.Body[T](w, r)
}

// requireTerminalToken checks Authorization: Bearer against the per-run token.
// The terminal endpoints authenticate while their siblings deliberately do not:
// a terminal is arbitrary command execution, which no other route on this
// surface offers (ADR terminal-transport).
func requireTerminalToken(r *http.Request, token string) error {
	if token == "" {
		return app.Errorf(app.KindUnavailable, "the terminal control plane is not configured")
	}
	presented := bearerToken(r)
	if presented == "" || subtle.ConstantTimeCompare([]byte(presented), []byte(token)) != 1 {
		return app.Errorf(app.KindUnauthenticated, "a terminal bearer token is required")
	}
	return nil
}

// bearerToken is the Authorization: Bearer credential a request presents, ""
// when it presents none.
func bearerToken(r *http.Request) string {
	presented, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok {
		return ""
	}
	return strings.TrimSpace(presented)
}

// corsPolicy answers preflights and stamps responses for the terminal paths.
// Auth is a bearer token rather than a cookie, so the allowlist does not have
// to be credentialed.
type corsPolicy struct {
	origins []string
	log     zerolog.Logger
}

func (c corsPolicy) wrap(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c.stamp(w, r.Header.Get("Origin"))
		next(w, r)
	}
}

func (c corsPolicy) preflight(w http.ResponseWriter, r *http.Request) {
	if !c.stamp(w, r.Header.Get("Origin")) {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (c corsPolicy) stamp(w http.ResponseWriter, origin string) bool {
	w.Header().Add("Vary", "Origin")
	if origin == "" || !originAllowed(c.origins, origin) {
		if origin != "" {
			c.log.Debug().Str("origin", origin).Strs("allowed", c.origins).Msg("terminal origin rejected")
		}
		return false
	}
	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
	return true
}

// originAllowed matches an Origin header against the allowlist composed in
// main.go. An absent Origin is not this function's call: a non-browser client
// sends none, and the bearer token is what authenticates it either way.
//
// The wails:// scheme is accepted structurally rather than by list: only this
// app's own webview can produce it (a web page's origin is always a web
// scheme), and macOS appends the dev server's freshly-picked port to it every
// dev run, so no static list can name it.
func originAllowed(allowed []string, origin string) bool {
	if origin == "wails://localhost" || strings.HasPrefix(origin, "wails://localhost:") {
		return true
	}
	for _, candidate := range allowed {
		if strings.EqualFold(candidate, origin) {
			return true
		}
	}
	return false
}
