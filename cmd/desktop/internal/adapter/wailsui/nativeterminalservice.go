package wailsui

import (
	"context"
	"fmt"
	"math"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/rs/zerolog"
	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/colonyops/hive/cmd/desktop/internal/adapter/nativeterm"
	"github.com/colonyops/hive/cmd/desktop/internal/adapter/panebridge"
	"github.com/colonyops/hive/cmd/desktop/internal/app"
	"github.com/colonyops/hive/internal/platform/herdrcore/bridge"
)

// nativeTerminalEvent is the Wails event a native surface's reports ride.
const nativeTerminalEvent = "nativeterm:event"

// NativeTerminalEvent is one surface's report. Kind is closed, title, focus,
// key, or open-url.
type NativeTerminalEvent struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
	Data string `json:"data"`
}

// NativeTerminal is a herdr-core terminal a native tab shows.
type NativeTerminal struct {
	Address string `json:"address"`
	Cwd     string `json:"cwd"`
}

// NativeTerminalService runs native terminal tabs: herdr-core holds each
// program and a libghostty surface over the webview draws it, so the webview
// never renders these terminals. The frontend owns where each surface sits
// and reports its box with SetFrame.
type NativeTerminalService struct {
	herdr     *app.HerdrTerminalsService
	settings  *app.SettingsService
	webhooks  *app.WebhookService
	transport TerminalTransport
	window    func() *application.WebviewWindow
	app       func() *application.App
	logger    zerolog.Logger

	mu    sync.Mutex
	host  *nativeterm.Host
	theme NativeTerminalTheme
}

// NativeTerminalTheme is the page's terminal colors, as `#rrggbb`. A color
// that is empty or not hex keeps the user's Ghostty color.
type NativeTerminalTheme struct {
	Background string `json:"background"`
	Foreground string `json:"foreground"`
	Cursor     string `json:"cursor"`
	// Palette is the 16 ANSI colors, black through bright white.
	Palette []string `json:"palette"`
}

func (t NativeTerminalTheme) equal(o NativeTerminalTheme) bool {
	return t.Background == o.Background && t.Foreground == o.Foreground && t.Cursor == o.Cursor && slices.Equal(t.Palette, o.Palette)
}

// NativeTerminalDeps are NativeTerminalService's collaborators.
type NativeTerminalDeps struct {
	Herdr     *app.HerdrTerminalsService
	Settings  *app.SettingsService
	Webhooks  *app.WebhookService
	Transport TerminalTransport
	Window    func() *application.WebviewWindow
	App       func() *application.App
	Logger    zerolog.Logger
}

func NewNativeTerminalService(d NativeTerminalDeps) *NativeTerminalService {
	return &NativeTerminalService{
		herdr: d.Herdr, settings: d.Settings, webhooks: d.Webhooks, transport: d.Transport,
		window: d.Window, app: d.App, logger: d.Logger,
	}
}

// RendererAvailable reports whether this build can draw tmux panes natively.
func (s *NativeTerminalService) RendererAvailable() bool {
	return nativeterm.Available() && s.transport.PaneStreamPath != ""
}

// OpenPane starts a hidden surface id that draws slug's tmux pane. tmux keeps
// the pane, its size and its session; the surface only renders it.
func (s *NativeTerminalService) OpenPane(ctx context.Context, id, slug, pane string) error {
	if s.transport.PaneStreamPath == "" || s.transport.Token == "" {
		return app.Errorf(app.KindUnavailable, "The pane stream is not mounted in this build.")
	}
	running, port := s.webhooks.Endpoint(ctx)
	if !running {
		return app.Errorf(app.KindUnavailable, "The local HTTP server is not running, so a native pane has nothing to read.")
	}
	host, err := s.ensureHost(ctx)
	if err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return app.Wrap(err, app.KindInternal, "find the Hive executable")
	}
	streamURL := "ws://" + net.JoinHostPort(s.webhooks.Host(), strconv.Itoa(port)) + s.transport.PaneStreamPath
	return host.Open(id, nativeterm.Spec{
		Command:  panebridge.Command(exe, slug, pane),
		FontSize: s.fontSize(ctx),
		Env:      map[string]string{panebridge.EnvURL: streamURL, panebridge.EnvToken: s.transport.Token},
	})
}

// SetTheme gives every native surface the page's terminal colors.
func (s *NativeTerminalService) SetTheme(ctx context.Context, theme NativeTerminalTheme) error {
	s.mu.Lock()
	changed := !s.theme.equal(theme)
	s.theme = theme
	host := s.host
	s.mu.Unlock()
	if !changed || host == nil {
		return nil
	}
	path, err := s.writeConfig(ctx, theme)
	if err != nil {
		return app.Wrap(err, app.KindInternal, "write the native terminal config")
	}
	host.ReloadConfig(path)
	return nil
}

// Available gates native terminal tabs: this build must draw them and
// herdr-core must be installed.
func (s *NativeTerminalService) Available(ctx context.Context) TerminalAvailability {
	if !nativeterm.Available() {
		return TerminalAvailability{Reason: "This build of Hive draws no native terminals."}
	}
	if !s.herdr.Available(ctx) {
		return TerminalAvailability{Reason: "Native terminals run in herdr-core. Install it from https://github.com/herdrdev/herdr-core."}
	}
	return TerminalAvailability{Available: true}
}

// Start opens a shell for slug's session in herdr-core.
func (s *NativeTerminalService) Start(ctx context.Context, slug string) (NativeTerminal, error) {
	t, err := s.herdr.Start(ctx, slug)
	if err != nil {
		return NativeTerminal{}, err
	}
	return NativeTerminal{Address: t.Address, Cwd: t.Cwd}, nil
}

// Kill ends the terminal's program: the user closed its tab.
func (s *NativeTerminalService) Kill(ctx context.Context, address string) error {
	return s.herdr.Kill(ctx, address)
}

// Open starts a hidden surface id attached to address. It does nothing when
// id is already open.
func (s *NativeTerminalService) Open(ctx context.Context, id, address string) error {
	host, err := s.ensureHost(ctx)
	if err != nil {
		return err
	}
	socket, err := s.herdr.Socket(ctx)
	if err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return app.Wrap(err, app.KindInternal, "find the Hive executable")
	}
	return host.Open(id, nativeterm.Spec{Command: bridge.Command(exe, socket, address), FontSize: s.fontSize(ctx)})
}

// SetFrame places surface id over the webview box at x, y, w, h (CSS pixels
// from the viewport's top-left), shown or hidden.
func (s *NativeTerminalService) SetFrame(id string, x, y, w, h float64, visible bool) {
	if host := s.current(); host != nil {
		host.SetFrame(id, nativeterm.Rect{X: x, Y: y, W: w, H: h, Visible: visible})
	}
}

// OccluderRect is a page element floating over the terminals, in CSS pixels
// from the viewport's top-left.
type OccluderRect struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	H float64 `json:"h"`
}

// SetOccluders tells every native surface which page elements float over it
// (menus, popovers, dialogs), so it lets the page show through them.
func (s *NativeTerminalService) SetOccluders(rects []OccluderRect) {
	out := make([]nativeterm.Rect, len(rects))
	for i, r := range rects {
		out[i] = nativeterm.Rect{X: r.X, Y: r.Y, W: r.W, H: r.H}
	}
	nativeterm.SetOccluders(out)
}

// Focus gives surface id the keyboard.
func (s *NativeTerminalService) Focus(id string) {
	if host := s.current(); host != nil {
		host.Focus(id)
	}
}

// Close frees surface id. Its terminal keeps running in herdr-core.
func (s *NativeTerminalService) Close(id string) {
	if host := s.current(); host != nil {
		host.Close(id)
	}
}

func (s *NativeTerminalService) current() *nativeterm.Host {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.host
}

func (s *NativeTerminalService) ensureHost(ctx context.Context) (*nativeterm.Host, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.host != nil {
		return s.host, nil
	}
	if !nativeterm.Available() {
		return nil, app.Errorf(app.KindUnavailable, "This build of Hive draws no native terminals.")
	}
	win := s.window()
	if win == nil {
		return nil, app.Errorf(app.KindUnavailable, "The window is not open yet.")
	}
	configPath, err := s.writeConfig(ctx, s.theme)
	if err != nil {
		s.logger.Warn().Err(err).Msg("native terminal config not written; using Ghostty's")
	}
	host, err := nativeterm.New(win.NativeWindow(), configPath, s.forward)
	if err != nil {
		return nil, app.Wrap(err, app.KindUnavailable, "start native terminals")
	}
	s.host = host
	return host, nil
}

var nativeEventKinds = map[nativeterm.EventKind]string{
	nativeterm.EventClosed:  "closed",
	nativeterm.EventTitle:   "title",
	nativeterm.EventFocus:   "focus",
	nativeterm.EventKey:     "key",
	nativeterm.EventOpenURL: "open-url",
	nativeterm.EventCell:    "cell",
}

// forward runs on the main thread, so the emit leaves it.
func (s *NativeTerminalService) forward(e nativeterm.Event) {
	kind := nativeEventKinds[e.Kind]
	go func() {
		wailsApp := s.app()
		if wailsApp == nil {
			return
		}
		if e.Kind == nativeterm.EventOpenURL {
			if err := wailsApp.Browser.OpenURL(e.Data); err != nil {
				s.logger.Warn().Err(err).Str("url", e.Data).Msg("open terminal link")
			}
			return
		}
		wailsApp.Event.Emit(nativeTerminalEvent, NativeTerminalEvent{ID: e.ID, Kind: kind, Data: e.Data})
	}()
}

// terminalBasePx is the terminal text size at 100%, the same base the
// webview terminal scales (frontend/src/stores/useSurfaceScale.ts).
const terminalBasePx = 13

// fontSize is the terminal text size in points: TerminalScale is a percent
// of terminalBasePx, and a CSS pixel is a point.
func (s *NativeTerminalService) fontSize(ctx context.Context) float64 {
	a, err := s.settings.Appearance(ctx)
	if err != nil || a.TerminalScale <= 0 {
		return terminalBasePx
	}
	return math.Round(terminalBasePx * float64(a.TerminalScale) / 100)
}

// writeConfig writes the settings hive lays over the user's Ghostty config:
// only copy and paste stay with the surface, so every other ⌘ chord reaches
// the app's keymap.
func (s *NativeTerminalService) writeConfig(ctx context.Context, theme NativeTerminalTheme) (string, error) {
	var b strings.Builder
	b.WriteString("keybind = clear\n")
	b.WriteString("keybind = super+c=copy_to_clipboard\n")
	b.WriteString("keybind = super+v=paste_from_clipboard\n")
	b.WriteString("confirm-close-surface = false\n")
	b.WriteString("shell-integration = none\n")
	// The page sizes a tmux pane's box in whole cells, so the grid has to
	// fill the box exactly.
	b.WriteString("window-padding-x = 0\nwindow-padding-y = 0\nwindow-padding-balance = false\n")
	if isHexColor(theme.Background) {
		fmt.Fprintf(&b, "background = %s\n", theme.Background)
	}
	if isHexColor(theme.Foreground) {
		fmt.Fprintf(&b, "foreground = %s\n", theme.Foreground)
	}
	if isHexColor(theme.Cursor) {
		fmt.Fprintf(&b, "cursor-color = %s\n", theme.Cursor)
	}
	for i, color := range theme.Palette {
		if i < 16 && isHexColor(color) {
			fmt.Fprintf(&b, "palette = %d=%s\n", i, color)
		}
	}
	fmt.Fprintf(&b, "font-size = %g\n", s.fontSize(ctx))
	if a, err := s.settings.Appearance(ctx); err == nil && a.TerminalFontFamily != "" {
		fmt.Fprintf(&b, "font-family = %q\n", a.TerminalFontFamily)
	}
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dir = filepath.Join(dir, "hive-desktop")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "ghostty.conf")
	return path, os.WriteFile(path, []byte(b.String()), 0o600)
}

func isHexColor(s string) bool {
	if len(s) != 7 || s[0] != '#' {
		return false
	}
	for _, c := range s[1:] {
		if !strings.ContainsRune("0123456789abcdefABCDEF", c) {
			return false
		}
	}
	return true
}
