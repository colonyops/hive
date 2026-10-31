package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/shirou/gopsutil/v4/process"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/colonyops/hive/internal/domain/multiplexer"
	"github.com/colonyops/hive/internal/platform/observe"
	"github.com/colonyops/hive/internal/platform/tmux/control"
)

// terminalStarter spawns the tmux session a slug names when tmux has none. It
// belongs to the session domain rather than to this one: the slug names a hive
// session, and what its terminal holds is hive's spawn configuration.
type terminalStarter interface {
	StartTmuxSession(ctx context.Context, slug string) (multiplexer.LaunchResult, error)
}

// ScratchSlug is the tmux session name of the scratch terminal — the one
// terminal in the app that belongs to no piece of tracked work.
//
// It is a slug hive cannot mint: Slugify lowercases before it replaces, so no
// session a user can name reaches a capital letter, and nothing hive creates can
// therefore shadow the scratch session or be shadowed by it. `scratch` would
// have been that session's slug the first time someone named one that.
const ScratchSlug = "Scratch"

// scratchName heads the section its tabs are listed in, which is the only place
// it is drawn — plural because that is what is under it. `tmux ls` still says
// Scratch, which is the name that has to be addressable.
const scratchName = "Terminals"

// ScratchTerminal declares the scratch terminal to the surfaces that draw it.
// There is exactly one, it is created on first use, and it holds no hive
// session, checkout or agent — its tabs are whatever the user opened.
type ScratchTerminal struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
}

// terminalServers gives the tmux manager of a server under servers:.
type terminalServers interface {
	Manager(ctx context.Context, name string) (*tmuxcc.Manager, error)
}

// serverSweepBudget bounds one server's part of a sweep. Without it a server
// that freezes holds the sidebar, local sessions too, for the 70s ssh takes to
// give up on a shared connection. It is ssh's ConnectTimeout: a shorter budget
// kills a slow first connect on every sweep, so that server never answers.
const serverSweepBudget = 10 * time.Second

// TerminalsService is the slug-keyed driving service both the HTTP and the
// Wails adapter call. It holds no token, base URL or stream path: what the
// terminal is reached over is the adapter's, not the core's (ADR terminal-transport).
//
// A target is the terminal of a session: "<server>:<slug>" on that server, run
// over ssh, or a bare slug, which is local. See splitTarget.
type TerminalsService struct {
	manager *tmuxcc.Manager
	servers terminalServers
	starter terminalStarter
	home    func() (string, error)
	// foreground answers whether a pid holds its terminal's foreground process
	// group. It is a field so a test can drive the answer without arranging the
	// process states it stands for.
	foreground func(ctx context.Context, pid int) (bool, error)
	log        zerolog.Logger
	// serverSweepBudget is a field so a test need not wait the real one out.
	serverSweepBudget time.Duration
}

type TerminalsDeps struct {
	Manager *tmuxcc.Manager
	Servers terminalServers
	Starter terminalStarter
	Home    func() (string, error)
	Logger  zerolog.Logger
}

func newTerminalsService(d TerminalsDeps) *TerminalsService {
	return &TerminalsService{serverSweepBudget: serverSweepBudget, manager: d.Manager, servers: d.Servers, starter: d.Starter, home: d.Home, foreground: processForeground, log: d.Logger}
}

// Scratch declares the scratch terminal. It is a constant rather than a probe:
// whether tmux is holding the session is what Start and the window listing
// answer, and a caller that draws the row needs it before either.
func (s *TerminalsService) Scratch(_ context.Context) ScratchTerminal {
	return ScratchTerminal{Slug: ScratchSlug, Name: scratchName}
}

// Available reports tmux/build/platform availability only. Whether the
// loopback server that carries the transport is up is composed by the adapter
// that owns that transport.
func (s *TerminalsService) Available(ctx context.Context) error {
	return terminalError(s.manager.Available(ctx), "Terminal sessions need tmux 3.2 or newer. Hive searches PATH and the usual install prefixes; set paths.tmux in settings.yaml if yours is elsewhere.")
}

// Attach opens, or returns the windows of, the control client for slug, and
// leaves a first paint on its stream either way: a control client that outlived
// the transport rendering it is repainted rather than handed back with nothing
// to draw. cols and rows are the caller's opening size vote, read only when the
// attach opens a client; 0x0 attaches without setting a client size at all,
// which leaves the session at the size its other clients gave it until the
// first Resize.
//
// A slug tmux is not running is a KindNotFound the caller is expected to answer
// with Start — attaching never spawns on its own, because spawning runs the
// session's agent command and that is the user's call to make (ADR terminal-start-is-an-offered-action). That
// answer comes from a has-session probe rather than from a dead control
// stream's message, which is unclassifiable and reads as an internal fault.
func (s *TerminalsService) Attach(ctx context.Context, target string, cols, rows int) ([]tmuxcc.Window, error) {
	// A trigger, and the one behind "why was the terminal slow to open": the
	// probe, the handshake, the size vote and the first paint all hang off it,
	// and only this level knows they were one attach
	// (ADR a-span-is-a-trigger-or-a-wait-and-its-count-per-trigger-is-bounded-by-configuration).
	ctx, span := tracer.Start(ctx, "terminal.attach", trace.WithAttributes(
		attribute.String(attrTerminalSlug, target),
		attribute.Int(attrTerminalCols, cols),
		attribute.Int(attrTerminalRows, rows),
	))
	defer span.End()

	manager, slug, err := s.route(ctx, target)
	if err != nil {
		observe.RecordError(span, err)
		return nil, err
	}
	exists, err := manager.HasSession(ctx, slug)
	if err != nil {
		observe.RecordError(span, err)
		if server, _, remote := splitTarget(target); remote {
			if errors.Is(err, tmuxcc.ErrUnavailable) {
				// The caller sees only the message, and tmux's usual answer for this
				// on a local machine does not apply to a server.
				return nil, Wrap(err, KindUnavailable, "server %q has no usable tmux (%v); check ssh to it, or set tmux for it under servers: in config.yaml", server, err)
			}
			return nil, serverError(err, KindUnavailable, "attaching to session %q on server %q", slug, server)
		}
		return nil, terminalError(err, "attaching to session %q", target)
	}
	// A session tmux is not running answers the attach rather than failing it,
	// so the attribute is what separates that span from one that attached.
	span.SetAttributes(attribute.Bool(attrTerminalRunning, exists))
	if !exists {
		return nil, Errorf(KindNotFound, "session %q is not running", target)
	}
	if err := manager.CheckLaunch(ctx, slug); err != nil {
		return nil, terminalError(err, "attaching to session %q", target)
	}
	windows, err := manager.Attach(ctx, slug, cols, rows)
	if err != nil {
		observe.RecordError(span, err)
		return nil, terminalError(err, "attaching to session %q", target)
	}
	span.SetAttributes(attribute.Int(attrTerminalWindows, len(windows)))
	return windows, nil
}

// Start spawns the tmux session slug names, from the hive session's own spawn
// configuration, and reports whether it had to. A session tmux is already
// running is left alone.
//
// ScratchSlug is the one slug this does not ask hive about — see startScratch.
//
// A server's session is started on the server: this app does not have the
// server's rules to start it with.
func (s *TerminalsService) Start(ctx context.Context, target string) (multiplexer.LaunchResult, error) {
	server, slug, remote := splitTarget(target)
	if remote {
		return multiplexer.LaunchResult{}, Errorf(KindUnavailable, "session %q is on server %q; start its terminal on that server", target, server)
	}
	exists, err := s.manager.HasSession(ctx, slug)
	if err != nil {
		return multiplexer.LaunchResult{}, terminalError(err, "starting session %q", slug)
	}
	if exists {
		if err := s.manager.CheckLaunch(ctx, slug); err != nil {
			if errors.Is(err, tmuxcc.ErrLaunchPending) && slug != ScratchSlug {
				return s.starter.StartTmuxSession(ctx, slug)
			}
			return multiplexer.LaunchResult{}, terminalError(err, "starting session %q", slug)
		}
		return multiplexer.LaunchResult{}, nil
	}
	if slug == ScratchSlug {
		return multiplexer.LaunchResult{Created: true}, s.startScratch(ctx)
	}
	return s.starter.StartTmuxSession(ctx, slug)
}

// startScratch creates the scratch session here rather than through hive's spawn
// configuration, which has nothing to say about it: there is no session record,
// no remote for ResolveSpawn to match a rule against, and no checkout to open in.
// What it gets instead is the user's home directory and an interactive login
// shell — the same empty-command create an agent workspace's session uses, and
// what makes the shell's own startup files, not this app's environment, decide
// what is on PATH (ADR subprocess-environment). Home is the *session's* directory rather than that
// first window's, so every tab opened in it later starts there too.
func (s *TerminalsService) startScratch(ctx context.Context) error {
	home, err := s.home()
	if err != nil {
		return Wrap(err, KindUnavailable, "finding your home directory to open the scratch terminal in")
	}
	return terminalError(s.manager.NewSession(ctx, ScratchSlug, home, "", nil), "starting the scratch terminal")
}

// Kill kills the tmux session slug names and reports whether there was one to
// kill. It is the terminal's own lifecycle only: the hive session, its checkout
// and its record are untouched, which is what separates this from a delete or a
// recycle. Whatever was running inside — the agent included — stops with it.
func (s *TerminalsService) Kill(ctx context.Context, target string) (bool, error) {
	manager, slug, err := s.route(ctx, target)
	if err != nil {
		return false, err
	}
	killed, err := manager.KillSession(ctx, slug)
	if err != nil {
		return false, terminalError(err, "killing the terminal for session %q", target)
	}
	return killed, nil
}

// ListAllWindows answers the window sets of every slug in one tmux call per
// server, keyed by slug. This is what the sidebar sweeps with: asking per slug
// spawned two tmux processes for each unattached session. A slug with no tmux
// session behind it is absent from the result rather than an error, and so are
// the slugs of a server that does not answer: one server offline must not fail
// the sweep of the others or hold it past serverSweepBudget.
func (s *TerminalsService) ListAllWindows(ctx context.Context, targets []string) (map[string][]tmuxcc.Window, error) {
	local := []string{}
	remote := map[string][]string{}
	for _, target := range targets {
		if server, slug, isRemote := splitTarget(target); isRemote {
			remote[server] = append(remote[server], slug)
			continue
		}
		local = append(local, target)
	}
	windows, err := s.manager.ListAllWindows(ctx, local)
	if err != nil {
		return nil, terminalError(err, "listing windows of %d sessions", len(local))
	}
	var (
		wg sync.WaitGroup
		mu sync.Mutex
	)
	for server, slugs := range remote {
		wg.Go(func() {
			ctx, cancel := context.WithTimeout(ctx, s.serverSweepBudget)
			defer cancel()
			manager, err := s.serverManager(ctx, server)
			if err == nil {
				var found map[string][]tmuxcc.Window
				found, err = manager.ListAllWindows(ctx, slugs)
				mu.Lock()
				defer mu.Unlock()
				for slug, set := range found {
					if windows == nil {
						windows = map[string][]tmuxcc.Window{}
					}
					windows[server+":"+slug] = set
				}
			}
			if err != nil {
				s.log.Debug().Err(err).Str("server", server).Int("sessions", len(slugs)).Msg("listing windows on a server failed")
			}
		})
	}
	wg.Wait()
	return windows, nil
}

// Subscribe returns slug's event stream and its unsubscribe func. There is one
// active subscriber per session: a second call closes the first channel, which
// is how a replaced transport learns it was replaced.
func (s *TerminalsService) Subscribe(ctx context.Context, target string) (<-chan tmuxcc.Event, func(), error) {
	manager, slug, err := s.route(ctx, target)
	if err != nil {
		return nil, nil, err
	}
	events, unsubscribe, err := manager.Subscribe(slug)
	if err != nil {
		return nil, nil, terminalError(err, "subscribing to session %q", target)
	}
	return events, unsubscribe, nil
}

// Write sends bytes to a pane. The pane is named rather than resolved from a
// window's active pane, because the keystrokes that follow a click into a
// pane are sent while the select-pane it caused is still in flight.
func (s *TerminalsService) Write(ctx context.Context, target, paneID string, p []byte) error {
	client, err := s.client(ctx, target)
	if err != nil {
		return err
	}
	return terminalError(client.Write(ctx, paneID, p), "writing to pane %q", paneID)
}

// Paste inserts text into a pane as a paste rather than as keystrokes, so the
// pane's program decides how to read it.
func (s *TerminalsService) Paste(ctx context.Context, target, paneID string, p []byte) error {
	client, err := s.client(ctx, target)
	if err != nil {
		return err
	}
	return terminalError(client.Paste(ctx, paneID, p), "pasting into pane %q", paneID)
}

// SplitPane splits a pane of an attached session and returns the new pane's
// id. The new layout, and the new pane's first paint, follow on the stream.
func (s *TerminalsService) SplitPane(ctx context.Context, target, paneID string, direction tmuxcc.SplitDirection) (string, error) {
	client, err := s.client(ctx, target)
	if err != nil {
		return "", err
	}
	id, err := client.SplitPane(ctx, paneID, direction)
	if err != nil {
		return "", terminalError(err, "splitting pane %q", paneID)
	}
	return id, nil
}

// SelectPane selects paneID when direction is empty; otherwise it selects the
// neighbor in that direction.
func (s *TerminalsService) SelectPane(ctx context.Context, target, paneID string, direction tmuxcc.PaneDirection) error {
	client, err := s.client(ctx, target)
	if err != nil {
		return err
	}
	return terminalError(client.SelectPane(ctx, paneID, direction), "selecting pane %q", paneID)
}

// ClosePane kills one pane; the last pane of a window takes the window with it.
func (s *TerminalsService) ClosePane(ctx context.Context, target, paneID string) error {
	client, err := s.client(ctx, target)
	if err != nil {
		return err
	}
	return terminalError(client.KillPane(ctx, paneID), "closing pane %q", paneID)
}

// ResizePane sets width and/or height in cells; zero leaves that axis unchanged.
func (s *TerminalsService) ResizePane(ctx context.Context, target, paneID string, width, height int) error {
	client, err := s.client(ctx, target)
	if err != nil {
		return err
	}
	return terminalError(client.ResizePane(ctx, paneID, width, height), "resizing pane %q", paneID)
}

// ZoomPane toggles a pane between filling its window and its place in the
// layout.
func (s *TerminalsService) ZoomPane(ctx context.Context, target, paneID string) error {
	client, err := s.client(ctx, target)
	if err != nil {
		return err
	}
	return terminalError(client.ZoomPane(ctx, paneID), "zooming pane %q", paneID)
}

// Resize renegotiates the control client's size. Every client attached to a
// window renders the same grid, and tmux's window-size option decides whose
// size that is, so this is a vote rather than a resize.
func (s *TerminalsService) Resize(ctx context.Context, target string, cols, rows int) error {
	client, err := s.client(ctx, target)
	if err != nil {
		return err
	}
	return terminalError(client.Resize(ctx, cols, rows), "resizing session %q", target)
}

// ResizeWindow asks for one window's own grid, for two windows on screen in panes
// of different sizes. It is a vote like Resize.
func (s *TerminalsService) ResizeWindow(ctx context.Context, target, windowID string, cols, rows int) error {
	client, err := s.client(ctx, target)
	if err != nil {
		return err
	}
	return terminalError(client.ResizeWindow(ctx, windowID, cols, rows), "resizing window %q of session %q", windowID, target)
}

func (s *TerminalsService) SelectWindow(ctx context.Context, target, windowID string) error {
	client, err := s.client(ctx, target)
	if err != nil {
		return err
	}
	return terminalError(client.SelectWindow(ctx, windowID), "selecting window %q", windowID)
}

// NewWindow creates a window and returns its id; the tab set itself follows
// from the notification tmux sends afterwards.
//
// Attaching is not a precondition. A session's windows are its own, and the row
// offering a new one is offering it for the session rather than for what happens
// to be on screen — so a slug with no control client is served by a one-shot,
// and the attach that follows lists what it made. Either way the window opens
// where the session's active pane is, not where the session was started.
func (s *TerminalsService) NewWindow(ctx context.Context, target string) (windowID string, resultErr error) {
	s.log.Info().Str("session", target).Msg("terminal window requested")
	defer func() {
		if resultErr == nil {
			s.log.Info().Str("session", target).Str("window_id", windowID).Msg("terminal window created")
		}
	}()
	manager, slug, err := s.route(ctx, target)
	if err != nil {
		return "", err
	}
	client, ok := manager.Client(slug)
	if !ok {
		id, err := manager.NewWindow(ctx, slug)
		if err != nil {
			s.log.Warn().Str("slug", target).Str("path", "one-shot").Err(err).Msg("creating a window failed")
			if errors.Is(err, tmuxcc.ErrNotAttached) {
				return "", terminalError(err, "session %q is not running", target)
			}
			return "", terminalError(err, "creating a window in session %q", target)
		}
		return id, nil
	}
	id, err := client.NewWindow(ctx)
	if err != nil {
		s.log.Warn().Str("slug", target).Str("path", "control-client").Err(err).Msg("creating a window failed")
		return "", terminalError(err, "creating a window in session %q", target)
	}
	return id, nil
}

// WorkingDirectory is where a session's terminal currently is — the directory
// its active pane would print, not the one the session was started in. It is
// the core's answer to "here" for a caller that has a slug and needs a path:
// a launcher with no cwd of its own, and anything else opened against the
// terminal on screen.
//
// It is answered from tmux rather than from the session record on purpose. A
// slug in this view is not always a hive session — the scratch terminal and a
// pinned chat are tmux sessions with no checkout behind them — and a pane that
// has been cd'd somewhere else is where its user is, whichever kind it is. A
// slug tmux is not running is KindNotFound, which leaves the caller to decide
// what a terminal that is not up should fall back to.
func (s *TerminalsService) WorkingDirectory(ctx context.Context, target string) (string, error) {
	manager, slug, err := s.route(ctx, target)
	if err != nil {
		return "", err
	}
	dir, err := manager.CurrentPath(ctx, slug)
	if err != nil {
		return "", terminalError(err, "reading the working directory of session %q", target)
	}
	return dir, nil
}

func (s *TerminalsService) CloseWindow(ctx context.Context, target, windowID string) error {
	client, err := s.client(ctx, target)
	if err != nil {
		return err
	}
	return terminalError(client.CloseWindow(ctx, windowID), "closing window %q", windowID)
}

// WindowForeground is what a window has in front of it: whether it is running
// anything other than a prompt, and the name of what that is — empty when there
// is nothing running, or when the name could not be read.
type WindowForeground struct {
	Running bool
	Command string
}

// WindowForeground reports whether closing a window would kill work. Running is
// false only when every live pane in it is a shell sitting at its prompt;
// anything else — an agent, an editor, a script — answers true and names the
// process, so a caller can say what it is about to stop.
//
// Uncertainty answers true. A pane whose state cannot be read is one whose work
// this cannot account for, and the two ways of being wrong do not cost the
// same: a confirmation nobody needed against a process killed without one.
func (s *TerminalsService) WindowForeground(ctx context.Context, target, windowID string) (WindowForeground, error) {
	client, err := s.client(ctx, target)
	if err != nil {
		return WindowForeground{}, err
	}
	panes, err := client.ListPanes(ctx, windowID)
	if err != nil {
		return WindowForeground{}, terminalError(err, "reading what window %q is running", windowID)
	}
	return s.foregroundOf(ctx, target, panes), nil
}

// PaneForeground reports whether closing paneID would kill work.
func (s *TerminalsService) PaneForeground(ctx context.Context, target, paneID string) (WindowForeground, error) {
	client, err := s.client(ctx, target)
	if err != nil {
		return WindowForeground{}, err
	}
	window, err := client.PaneWindow(paneID)
	if err != nil {
		return WindowForeground{}, terminalError(err, "reading what pane %q is running", paneID)
	}
	panes, err := client.ListPanes(ctx, window.ID)
	if err != nil {
		return WindowForeground{}, terminalError(err, "reading what pane %q is running", paneID)
	}
	for _, pane := range panes {
		if pane.ID == paneID {
			return s.foregroundOf(ctx, target, []tmuxcc.Pane{pane}), nil
		}
	}
	// A pane the listing no longer holds has nothing left to kill.
	return WindowForeground{}, nil
}

// foregroundOf answers for the whole window, because closing one kills every
// pane in it. The active pane's process is the one named when several are
// running: it is the one the user is looking at.
func (s *TerminalsService) foregroundOf(ctx context.Context, target string, panes []tmuxcc.Pane) WindowForeground {
	answer := WindowForeground{}
	for _, pane := range panes {
		if pane.Dead || s.paneIsAtAPrompt(ctx, target, pane) {
			continue
		}
		if !answer.Running || pane.Active {
			answer = WindowForeground{Running: true, Command: pane.Command}
		}
	}
	return answer
}

// paneIsAtAPrompt reports the one state a pane can be closed from without
// asking: its foreground process is a shell, and that shell is the pane's own
// process rather than something it started.
//
// Both halves are load-bearing. tmux names the foreground process but not which
// process it is, so the name alone reads a running `#!/bin/bash` script as a
// prompt — it runs under the shell's own name. And the process check alone
// reads an agent as a prompt, because `sh -c claude` execs claude in place and
// leaves it as the pane's own process.
//
// A pane on a server never qualifies: its PID names a process over there, and
// the check reads this machine's process table.
func (s *TerminalsService) paneIsAtAPrompt(ctx context.Context, target string, pane tmuxcc.Pane) bool {
	if _, _, remote := splitTarget(target); remote || pane.PID <= 0 || !isShell(pane.Command) {
		return false
	}
	foreground, err := s.foreground(ctx, pane.PID)
	return err == nil && foreground
}

// shells are the interactive shells a pane sits in at a prompt. tmux reports a
// login shell without the leading dash argv[0] carries, but a pane whose
// command was spelled that way reaches us with it.
var shells = map[string]bool{
	"ash": true, "bash": true, "csh": true, "dash": true, "elvish": true,
	"fish": true, "ksh": true, "mksh": true, "nu": true, "pwsh": true,
	"sh": true, "tcsh": true, "xonsh": true, "zsh": true,
}

func isShell(command string) bool { return shells[strings.TrimPrefix(command, "-")] }

// processForeground reports whether pid holds the foreground process group of
// its controlling terminal — on a pane's own shell, whether it is waiting at a
// prompt rather than on something it started.
func processForeground(ctx context.Context, pid int) (bool, error) {
	proc, err := process.NewProcessWithContext(ctx, int32(pid))
	if err != nil {
		return false, err
	}
	return proc.ForegroundWithContext(ctx)
}

// MoveWindow moves a window to a position in the session's window order and
// answers with the order tmux settled on, so a caller renders what happened
// rather than what it asked for. Window order is tmux session state: the move
// reaches every other client attached to the same session.
func (s *TerminalsService) MoveWindow(ctx context.Context, target, windowID string, position int) ([]tmuxcc.Window, error) {
	client, err := s.client(ctx, target)
	if err != nil {
		return nil, err
	}
	windows, err := client.MoveWindow(ctx, windowID, position)
	if err != nil {
		return nil, terminalError(err, "moving window %q of session %q", windowID, target)
	}
	return windows, nil
}

func (s *TerminalsService) RenameWindow(ctx context.Context, target, windowID, name string) error {
	client, err := s.client(ctx, target)
	if err != nil {
		return err
	}
	return terminalError(client.RenameWindow(ctx, windowID, name), "renaming window %q", windowID)
}

// Detach closes slug's control client, leaving the tmux session itself alone.
// An unknown slug is not an error: detaching what is already gone succeeded.
func (s *TerminalsService) Detach(ctx context.Context, target string) error {
	manager, slug, err := s.route(ctx, target)
	if err != nil {
		return err
	}
	return terminalError(manager.Detach(ctx, slug), "detaching from session %q", target)
}

// ObserveFrameLatency records how long an output frame took from tmux decode to
// the moment a transport put it on the wire. Only the transport knows when the
// send happened, so it measures and reports it here.
func (s *TerminalsService) ObserveFrameLatency(ctx context.Context, latency time.Duration) {
	tmuxcc.ObserveFrameLatency(ctx, latency)
}

func (s *TerminalsService) client(ctx context.Context, target string) (*tmuxcc.Client, error) {
	manager, slug, err := s.route(ctx, target)
	if err != nil {
		return nil, err
	}
	client, ok := manager.Client(slug)
	if !ok {
		return nil, Errorf(KindNotFound, "no terminal is attached for session %q", target)
	}
	return client, nil
}

// route splits a terminal target into the manager that runs it and the tmux
// session name on that manager's server.
func (s *TerminalsService) route(ctx context.Context, target string) (*tmuxcc.Manager, string, error) {
	server, slug, remote := splitTarget(target)
	if !remote {
		return s.manager, slug, nil
	}
	if slug == "" {
		return nil, "", Errorf(KindInvalid, "terminal %q names a server but no session", target)
	}
	manager, err := s.serverManager(ctx, server)
	if err != nil {
		return nil, "", err
	}
	return manager, slug, nil
}

// splitTarget reads "<server>:<slug>" as a session on that server and anything
// else as a local slug. tmux allows no ":" in a session name and server names
// refuse one, so the two forms cannot collide.
func splitTarget(target string) (server, slug string, remote bool) {
	server, slug, remote = strings.Cut(target, ":")
	if !remote {
		return "", target, false
	}
	return server, slug, true
}

func (s *TerminalsService) serverManager(ctx context.Context, name string) (*tmuxcc.Manager, error) {
	if s.servers == nil {
		return nil, Errorf(KindUnavailable, "terminals on server %q are not available here", name)
	}
	return s.servers.Manager(ctx, name)
}

// terminalError classifies a tmuxcc failure by sentinel rather than by message.
// A residual *tmuxcc.CommandError — tmux refused a command we had already
// validated — is ours, not the caller's, so it falls through to internal.
func terminalError(err error, format string, args ...any) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, tmuxcc.ErrLaunchPending):
		return &Error{Kind: KindConflict, Msg: fmt.Sprintf(format, args...) + ": " + err.Error(), Err: err}
	case errors.Is(err, tmuxcc.ErrUnavailable):
		return Wrap(err, KindUnavailable, format, args...)
	case errors.Is(err, tmuxcc.ErrInvalidSize), errors.Is(err, tmuxcc.ErrInvalidName),
		errors.Is(err, tmuxcc.ErrInvalidPosition), errors.Is(err, tmuxcc.ErrInvalidDirection):
		return Wrap(err, KindInvalid, format, args...)
	case errors.Is(err, tmuxcc.ErrNotAttached), errors.Is(err, tmuxcc.ErrUnknownWindow), errors.Is(err, tmuxcc.ErrUnknownPane):
		return Wrap(err, KindNotFound, format, args...)
	default:
		return Wrap(err, KindInternal, format, args...)
	}
}
