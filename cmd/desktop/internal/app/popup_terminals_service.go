package app

import (
	"context"
	"errors"
	"os"
	"strings"

	"github.com/colonyops/hive/cmd/desktop/internal/app/actions"
	"github.com/colonyops/hive/cmd/desktop/internal/app/ptyterm"
	"github.com/colonyops/hive/pkg/pathutil"
)

// terminalDirectory answers the checkout a hive session's slug names. It
// belongs to the session domain rather than to this one, the same seam
// StartTmuxSession crosses for the tmux backend.
type terminalDirectory interface {
	SessionDirectory(ctx context.Context, slug string) (string, error)
}

// terminalWorkingDirectory answers where the terminal a slug names currently
// is. It belongs to the terminal domain: the answer is tmux's, and it is the
// one thing that holds for every slug this view attaches — a hive session, the
// scratch terminal, a pinned chat.
type terminalWorkingDirectory interface {
	WorkingDirectory(ctx context.Context, slug string) (string, error)
}

// OpenPopupTerminal is one launch. The directory is resolved in order —
// Launcher's own cwd, where SessionSlug's terminal currently is, then Dir, then
// the user's home — so a caller with a terminal in hand does not have to know
// where it lives, and one with neither still gets a shell somewhere sensible
// rather than wherever the app happened to be started from. The home fallback
// is the bare shell's alone: a launcher that takes its directory from the
// terminal is refused without one (ADR quick-terminal-launchers-are-session-scoped, ADR a-new-tab-and-a-launcher-open-where-the-terminal-s-active-pane-is).
type OpenPopupTerminal struct {
	// Launcher is a configured launcher's id, and supplies the command line and
	// optionally the directory. The caller sends the id rather than the command
	// so what a launcher runs is the catalog's answer and not the client's.
	Launcher string
	// SessionSlug is a tmux session name, not a hive session id. Any slug the
	// Code view attaches answers here, which is what lets a launcher open on
	// the scratch terminal or a pinned chat as readily as on a checkout.
	SessionSlug string
	Dir         string
	Command     string
	Cols        int
	Rows        int
}

// PopupLauncher is one configured launcher, as much of it as a menu needs.
// What it runs is deliberately absent: a caller invokes it by id.
type PopupLauncher struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Icon  string `json:"icon"`
	// RequiresSession reports that this launcher opens wherever the terminal on
	// screen is, so a surface offering it with no terminal attached is offering
	// a launch the core will refuse (ADR quick-terminal-launchers-are-session-scoped).
	RequiresSession bool `json:"requiresSession"`
}

// PopupTerminalsService opens ephemeral terminals: a shell, or a command run
// through one, that this process owns outright and that ends when it is closed
// or when the app exits (ADR ephemeral-popup-terminals). It holds no token, base URL or stream path
// — what it is reached over is the adapter's (ADR terminal-transport).
type PopupTerminalsService struct {
	manager   *ptyterm.Manager
	terminals terminalWorkingDirectory
	directory terminalDirectory
	catalog   *actions.ActionStore
}

type PopupTerminalsDeps struct {
	Manager   *ptyterm.Manager
	Terminals terminalWorkingDirectory
	Directory terminalDirectory
	Catalog   *actions.ActionStore
}

func newPopupTerminalsService(d PopupTerminalsDeps) *PopupTerminalsService {
	return &PopupTerminalsService{manager: d.Manager, terminals: d.Terminals, directory: d.Directory, catalog: d.Catalog}
}

// Available reports build and platform support. There is no external program to
// find, so unlike tmux this cannot become available while the app is running.
func (s *PopupTerminalsService) Available(ctx context.Context) error {
	return popupError(s.manager.Available(ctx), "pop-up terminals need macOS or Linux and a desktop build.")
}

// Launchers returns the configured launchers in file order.
func (s *PopupTerminalsService) Launchers(context.Context) ([]PopupLauncher, error) {
	out := make([]PopupLauncher, 0)
	for _, l := range s.catalog.Launchers() {
		out = append(out, PopupLauncher{ID: l.ID, Label: l.Label, Icon: l.Icon, RequiresSession: l.Cwd == ""})
	}
	return out, nil
}

// Open launches a terminal and returns it.
func (s *PopupTerminalsService) Open(ctx context.Context, req OpenPopupTerminal) (ptyterm.Terminal, error) {
	req, err := s.applyLauncher(req)
	if err != nil {
		return ptyterm.Terminal{}, err
	}
	dir, err := s.resolveDir(ctx, req)
	if err != nil {
		return ptyterm.Terminal{}, err
	}
	term, err := s.manager.Open(ctx, ptyterm.Spec{
		Dir:     dir,
		Command: req.Command,
		Cols:    req.Cols,
		Rows:    req.Rows,
	})
	if err != nil {
		return ptyterm.Terminal{}, popupError(err, "opening a terminal in %q", dir)
	}
	return term, nil
}

// applyLauncher folds a named launcher into the launch spec it stands for — a
// launcher is that spec with config in front of it and nothing more (ADR launchers-are-their-own-list-in-actions-yml).
// A configured cwd wins over the terminal's own directory, which is what pins a
// launcher to one place; without one the launcher is session-scoped and a launch
// with no terminal to resolve is refused rather than opened somewhere else (ADR
// quick-terminal-launchers-are-session-scoped). The caller's own Dir is dropped
// in that case: a session-scoped launcher takes its directory from the terminal,
// and honouring a path beside the slug would be a second way to answer the same
// question.
func (s *PopupTerminalsService) applyLauncher(req OpenPopupTerminal) (OpenPopupTerminal, error) {
	id := strings.TrimSpace(req.Launcher)
	if id == "" {
		return req, nil
	}
	if strings.TrimSpace(req.Command) != "" {
		return req, Errorf(KindInvalid, "a launcher brings its own command, so %q cannot also be given one", id)
	}
	launcher, ok := s.catalog.Launcher(id)
	if !ok {
		return req, Errorf(KindNotFound, "unknown launcher %q", id)
	}
	req.Command = launcher.Command
	if launcher.Cwd != "" {
		req.SessionSlug = ""
		req.Dir = launcher.Cwd
		return req, nil
	}
	if strings.TrimSpace(req.SessionSlug) == "" {
		return req, Errorf(KindInvalid, "%q opens where a terminal is, so it needs a terminal to open in", id)
	}
	req.Dir = ""
	return req, nil
}

// Close ends a terminal and every process in it, and reports whether there was
// one to close.
func (s *PopupTerminalsService) Close(_ context.Context, id string) (bool, error) {
	closed, err := s.manager.Close(id)
	if err != nil {
		return false, popupError(err, "closing terminal %q", id)
	}
	return closed, nil
}

// List reports the open terminals, oldest first.
func (s *PopupTerminalsService) List(context.Context) ([]ptyterm.Terminal, error) {
	return s.manager.List(), nil
}

// Subscribe returns a terminal's event stream and its unsubscribe func. There
// is one active subscriber per terminal; a second call closes the first channel.
func (s *PopupTerminalsService) Subscribe(_ context.Context, id string) (<-chan ptyterm.Event, func(), error) {
	events, unsubscribe, err := s.manager.Subscribe(id)
	if err != nil {
		return nil, nil, popupError(err, "subscribing to terminal %q", id)
	}
	return events, unsubscribe, nil
}

func (s *PopupTerminalsService) Write(_ context.Context, id string, p []byte) error {
	return popupError(s.manager.Write(id, p), "writing to terminal %q", id)
}

// Resize sets the terminal's size outright. Nothing else is attached to these
// PTYs, so unlike the tmux backend this is not a vote.
func (s *PopupTerminalsService) Resize(_ context.Context, id string, cols, rows int) error {
	return popupError(s.manager.Resize(id, cols, rows), "resizing terminal %q", id)
}

func (s *PopupTerminalsService) resolveDir(ctx context.Context, req OpenPopupTerminal) (string, error) {
	if slug := strings.TrimSpace(req.SessionSlug); slug != "" {
		return s.terminalDir(ctx, slug)
	}
	if dir := strings.TrimSpace(req.Dir); dir != "" {
		expanded, err := pathutil.ExpandHomeE(dir)
		if err != nil {
			return "", Wrap(err, KindInternal, "expanding %q", dir)
		}
		return expanded, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", Wrap(err, KindInternal, "finding a directory to open a terminal in")
	}
	return home, nil
}

// terminalDir is where a launch that named a terminal opens: the directory that
// terminal's active pane is in, so "here" is the prompt the user is looking at
// rather than wherever the session was started (ADR a-new-tab-and-a-launcher-open-where-the-terminal-s-active-pane-is).
//
// A slug tmux is not running has no pane to read, and then the hive session's
// checkout is the answer — which is what keeps a launcher working on a session
// whose terminal is stopped, and what leaves a stopped terminal with no record
// behind it reporting that there is no session by that name.
func (s *PopupTerminalsService) terminalDir(ctx context.Context, slug string) (string, error) {
	dir, err := s.terminals.WorkingDirectory(ctx, slug)
	if err == nil {
		return dir, nil
	}
	if KindOf(err) != KindNotFound {
		return "", err
	}
	return s.directory.SessionDirectory(ctx, slug)
}

// popupError classifies a ptyterm failure by sentinel rather than by message.
func popupError(err error, format string, args ...any) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ptyterm.ErrUnavailable):
		return Wrap(err, KindUnavailable, format, args...)
	case errors.Is(err, ptyterm.ErrInvalidSize), errors.Is(err, ptyterm.ErrInvalidSpec):
		return Wrap(err, KindInvalid, format, args...)
	case errors.Is(err, ptyterm.ErrNotFound):
		return Wrap(err, KindNotFound, format, args...)
	default:
		return Wrap(err, KindInternal, format, args...)
	}
}
