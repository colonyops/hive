package app

import (
	"context"
	"strings"

	"github.com/colonyops/hive/cmd/desktop/internal/app/events"
	"github.com/colonyops/hive/internal/domain/drive"
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

// TabPlacement says where a tab goes: behind a tab in a panel, or in a new
// panel split off beside or below one.
type TabPlacement string

const (
	TabPlacementBehind TabPlacement = "behind"
	TabPlacementBeside TabPlacement = "beside"
	TabPlacementBelow  TabPlacement = "below"
)

// TabVerb names one change to a session's panel layout; it is the verb of the
// session:layout event.
type TabVerb string

const (
	TabVerbOpen       TabVerb = "open"
	TabVerbClose      TabVerb = "close"
	TabVerbMove       TabVerb = "move"
	TabVerbFocus      TabVerb = "focus"
	TabVerbClosePanel TabVerb = "close_panel"
	TabVerbResize     TabVerb = "resize"
	TabVerbTurn       TabVerb = "turn"
)

const (
	minSplitShare = 0.2
	maxSplitShare = 0.8
)

// parseTabPlacement reads an empty placement as behind, the one that never
// takes the screen.
func parseTabPlacement(s string) (TabPlacement, error) {
	switch p := TabPlacement(s); p {
	case "":
		return TabPlacementBehind, nil
	case TabPlacementBehind, TabPlacementBeside, TabPlacementBelow:
		return p, nil
	}
	return "", Errorf(KindInvalid, "placement %q is not one of behind, beside, below", s)
}

// TabRef is a tab as an agent names it: a view, plus for a Code session's
// terminal the tmux window.
type TabRef struct {
	View   TabView
	Window string
}

// parseTabRef reads drive, diff and tasks as those views, terminal as the
// terminal view with no window (a chat's one terminal), and anything else as
// a tmux window of a Code session, by id or exact name.
func parseTabRef(s string) (TabRef, error) {
	s = strings.TrimSpace(s)
	switch v := TabView(s); v {
	case "":
		return TabRef{}, Errorf(KindInvalid, "a tab reference is empty; say drive, diff, tasks, terminal, or a window by id (@3) or name")
	case TabViewDrive, TabViewDiff, TabViewTasks, TabViewTerminal:
		return TabRef{View: v}, nil
	}
	return TabRef{View: TabViewTerminal, Window: s}, nil
}

// OpenTabRequest is one open_tab call. Path applies to the drive view; Window
// and Name to the terminal view.
type OpenTabRequest struct {
	Session   string
	View      string
	Path      string
	Placement string
	// Window places an existing window of the session, by tmux id or exact
	// name, instead of starting one.
	Window string
	// Name names the window a terminal request starts.
	Name string
	// Anchor is a tab reference: the tab to place relative to. Empty is the
	// tab in front of the focused panel.
	Anchor string
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
// tmux window of the Code session, a login shell in its checkout, or places
// one that exists. It takes no command: this surface runs nothing an agent
// writes (ADR a-code-session-is-a-panel-layout-that-the-tree-mirrors-and-tmux-follows).
func (s *LayoutsService) OpenTab(ctx context.Context, req OpenTabRequest) (OpenTabResult, error) {
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
	anchor, hasAnchor, err := optionalTabRef(req.Anchor)
	if err != nil {
		return OpenTabResult{}, err
	}
	caller, err := s.callers.tabCaller(ctx, req.Session)
	if err != nil {
		return OpenTabResult{}, err
	}
	if err := checkTabCaller(caller, v); err != nil {
		return OpenTabResult{}, err
	}
	if hasAnchor {
		if anchor, err = s.resolveTab(ctx, caller, anchor); err != nil {
			return OpenTabResult{}, err
		}
	}
	var res OpenTabResult
	switch v {
	case TabViewDrive:
		if req.Path != "" {
			if res.Path, err = s.callers.resolveTabPath(ctx, caller, req.Path); err != nil {
				return OpenTabResult{}, err
			}
		}
	case TabViewDiff, TabViewTasks:
	case TabViewTerminal:
		if res.Window, err = s.terminalWindow(ctx, caller, req); err != nil {
			return OpenTabResult{}, err
		}
	}
	s.publishLayout(ctx, caller, events.SessionLayoutRequested{
		Verb: string(TabVerbOpen), View: string(v), Window: res.Window, Path: res.Path, Placement: string(p),
		AnchorView: string(anchor.View), AnchorWindow: anchor.Window,
	})
	return res, nil
}

func optionalTabRef(s string) (TabRef, bool, error) {
	if strings.TrimSpace(s) == "" {
		return TabRef{}, false, nil
	}
	ref, err := parseTabRef(s)
	return ref, err == nil, err
}

func (s *LayoutsService) publishLayout(ctx context.Context, caller driveCaller, e events.SessionLayoutRequested) {
	e.Session = caller.author.Session
	e.HiveSession = caller.author.HiveSession
	e.Space = caller.space.Key
	s.events.Publish(ctx, e)
}

// checkTabInputs refuses an input the view does not take, before any session
// lookup or tmux call.
func checkTabInputs(v TabView, req OpenTabRequest) error {
	switch {
	case req.Path != "" && v != TabViewDrive:
		return Errorf(KindInvalid, "path applies only to the drive view; %s has no document to pick", v)
	case v != TabViewTerminal && (req.Window != "" || req.Name != ""):
		return Errorf(KindInvalid, "window and name apply only to the terminal view; %s takes neither", v)
	case req.Window != "" && req.Name != "":
		return Errorf(KindInvalid, "window places an existing terminal; omit name, or omit window to start a new one")
	}
	return nil
}

// terminalWindow finds or starts the window a terminal request names and
// answers its id. A hive session's tmux session may not be running: the
// window belongs to it, so a request never starts one.
func (s *LayoutsService) terminalWindow(ctx context.Context, caller driveCaller, req OpenTabRequest) (string, error) {
	if err := s.requireRunning(ctx, caller); err != nil {
		return "", err
	}
	if req.Window != "" {
		return s.terminals.FindWindow(ctx, caller.tmuxSession, req.Window)
	}
	id, err := s.terminals.NewWindow(ctx, caller.tmuxSession, caller.checkout, req.Name)
	if err != nil {
		return "", terminalError(err, "starting a terminal in session %q", caller.tmuxSession)
	}
	return id, nil
}

func (s *LayoutsService) requireRunning(ctx context.Context, caller driveCaller) error {
	if s.terminals == nil {
		return Errorf(KindUnavailable, "terminals are not available here")
	}
	running, err := s.terminals.HasSession(ctx, caller.tmuxSession)
	if err != nil {
		return terminalError(err, "checking whether session %q is running", caller.tmuxSession)
	}
	if !running {
		return Errorf(KindInvalid, "the session is not running; start it in Code first")
	}
	return nil
}

// CloseTab closes the calling session's tab for view. Drive, Diff and Tasks
// ask the UI to close the tab wherever it sits, and documents and canvases are
// untouched. A terminal is a tmux window of a Code session: closing it kills
// the window and everything running in it, and the UI's reconcile removes the
// tab, so no event is published.
func (s *LayoutsService) CloseTab(ctx context.Context, session, view, window string) error {
	v, err := parseTabView(view)
	if err != nil {
		return err
	}
	if window != "" && v != TabViewTerminal {
		return Errorf(KindInvalid, "window applies only to the terminal view; %s takes none", v)
	}
	caller, err := s.callers.tabCaller(ctx, session)
	if err != nil {
		return err
	}
	if v == TabViewTerminal {
		return s.closeTerminal(ctx, caller, window)
	}
	if err := checkTabCaller(caller, v); err != nil {
		return err
	}
	s.publishLayout(ctx, caller, events.SessionLayoutRequested{Verb: string(TabVerbClose), View: string(v)})
	return nil
}

func (s *LayoutsService) closeTerminal(ctx context.Context, caller driveCaller, window string) error {
	if err := checkTabSpace(caller); err != nil {
		return err
	}
	if caller.author.Session != "" {
		return Errorf(KindInvalid, "a chat's terminal is the chat; stop it from its row")
	}
	if window == "" {
		return Errorf(KindInvalid, "close_tab on a terminal needs window: name the window by id (@3) or name")
	}
	ref, err := s.resolveTab(ctx, caller, TabRef{View: TabViewTerminal, Window: window})
	if err != nil {
		return err
	}
	return terminalError(s.terminals.CloseWindow(ctx, caller.tmuxSession, ref.Window), "closing window %q of session %q", ref.Window, caller.tmuxSession)
}

// MoveTabRequest is one move_tab call. Tab and Anchor are tab references.
type MoveTabRequest struct {
	Session   string
	Tab       string
	Placement string
	Anchor    string
}

// MoveTab asks the UI to move a tab. Behind puts it in the anchor's panel
// right after the anchor, or at the end of the focused panel with no anchor;
// beside and below split the anchor's panel, or the focused one, and move the
// tab into the new panel.
func (s *LayoutsService) MoveTab(ctx context.Context, req MoveTabRequest) error {
	tab, err := parseTabRef(req.Tab)
	if err != nil {
		return err
	}
	p, err := parseTabPlacement(req.Placement)
	if err != nil {
		return err
	}
	anchor, hasAnchor, err := optionalTabRef(req.Anchor)
	if err != nil {
		return err
	}
	caller, err := s.callers.tabCaller(ctx, req.Session)
	if err != nil {
		return err
	}
	if tab, err = s.resolveTab(ctx, caller, tab); err != nil {
		return err
	}
	if hasAnchor {
		if anchor, err = s.resolveTab(ctx, caller, anchor); err != nil {
			return err
		}
	}
	s.publishLayout(ctx, caller, events.SessionLayoutRequested{
		Verb: string(TabVerbMove), View: string(tab.View), Window: tab.Window, Placement: string(p),
		AnchorView: string(anchor.View), AnchorWindow: anchor.Window,
	})
	return nil
}

// FocusTab asks the UI to bring a tab forward and focus its panel. tmux is
// not told here: the UI selects a focused window itself, and only while the
// session is on screen, so an agent cannot switch the user's tmux window
// behind another view.
func (s *LayoutsService) FocusTab(ctx context.Context, session, tab string) error {
	caller, ref, err := s.resolveTabCall(ctx, session, tab)
	if err != nil {
		return err
	}
	s.publishTab(ctx, caller, TabVerbFocus, ref, 0)
	return nil
}

// ClosePanel asks the UI to close the panel holding a tab. Its tabs join the
// neighbour panel, and the last panel stays.
func (s *LayoutsService) ClosePanel(ctx context.Context, session, tab string) error {
	caller, ref, err := s.resolveTabCall(ctx, session, tab)
	if err != nil {
		return err
	}
	s.publishTab(ctx, caller, TabVerbClosePanel, ref, 0)
	return nil
}

// ResizeSplit asks the UI to give the side holding the tab's panel share of
// its nearest split. A share outside [0.2, 0.8] is refused: the UI clamps to
// the same range, and an agent that asks for 0.95 should learn it cannot have it.
func (s *LayoutsService) ResizeSplit(ctx context.Context, session, tab string, share float64) error {
	if share < minSplitShare || share > maxSplitShare {
		return Errorf(KindInvalid, "share %v is outside %v to %v", share, minSplitShare, maxSplitShare)
	}
	caller, ref, err := s.resolveTabCall(ctx, session, tab)
	if err != nil {
		return err
	}
	s.publishTab(ctx, caller, TabVerbResize, ref, share)
	return nil
}

// TurnSplit asks the UI to turn the tab's nearest split between side by side
// and stacked.
func (s *LayoutsService) TurnSplit(ctx context.Context, session, tab string) error {
	caller, ref, err := s.resolveTabCall(ctx, session, tab)
	if err != nil {
		return err
	}
	s.publishTab(ctx, caller, TabVerbTurn, ref, 0)
	return nil
}

func (s *LayoutsService) resolveTabCall(ctx context.Context, session, tab string) (driveCaller, TabRef, error) {
	ref, err := parseTabRef(tab)
	if err != nil {
		return driveCaller{}, TabRef{}, err
	}
	caller, err := s.callers.tabCaller(ctx, session)
	if err != nil {
		return driveCaller{}, TabRef{}, err
	}
	ref, err = s.resolveTab(ctx, caller, ref)
	return caller, ref, err
}

func (s *LayoutsService) publishTab(ctx context.Context, caller driveCaller, verb TabVerb, ref TabRef, share float64) {
	s.publishLayout(ctx, caller, events.SessionLayoutRequested{
		Verb: string(verb), View: string(ref.View), Window: ref.Window, Share: share,
	})
}

// resolveTab applies the caller's rules to a tab reference and turns a window
// name into its id. A chat has its Drive tab and its one terminal, which
// carries no window; a Code session names every terminal by its window.
func (s *LayoutsService) resolveTab(ctx context.Context, caller driveCaller, ref TabRef) (TabRef, error) {
	if err := checkTabSpace(caller); err != nil {
		return TabRef{}, err
	}
	chat := caller.author.Session != ""
	switch {
	case chat && ref.View != TabViewDrive && ref.View != TabViewTerminal:
		return TabRef{}, Errorf(KindInvalid, "a chat has only a Drive tab and its terminal; Diff and Tasks belong to a Code session")
	case chat && ref.Window != "":
		return TabRef{}, Errorf(KindInvalid, "a chat has one terminal; say terminal")
	case !chat && ref.View == TabViewTerminal && ref.Window == "":
		return TabRef{}, Errorf(KindInvalid, "name the window by id or name")
	case ref.Window == "":
		return ref, nil
	}
	if err := s.requireRunning(ctx, caller); err != nil {
		return TabRef{}, err
	}
	id, err := s.terminals.FindWindow(ctx, caller.tmuxSession, ref.Window)
	if err != nil {
		return TabRef{}, err
	}
	ref.Window = id
	return ref, nil
}

func checkTabSpace(caller driveCaller) error {
	if caller.space.Kind == drive.SpaceKindShared {
		return Errorf(KindInvalid, "the shared space sits beside no chat or Code session, so it has no tabs; the user reads it in the Drive page")
	}
	return nil
}

func checkTabCaller(caller driveCaller, view TabView) error {
	if err := checkTabSpace(caller); err != nil {
		return err
	}
	if caller.author.Session != "" && view == TabViewTerminal {
		return Errorf(KindInvalid, "a chat has one terminal; new terminals belong to a Code session")
	}
	if caller.author.Session != "" && view != TabViewDrive {
		return Errorf(KindInvalid, "a chat has only a Drive tab; Diff and Tasks belong to a Code session")
	}
	return nil
}
