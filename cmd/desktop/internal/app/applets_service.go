package app

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"

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
	// Enabled is false for an applet the user turned off. It stays listed, and nothing
	// of it runs.
	Enabled bool `json:"enabled"`
	// Warnings names what this Hive skipped in the file, such as a key from a later
	// release. The applet runs without it.
	Warnings []string `json:"warnings"`
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

	// hiveVersion is this build's, which a file can say is too old and an export records.
	hiveVersion string
	preferences appletPreferences
	updated     func()
	logger      zerolog.Logger

	shareMu       sync.Mutex
	pendingImport *pendingAppletImport
	pendingExport *pendingAppletExport
}

// appletPreferences is what the user chose about applets, kept in settings.yaml.
type appletPreferences interface {
	AppletDisabled(id string) (bool, error)
	SetAppletDisabled(id string, disabled bool) error
	ForgetApplet(id string) error
}

type pendingAppletExport struct {
	token string
	data  []byte
}

type pendingAppletImport struct {
	token   string
	archive applets.Archive
}

// AppletsDeps is what AppletsService needs from the rest of the app.
type AppletsDeps struct {
	Catalog     *applets.Store
	Environ     func(context.Context) []string
	SessionDir  func(context.Context, string) (string, error)
	HiveVersion string
	Preferences appletPreferences
	// Updated tells every window the catalog changed.
	Updated func()
	Logger  zerolog.Logger
}

func newAppletsService(deps AppletsDeps) *AppletsService {
	return &AppletsService{
		catalog:     deps.Catalog,
		environ:     deps.Environ,
		sessionDir:  deps.SessionDir,
		hiveVersion: deps.HiveVersion,
		preferences: deps.Preferences,
		updated:     deps.Updated,
		logger:      deps.Logger,
		pages:       buildServesAppletPages || settings.E2EHarnessMarkerValid(),
		tokens:      map[string]string{},
	}
}

// isDisabled reads settings.yaml. A file that cannot be read leaves applets running, as it
// leaves the rest of the app, rather than stopping every applet over one bad edit.
func (s *AppletsService) isDisabled(id string) bool {
	disabled, err := s.preferences.AppletDisabled(id)
	if err != nil {
		s.logger.Warn().Err(err).Str("applet", id).Msg("could not read whether an applet is turned off")
		return false
	}
	return disabled
}

// SetEnabled turns an applet on or off. The frames an applet has open close when the
// frontend hears the catalog changed, and what they started stops with them.
func (s *AppletsService) SetEnabled(id string, enabled bool) error {
	if !slices.ContainsFunc(s.catalog.List(), func(entry applets.Entry) bool { return entry.Applet.ID == id }) {
		return Errorf(KindNotFound, "unknown applet %q", id)
	}
	if err := s.preferences.SetAppletDisabled(id, !enabled); err != nil {
		return Wrap(err, KindInternal, "saving settings")
	}
	s.updated()
	return nil
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
		applet.Enabled = !s.isDisabled(entry.Applet.ID)
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
		Warnings: a.Warnings,
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
