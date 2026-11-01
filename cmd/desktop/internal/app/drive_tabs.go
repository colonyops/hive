package app

import (
	"context"

	"github.com/colonyops/hive/cmd/desktop/internal/app/events"
	"github.com/colonyops/hive/internal/domain/drive"
	drivesvc "github.com/colonyops/hive/internal/hive/drive"
)

// TabView names a view that opens as a tab beside a chat or Code session.
type TabView string

const (
	TabViewDrive TabView = "drive"
	TabViewDiff  TabView = "diff"
	TabViewTasks TabView = "tasks"
	// TabViewTerminal is one tmux window of a Code session; a chat has one
	// terminal and opens no more.
	TabViewTerminal TabView = "terminal"
)

func parseTabView(s string) (TabView, error) {
	switch v := TabView(s); v {
	case TabViewDrive, TabViewDiff, TabViewTasks, TabViewTerminal:
		return v, nil
	}
	return "", Errorf(KindInvalid, "view %q is not one of drive, diff, tasks, terminal", s)
}

// TabPlacement says where an opened tab goes: behind the tab in front, or in
// a split beside it.
type TabPlacement string

const (
	TabPlacementBehind TabPlacement = "behind"
	TabPlacementBeside TabPlacement = "beside"
)

// parseTabPlacement reads an empty placement as behind, the one that never
// takes the screen.
func parseTabPlacement(s string) (TabPlacement, error) {
	switch p := TabPlacement(s); p {
	case "":
		return TabPlacementBehind, nil
	case TabPlacementBehind, TabPlacementBeside:
		return p, nil
	}
	return "", Errorf(KindInvalid, "placement %q is not one of behind, beside", s)
}

// OpenTabRequest is one open_tab call. Path applies to the drive view;
// Window, Name and Command to the terminal view.
type OpenTabRequest struct {
	Session   string
	View      string
	Path      string
	Placement string
	// Window places an existing window of the session, by tmux id or exact
	// name, instead of starting one.
	Window string
	// Name and Command shape the window a terminal request starts.
	Name    string
	Command string
}

// OpenTabResult is what the UI was asked to open. Path is the document path
// the drive view resolved, Window the terminal's tmux window id; each is empty
// for the other views.
type OpenTabResult struct {
	Path   string
	Window string
}

// OpenTab asks the UI to open one tab beside the calling chat or Code
// session. It is UI intent, applied only while the user is viewing that chat
// or session, so an agent can surface what it made without taking the screen
// from the user. A path names an existing document or canvas of the Drive
// view; a canvas may be given by name. The terminal view starts a detached
// tmux window of the Code session, or places one that exists.
func (s *DriveService) OpenTab(ctx context.Context, req OpenTabRequest) (OpenTabResult, error) {
	v, err := parseTabView(req.View)
	if err != nil {
		return OpenTabResult{}, err
	}
	p, err := parseTabPlacement(req.Placement)
	if err != nil {
		return OpenTabResult{}, err
	}
	if err := checkTabInputs(v, req); err != nil {
		return OpenTabResult{}, err
	}
	d, caller, err := s.open(ctx, req.Session)
	if err != nil {
		return OpenTabResult{}, err
	}
	if err := checkTabCaller(caller, v); err != nil {
		return OpenTabResult{}, err
	}
	var res OpenTabResult
	switch v {
	case TabViewDrive:
		if req.Path != "" {
			if res.Path, err = s.resolveTabPath(ctx, d, caller.space, req.Path); err != nil {
				return OpenTabResult{}, err
			}
		}
	case TabViewDiff, TabViewTasks:
	case TabViewTerminal:
		if res.Window, err = s.terminalWindow(ctx, caller, req); err != nil {
			return OpenTabResult{}, err
		}
	}
	s.events.Publish(ctx, events.SessionTabRequested{
		Session: caller.author.Session, HiveSession: caller.author.HiveSession,
		Space: caller.space.Key, View: string(v), Path: res.Path, Window: res.Window,
		Placement: string(p), Open: true,
	})
	return res, nil
}

// checkTabInputs refuses an input the view does not take, before any session
// lookup or tmux call.
func checkTabInputs(v TabView, req OpenTabRequest) error {
	switch {
	case req.Path != "" && v != TabViewDrive:
		return Errorf(KindInvalid, "path applies only to the drive view; %s has no document to pick", v)
	case v != TabViewTerminal && (req.Window != "" || req.Name != "" || req.Command != ""):
		return Errorf(KindInvalid, "window, name and command apply only to the terminal view; %s takes none", v)
	case req.Window != "" && (req.Name != "" || req.Command != ""):
		return Errorf(KindInvalid, "window places an existing terminal; omit name and command, or omit window to start a new one")
	}
	return nil
}

// terminalWindow finds or starts the window a terminal request names and
// answers its id. A hive session's tmux session may not be running: the
// window belongs to it, so a request never starts one.
func (s *DriveService) terminalWindow(ctx context.Context, caller driveCaller, req OpenTabRequest) (string, error) {
	if s.terminals == nil {
		return "", Errorf(KindUnavailable, "terminals are not available here")
	}
	running, err := s.terminals.HasSession(ctx, caller.tmuxSession)
	if err != nil {
		return "", terminalError(err, "checking whether session %q is running", caller.tmuxSession)
	}
	if !running {
		return "", Errorf(KindInvalid, "the session is not running; start it in Code first")
	}
	if req.Window != "" {
		return s.terminals.FindWindow(ctx, caller.tmuxSession, req.Window)
	}
	id, err := s.terminals.NewWindow(ctx, caller.tmuxSession, caller.checkout, req.Name, req.Command)
	if err != nil {
		return "", terminalError(err, "starting a terminal in session %q", caller.tmuxSession)
	}
	return id, nil
}

// CloseTab asks the UI to close the calling session's tab for view, wherever
// it sits. Documents and canvases are untouched.
func (s *DriveService) CloseTab(ctx context.Context, session, view string) error {
	v, err := parseTabView(view)
	if err != nil {
		return err
	}
	if v == TabViewTerminal {
		return Errorf(KindInvalid, "a terminal closes from its tab or its shell; close_tab takes drive, diff or tasks")
	}
	_, caller, err := s.open(ctx, session)
	if err != nil {
		return err
	}
	if err := checkTabCaller(caller, v); err != nil {
		return err
	}
	s.events.Publish(ctx, events.SessionTabRequested{
		Session: caller.author.Session, HiveSession: caller.author.HiveSession,
		Space: caller.space.Key, View: string(v),
	})
	return nil
}

func checkTabCaller(caller driveCaller, view TabView) error {
	if caller.space.Kind == drive.SpaceKindShared {
		return Errorf(KindInvalid, "the shared space sits beside no chat or Code session, so it has no tabs; the user reads it in the Drive page")
	}
	if caller.author.Session != "" && view == TabViewTerminal {
		return Errorf(KindInvalid, "a chat has one terminal; new terminals belong to a Code session")
	}
	if caller.author.Session != "" && view != TabViewDrive {
		return Errorf(KindInvalid, "a chat has only a Drive tab; Diff and Tasks belong to a Code session")
	}
	return nil
}

// resolveTabPath maps what an agent typed to the path the Drive tab shows: a
// document at that path, else the canvas that name or path stands for.
func (s *DriveService) resolveTabPath(ctx context.Context, d *drivesvc.Service, space drive.Space, path string) (string, error) {
	if err := drive.ValidatePath(path); err != nil {
		return "", documentError(err, space, path)
	}
	isDir, err := s.isDir(d, space, path)
	switch {
	case err == nil && isDir:
		return "", Errorf(KindInvalid, "%q is a folder; open a document inside it", path)
	case err == nil:
		return path, nil
	case KindOf(err) != KindNotFound:
		return "", err
	}
	cp, err := canvasPath(path)
	if err != nil {
		return "", documentNotFound(space, path)
	}
	c, ok, err := d.LoadCanvas(ctx, space, cp)
	if err != nil {
		return "", driveError(err, path)
	}
	if !ok {
		return "", documentNotFound(space, path)
	}
	return c.Path, nil
}
