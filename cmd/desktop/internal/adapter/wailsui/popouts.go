package wailsui

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
)

// popOutKind is one row of popOutKinds. The frontend's matching entry is in
// frontend/src/windows/surfaces.ts.
type popOutKind struct {
	title               string
	width, height       int
	minWidth, minHeight int
	params              []string
}

var popOutKinds = map[string]popOutKind{
	"drive": {
		title: "Hive Drive",
		width: 900, height: 760, minWidth: 420, minHeight: 320,
		params: []string{"space", "doc", "session"},
	},
}

// PopOutRequest names a surface and where to point it.
type PopOutRequest struct {
	Kind   string            `json:"kind"`
	Params map[string]string `json:"params"`
}

// PopOutResult carries the surface's URL. Opened is false when this build has
// no native windows, and the page opens the URL itself.
type PopOutResult struct {
	URL    string `json:"url"`
	Opened bool   `json:"opened"`
}

type popOutWindow struct {
	id    uint
	raise func()
}

// PopOutService opens surfaces in their own windows, one window per surface and
// scope: asking for a scope that is already open raises its window.
type PopOutService struct {
	native bool
	create func(name string, kind popOutKind, url string, closed func(id uint)) popOutWindow

	// Held across create, which hops to the main thread. The closing hook takes
	// only mu, so a close during a create cannot deadlock.
	openMu sync.Mutex
	nextID int

	mu      sync.Mutex
	windows map[uint]*openPopOut
}

type openPopOut struct {
	window popOutWindow
	scope  string
}

// NewPopOutService builds the service. native is false in the server build,
// where a window would never run and would never leave the Wails window map.
func NewPopOutService(native bool, create func(name string, kind popOutKind, url string, closed func(id uint)) popOutWindow) *PopOutService {
	return &PopOutService{native: native, create: create, windows: map[uint]*openPopOut{}}
}

// Open raises the window showing the request's scope, or creates one.
func (s *PopOutService) Open(req PopOutRequest) (PopOutResult, error) {
	kind, scope, err := resolvePopOut(req)
	if err != nil {
		return PopOutResult{}, err
	}
	result := PopOutResult{URL: "/?" + scope}
	if !s.native {
		return result, nil
	}
	result.Opened = true

	s.openMu.Lock()
	defer s.openMu.Unlock()
	if existing, ok := s.find(scope); ok {
		existing.raise()
		return result, nil
	}
	s.nextID++
	window := s.create(fmt.Sprintf("popout-%d", s.nextID), kind, result.URL, s.closed)
	s.mu.Lock()
	s.windows[window.id] = &openPopOut{window: window, scope: scope}
	s.mu.Unlock()
	return result, nil
}

// Retarget records that the calling window moved to another scope, so asking
// for the scope it left opens a new window and asking for this one raises it.
func (s *PopOutService) Retarget(ctx context.Context, req PopOutRequest) error {
	_, scope, err := resolvePopOut(req)
	if err != nil {
		return err
	}
	caller, ok := ctx.Value(application.WindowKey).(application.Window)
	if !ok {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if open, ok := s.windows[caller.ID()]; ok {
		open.scope = scope
	}
	return nil
}

func (s *PopOutService) find(scope string) (popOutWindow, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, open := range s.windows {
		if open.scope == scope {
			return open.window, true
		}
	}
	return popOutWindow{}, false
}

// A closed window must leave here: Wails does not clear its native pointer, so
// calling a closed window can touch released memory.
func (s *PopOutService) closed(id uint) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.windows, id)
}

// resolvePopOut checks a request against its kind and returns the encoded
// query, which is both the window's URL and its identity. The URL is built here
// so a page can only open a declared surface.
func resolvePopOut(req PopOutRequest) (popOutKind, string, error) {
	kind, ok := popOutKinds[req.Kind]
	if !ok {
		return popOutKind{}, "", app.Errorf(app.KindInvalid, "unknown pop-out kind %q", req.Kind)
	}
	query := url.Values{"popout": {req.Kind}}
	for name, value := range req.Params {
		if !slices.Contains(kind.params, name) {
			return popOutKind{}, "", app.Errorf(app.KindInvalid, "pop-out kind %q has no param %q", req.Kind, name)
		}
		if value != "" {
			query.Set(name, value)
		}
	}
	return kind, query.Encode(), nil
}

func (u *UI) createPopOut(name string, kind popOutKind, startURL string, closed func(id uint)) popOutWindow {
	window := u.app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             name,
		Title:            kind.title,
		Width:            kind.width,
		Height:           kind.height,
		MinWidth:         kind.minWidth,
		MinHeight:        kind.minHeight,
		BackgroundColour: windowBackground,
		URL:              startURL,
		Mac:              hiddenInsetChrome(),
	})
	id := window.ID()
	u.trackFocus(window)

	// No Cancel on the first close: the window is destroyed, which frees its
	// webview. The second is cancelled because a repeated native close crashes
	// Wails beta.21 (wailsapp/wails#6158), and holding ⌘W can repeat it.
	var closing atomic.Bool
	window.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		if closing.Swap(true) {
			e.Cancel()
			return
		}
		closed(id)
		u.focus.Changed()
	})
	return popOutWindow{id: id, raise: func() {
		window.Show()
		window.Focus()
	}}
}
