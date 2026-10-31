package app

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/colonyops/hive/cmd/desktop/internal/app/applets"
	"github.com/colonyops/hive/cmd/desktop/internal/app/dispatch"
	"github.com/colonyops/hive/pkg/executil"
	"github.com/colonyops/hive/pkg/pathutil"
)

const (
	appletDataTimeout   = 30 * time.Second
	appletActionTimeout = 5 * time.Minute
	// maxAppletStdout is well past MaxRows of ordinary rows, so the row cap is
	// what a large listing hits first and truncates on, rather than this cap
	// cutting a JSON document in half.
	maxAppletStdout = 32 << 20
	maxAppletStderr = 4 << 10
)

// AppletAction is an action as much as a surface needs to offer it. What it runs
// is deliberately absent: a caller invokes it by id, so what runs is the file's
// answer and not the client's.
type AppletAction struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Mode    string `json:"mode"`
	Key     string `json:"key"`
	Confirm string `json:"confirm"`
}

type AppletColumn struct {
	Key   string            `json:"key"`
	Label string            `json:"label"`
	Tones map[string]string `json:"tones"`
}

// Applet is one catalog entry. Error names what is wrong with its file. An applet
// that once loaded still runs its last good declaration; Runnable is false when
// none ever did.
type Applet struct {
	ID              string         `json:"id"`
	Title           string         `json:"title"`
	Icon            string         `json:"icon"`
	View            string         `json:"view"`
	Path            string         `json:"path"`
	RequiresSession bool           `json:"requiresSession"`
	RefreshMillis   int64          `json:"refreshMillis"`
	Entry           string         `json:"entry"`
	Width           int            `json:"width"`
	Height          int            `json:"height"`
	Revision        int64          `json:"revision"`
	Runnable        bool           `json:"runnable"`
	Error           string         `json:"error"`
	Columns         []AppletColumn `json:"columns"`
	Actions         []AppletAction `json:"actions"`
}

// AppletRows is one run of an applet's data command.
type AppletRows struct {
	Rows      []applets.Row `json:"rows"`
	Truncated bool          `json:"truncated"`
}

// AppletsService runs the commands applets declare (ADR
// applets-are-declared-in-yaml-and-rendered-natively). Nothing runs when a file
// loads: a data command runs when the applet is on screen, an action when the
// user invokes it.
type AppletsService struct {
	catalog *applets.Store
	environ func(context.Context) []string
	spawns  spawnTable
	// sessionDir answers where a session-scoped applet runs, the same answer a
	// launcher gets, including the refusal for a session on a server.
	sessionDir func(context.Context, string) (string, error)
}

func newAppletsService(catalog *applets.Store, environ func(context.Context) []string, sessionDir func(context.Context, string) (string, error)) *AppletsService {
	return &AppletsService{catalog: catalog, environ: environ, sessionDir: sessionDir}
}

// List returns the catalog, broken files included.
func (s *AppletsService) List(context.Context) []Applet {
	entries := s.catalog.List()
	out := make([]Applet, 0, len(entries))
	for _, entry := range entries {
		out = append(out, appletOf(entry))
	}
	return out
}

// Dir is the root new applets are written under.
func (s *AppletsService) Dir() string { return s.catalog.Root() }

func appletOf(entry applets.Entry) Applet {
	a := entry.Applet
	out := Applet{
		ID:              a.ID,
		Title:           a.Title,
		Icon:            a.Icon,
		View:            a.View,
		Path:            filepath.Join(a.Dir, applets.FileName),
		RequiresSession: entry.Usable() && a.Cwd == "" && a.View != applets.ViewHTML,
		RefreshMillis:   a.Data.Refresh.Milliseconds(),
		Revision:        entry.Revision,
		Runnable:        entry.Usable(),
		Entry:           a.Entry,
		Width:           a.Width,
		Height:          a.Height,
		Columns:         make([]AppletColumn, 0, len(a.Columns)),
		Actions:         make([]AppletAction, 0, len(a.Actions)),
	}
	if entry.Err != nil {
		out.Error = entry.Err.Error()
	}
	for _, c := range a.Columns {
		label := c.Label
		if label == "" {
			label = c.Key
		}
		out.Columns = append(out.Columns, AppletColumn{Key: c.Key, Label: label, Tones: c.Tones})
	}
	for _, action := range a.Actions {
		out.Actions = append(out.Actions, AppletAction{ID: action.ID, Label: action.Label, Mode: action.Mode, Key: action.Key, Confirm: action.Confirm})
	}
	return out
}

// Rows runs the applet's data command.
func (s *AppletsService) Rows(ctx context.Context, id, sessionSlug string) (AppletRows, error) {
	applet, err := s.applet(id)
	if err != nil {
		return AppletRows{}, err
	}
	dir, err := s.dir(ctx, applet, sessionSlug)
	if err != nil {
		return AppletRows{}, err
	}
	stdout, err := s.run(ctx, appletDataTimeout, applet.Data.Command, dir, appletEnv(applet))
	if err != nil {
		return AppletRows{}, Wrap(err, KindUnavailable, "running the data command of %q", id)
	}
	rows, truncated, err := applets.ParseRows(applet.Data.Format, stdout)
	if err != nil {
		return AppletRows{}, Wrap(err, KindInvalid, "reading the data command output of %q", id)
	}
	return AppletRows{Rows: rows, Truncated: truncated}, nil
}

// Run runs a background action against a row and returns its stdout. A terminal
// action opens through PopupTerminalsService instead.
func (s *AppletsService) Run(ctx context.Context, id, actionID, sessionSlug string, row applets.Row) (string, error) {
	applet, action, err := s.action(id, actionID)
	if err != nil {
		return "", err
	}
	if action.Mode != applets.ModeBackground {
		return "", Errorf(KindInvalid, "action %q of %q opens in a terminal", actionID, id)
	}
	dir, err := s.dir(ctx, applet, sessionSlug)
	if err != nil {
		return "", err
	}
	env, err := appletActionEnv(applet, row)
	if err != nil {
		return "", err
	}
	stdout, err := s.run(ctx, appletActionTimeout, action.Run, dir, env)
	if err != nil {
		return "", Wrap(err, KindUnavailable, "%s", action.Label)
	}
	return strings.TrimSpace(string(stdout)), nil
}

// appletLaunch is what the pop-up runs for a terminal action.
type appletLaunch struct {
	Command string
	Dir     string
	Env     []string
}

// terminalLaunch resolves a terminal action into what the pop-up runs.
func (s *AppletsService) terminalLaunch(ctx context.Context, id, actionID, sessionSlug string, row applets.Row) (appletLaunch, error) {
	applet, action, err := s.action(id, actionID)
	if err != nil {
		return appletLaunch{}, err
	}
	if action.Mode != applets.ModeTerminal {
		return appletLaunch{}, Errorf(KindInvalid, "action %q of %q does not open in a terminal", actionID, id)
	}
	dir, err := s.dir(ctx, applet, sessionSlug)
	if err != nil {
		return appletLaunch{}, err
	}
	env, err := appletActionEnv(applet, row)
	if err != nil {
		return appletLaunch{}, err
	}
	return appletLaunch{Command: action.Run, Dir: dir, Env: env}, nil
}

func (s *AppletsService) applet(id string) (applets.Applet, error) {
	applet, ok := s.catalog.Get(id)
	if !ok {
		return applets.Applet{}, Errorf(KindNotFound, "unknown applet %q", id)
	}
	return applet, nil
}

func (s *AppletsService) action(id, actionID string) (applets.Applet, applets.Action, error) {
	applet, err := s.applet(id)
	if err != nil {
		return applets.Applet{}, applets.Action{}, err
	}
	action, ok := applet.Action(actionID)
	if !ok {
		return applets.Applet{}, applets.Action{}, Errorf(KindNotFound, "applet %q has no action %q", id, actionID)
	}
	return applet, action, nil
}

func (s *AppletsService) dir(ctx context.Context, applet applets.Applet, sessionSlug string) (string, error) {
	if applet.Cwd == "" && applet.View == applets.ViewHTML {
		// A page has no notion of being session-scoped, so it follows the session when
		// it can, including when that session is on a server and has no directory here.
		if strings.TrimSpace(sessionSlug) != "" {
			if dir, err := s.sessionDir(ctx, sessionSlug); err == nil {
				return dir, nil
			}
		}
		return applet.Dir, nil
	}
	if applet.Cwd == "" {
		if strings.TrimSpace(sessionSlug) == "" {
			return "", Errorf(KindInvalid, "%q runs where a session is, so it needs a session on screen", applet.ID)
		}
		return s.sessionDir(ctx, sessionSlug)
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

func appletActionEnv(applet applets.Applet, row applets.Row) ([]string, error) {
	rowEnv, err := applets.RowEnv(row)
	if err != nil {
		return nil, Wrap(err, KindInvalid, "applet %q", applet.ID)
	}
	return append(appletEnv(applet), rowEnv...), nil
}

// run is sh -c in execenv's environment, the same way a shell action runs (ADR
// subprocess-environment). A failure carries the command's stderr, which is what
// tells the reader, or the agent that wrote the applet, what to fix.
func (s *AppletsService) run(ctx context.Context, timeout time.Duration, command, dir string, env []string) ([]byte, error) {
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	proc := exec.CommandContext(context.WithoutCancel(runCtx), "sh", "-c", command)
	proc.Dir = dir
	proc.Env = append(s.environ(runCtx), env...)
	stdout := &executil.HeadWriter{Max: maxAppletStdout}
	stderr := &executil.HeadWriter{Max: maxAppletStderr}
	proc.Stdout, proc.Stderr = stdout, stderr
	if err := dispatch.RunShellProcess(runCtx, proc); err != nil {
		if detail := strings.TrimSpace(stderr.String()); detail != "" {
			return nil, errors.New(detail)
		}
		return nil, err
	}
	return []byte(stdout.String()), nil
}
