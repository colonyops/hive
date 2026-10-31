package app

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/colonyops/hive/cmd/desktop/internal/app/applets"
	"github.com/colonyops/hive/cmd/desktop/internal/app/settings"
	"github.com/colonyops/hive/pkg/pathutil"
)

const (
	appletActionTimeout = 5 * time.Minute
	maxAppletStdout     = 32 << 20

	appletPagesUnavailable = "applets do not run in the headless server build"
)

// Applet is one catalog entry. Error names what is wrong with its file. An applet
// that once loaded still runs its last good file; Runnable is false when none ever did.
type Applet struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Icon     string `json:"icon"`
	Path     string `json:"path"`
	Builtin  bool   `json:"builtin"`
	Entry    string `json:"entry"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	Revision int64  `json:"revision"`
	Runnable bool   `json:"runnable"`
	Error    string `json:"error"`
	// Titlebar is nil for an applet that offers no title bar page.
	Titlebar *AppletTitlebar `json:"titlebar"`
	Commands []AppletCommand `json:"commands"`
}

// AppletCommand is a palette row an applet declares, bindable as applet.<id>.<command>.
type AppletCommand struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Icon     string   `json:"icon"`
	Keywords []string `json:"keywords"`
}

// AppletTitlebar is the page an applet offers for the title bar.
type AppletTitlebar struct {
	Entry string `json:"entry"`
	Width int    `json:"width"`
}

// AppletsService serves applets their files and answers window.hive (ADR
// applets-are-web-pages-with-full-access-to-the-machine). Nothing runs when a file
// loads: a page runs what it likes once the user opens it.
type AppletsService struct {
	catalog *applets.Store
	environ func(context.Context) []string
	spawns  spawnTable
	// pages reports whether this process may serve applets and answer their
	// bridge, which the headless server build may not.
	pages bool

	tokensMu sync.Mutex
	tokens   map[string]string
	// sessionDir answers where the session on screen is, the same answer a launcher
	// gets, including the refusal for a session on a server.
	sessionDir func(context.Context, string) (string, error)
}

func newAppletsService(catalog *applets.Store, environ func(context.Context) []string, sessionDir func(context.Context, string) (string, error)) *AppletsService {
	return &AppletsService{
		catalog:    catalog,
		environ:    environ,
		sessionDir: sessionDir,
		pages:      buildServesAppletPages || settings.E2EHarnessMarkerValid(),
		tokens:     map[string]string{},
	}
}

// List returns the catalog, broken files included.
func (s *AppletsService) List(context.Context) []Applet {
	entries := s.catalog.List()
	out := make([]Applet, 0, len(entries))
	for _, entry := range entries {
		// The server build cannot open a built-in, and nobody asked it to list one.
		if entry.Applet.Builtin && !s.pages {
			continue
		}
		applet := appletOf(entry)
		if !s.pages {
			applet.Runnable = false
			applet.Error = appletPagesUnavailable
		}
		out = append(out, applet)
	}
	return out
}

// Dir is the root new applets are written under.
func (s *AppletsService) Dir() string { return s.catalog.Root() }

func appletOf(entry applets.Entry) Applet {
	a := entry.Applet
	out := Applet{
		ID:       a.ID,
		Title:    a.Title,
		Icon:     a.Icon,
		Path:     filepath.Join(a.Dir, applets.FileName),
		Builtin:  a.Builtin,
		Revision: entry.Revision,
		Runnable: entry.Usable(),
		Entry:    a.Entry,
		Width:    a.Width,
		Height:   a.Height,
		Commands: make([]AppletCommand, 0, len(a.Commands)),
	}
	for _, command := range a.Commands {
		out.Commands = append(out.Commands, AppletCommand{ID: command.ID, Title: command.Title, Icon: command.Icon, Keywords: command.Keywords})
	}
	if a.Titlebar != nil {
		out.Titlebar = &AppletTitlebar{Entry: a.Titlebar.Entry, Width: a.Titlebar.Width}
	}
	if entry.Err != nil {
		out.Error = entry.Err.Error()
	}
	return out
}

func (s *AppletsService) applet(id string) (applets.Applet, error) {
	applet, ok := s.catalog.Get(id)
	if !ok {
		return applets.Applet{}, Errorf(KindNotFound, "unknown applet %q", id)
	}
	return applet, nil
}

func (s *AppletsService) dir(ctx context.Context, applet applets.Applet, sessionSlug string) (string, error) {
	if applet.Cwd == "" {
		// A session on a server has no directory here, so the page falls back to its own.
		if strings.TrimSpace(sessionSlug) != "" {
			if dir, err := s.sessionDir(ctx, sessionSlug); err == nil {
				return dir, nil
			}
		}
		return applet.Dir, nil
	}
	expanded, err := pathutil.ExpandHomeE(applet.Cwd)
	if err != nil {
		return "", Wrap(err, KindInternal, "expanding %q", applet.Cwd)
	}
	if !filepath.IsAbs(expanded) {
		expanded = filepath.Join(applet.Dir, expanded)
	}
	return expanded, nil
}

func appletEnv(applet applets.Applet) []string {
	return []string{"HIVE_APPLET_DIR=" + applet.Dir}
}
