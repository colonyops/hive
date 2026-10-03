package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/actions"
	"github.com/colonyops/hive/cmd/desktop/internal/app/ptyterm"
	"github.com/colonyops/hive/internal/domain/session"
)

// checkoutHarness is a real engine holding reviewSession with its checkout at
// path.
func checkoutHarness(t *testing.T, path string, state session.State) *hiveHarness {
	t.Helper()
	h := newHiveHarness(t, engineOptions{})
	sess := reviewSession()
	sess.Path, sess.State = path, state
	h.save(t, sess)
	return h
}

func newPopupHarness(t *testing.T, h *hiveHarness) *PopupTerminalsService {
	t.Helper()
	return newPopupHarnessWithCatalog(t, h, nil)
}

func newPopupHarnessWithCatalog(t *testing.T, h *hiveHarness, catalog *actions.ActionStore) *PopupTerminalsService {
	t.Helper()
	return newPopupHarnessIn(t, h, catalog, &fakeTerminalDirs{})
}

func newPopupHarnessIn(t *testing.T, h *hiveHarness, catalog *actions.ActionStore, terminals terminalWorkingDirectory) *PopupTerminalsService {
	t.Helper()
	sessions := newSessionsService(SessionsDeps{Hive: h.engine, Tmux: &fakeSessionTmux{}, Jobs: &fakeJobRunner{}})
	pty := ptyterm.NewManager(ptyterm.ManagerOptions{Shell: []string{"/bin/sh"}})
	t.Cleanup(func() { _ = pty.Stop(t.Context()) })
	return newPopupTerminalsService(PopupTerminalsDeps{Manager: pty, Terminals: terminals, Directory: sessions, Catalog: catalog})
}

// fakeTerminalDirs stands in for tmux: a slug it is holding answers with the
// directory that terminal's pane is in, and one it is not holding is a terminal
// that is not running.
type fakeTerminalDirs struct {
	dirs map[string]string
}

func (f *fakeTerminalDirs) WorkingDirectory(_ context.Context, slug string) (string, error) {
	if dir, ok := f.dirs[slug]; ok {
		return dir, nil
	}
	return "", Errorf(KindNotFound, "session %q is not running", slug)
}

// popupCatalog writes an actions.yml and returns a store over it, so the
// launcher tests below go through the real parser rather than a hand-built
// Action the loader would have rejected.
func popupCatalog(t *testing.T, yaml string) *actions.ActionStore {
	t.Helper()
	path := filepath.Join(t.TempDir(), "actions.yml")
	require.NoError(t, os.WriteFile(path, []byte(yaml), 0o600))
	store := actions.NewActionStore(path)
	require.NoError(t, store.Reload())
	return store
}

// A slug whose terminal is not running still resolves: the session's checkout
// is where the pop-up opens, which is what keeps a launch working from a row
// whose tmux session has not been started yet.
func TestPopupTerminalsService_OpensInTheSessionCheckout(t *testing.T) {
	checkout := t.TempDir()
	h := checkoutHarness(t, checkout, session.StateActive)
	svc := newPopupHarness(t, h)

	term, err := svc.Open(t.Context(), OpenPopupTerminal{SessionSlug: "review-81"})
	require.NoError(t, err)
	require.Equal(t, checkout, term.Dir)

	// Every open is its own terminal: there is nothing keyed on the session, so
	// a second pop-up over the same checkout is a second shell.
	second, err := svc.Open(t.Context(), OpenPopupTerminal{SessionSlug: "review-81"})
	require.NoError(t, err)
	require.NotEqual(t, term.ID, second.ID)

	open, err := svc.List(t.Context())
	require.NoError(t, err)
	require.Len(t, open, 2)
}

// The resolution order is the contract: a slug wins over a path, and a caller
// with neither still lands somewhere a shell makes sense.
func TestPopupTerminalsService_ResolvesTheDirectoryInOrder(t *testing.T) {
	h := activeHarness(t)
	svc := newPopupHarness(t, h)
	dir := t.TempDir()

	explicit, err := svc.Open(t.Context(), OpenPopupTerminal{Dir: dir})
	require.NoError(t, err)
	require.Equal(t, dir, explicit.Dir)

	home, err := os.UserHomeDir()
	require.NoError(t, err)
	fallback, err := svc.Open(t.Context(), OpenPopupTerminal{})
	require.NoError(t, err)
	require.Equal(t, home, fallback.Dir)
}

// A recycled session has no checkout left, so there is nowhere to open one. The
// tmux backend refuses the same case for the same reason.
func TestPopupTerminalsService_RefusesASessionWithNoCheckout(t *testing.T) {
	h := checkoutHarness(t, t.TempDir(), session.StateRecycled)
	svc := newPopupHarness(t, h)

	_, err := svc.Open(t.Context(), OpenPopupTerminal{SessionSlug: "review-81"})
	require.Equal(t, KindConflict, KindOf(err))
}

const launcherCatalogYAML = `version: 1
actions:
  - id: run-tests
    label: Run tests
    type: shell
    targets: [session]
    command_template: 'mise run test'
launchers:
  - id: lazygit
    label: lazygit
    icon: git-branch
    command: lazygit
  - id: dotfiles
    label: Edit dotfiles
    cwd: "~"
    command: $EDITOR .
`

// A launcher without a cwd opens where its terminal is, which is what makes one
// shortcut mean "lazygit here" wherever you are (ADR a-new-tab-and-a-launcher-open-where-the-terminal-s-active-pane-is).
func TestPopupTerminalsService_LauncherOpensWhereTheTerminalIs(t *testing.T) {
	checkout := t.TempDir()
	elsewhere := t.TempDir()
	h := checkoutHarness(t, checkout, session.StateActive)
	terminals := &fakeTerminalDirs{dirs: map[string]string{"review-81": elsewhere}}
	svc := newPopupHarnessIn(t, h, popupCatalog(t, launcherCatalogYAML), terminals)

	term, err := svc.Open(t.Context(), OpenPopupTerminal{Launcher: "lazygit", SessionSlug: "review-81"})
	require.NoError(t, err)
	require.Equal(t, elsewhere, term.Dir, "the pane the user is looking at, not the checkout it started in")
	require.Equal(t, "lazygit", term.Command)
}

// The scratch terminal and a pinned chat are tmux sessions with no hive record,
// and a launcher works on them for the same reason it follows a cd: the
// directory comes from tmux, which knows all three the same way.
func TestPopupTerminalsService_LauncherOpensOnATerminalHiveKnowsNothingAbout(t *testing.T) {
	h := activeHarness(t)
	dir := t.TempDir()
	terminals := &fakeTerminalDirs{dirs: map[string]string{ScratchSlug: dir}}
	svc := newPopupHarnessIn(t, h, popupCatalog(t, launcherCatalogYAML), terminals)

	term, err := svc.Open(t.Context(), OpenPopupTerminal{Launcher: "lazygit", SessionSlug: ScratchSlug})
	require.NoError(t, err)
	require.Equal(t, dir, term.Dir)
	require.Equal(t, "lazygit", term.Command)
}

// A terminal that is not running has no pane to read, and a hive session still
// has its checkout — so a launcher fired at a stopped session opens there
// rather than refusing.
func TestPopupTerminalsService_LauncherFallsBackToTheCheckout(t *testing.T) {
	checkout := t.TempDir()
	h := checkoutHarness(t, checkout, session.StateActive)
	svc := newPopupHarnessWithCatalog(t, h, popupCatalog(t, launcherCatalogYAML))

	term, err := svc.Open(t.Context(), OpenPopupTerminal{Launcher: "lazygit", SessionSlug: "review-81"})
	require.NoError(t, err)
	require.Equal(t, checkout, term.Dir)
}

// The launch a session-scoped launcher cannot serve, refused here rather than
// opened in the home directory: `lazygit` with no repository under it starts
// fine and fails immediately, which is the whole bug (ADR quick-terminal-launchers-are-session-scoped).
func TestPopupTerminalsService_LauncherWithoutATerminalIsRefused(t *testing.T) {
	h := checkoutHarness(t, t.TempDir(), session.StateActive)
	svc := newPopupHarnessWithCatalog(t, h, popupCatalog(t, launcherCatalogYAML))

	_, err := svc.Open(t.Context(), OpenPopupTerminal{Launcher: "lazygit"})
	require.Equal(t, KindInvalid, KindOf(err), "no slug is a refusal, not the home directory")

	// A caller cannot answer the session question with a path instead: the
	// directory a session-scoped launcher opens in is the session's.
	_, err = svc.Open(t.Context(), OpenPopupTerminal{Launcher: "lazygit", Dir: t.TempDir()})
	require.Equal(t, KindInvalid, KindOf(err))

	// A slug that names neither a running terminal nor a session is gone, and
	// the answer is that rather than a terminal somewhere else.
	_, err = svc.Open(t.Context(), OpenPopupTerminal{Launcher: "lazygit", SessionSlug: "deleted-yesterday"})
	require.Equal(t, KindNotFound, KindOf(err))

	open, err := svc.List(t.Context())
	require.NoError(t, err)
	require.Empty(t, open, "a refused launch spawns nothing")
}

// A configured cwd pins the launcher, and beats the session the caller was
// looking at when they pressed the key. It is also what makes one reachable
// with no session at all: the directory it needs is in the catalog.
func TestPopupTerminalsService_LauncherCwdWinsOverTheSession(t *testing.T) {
	h := checkoutHarness(t, t.TempDir(), session.StateActive)
	svc := newPopupHarnessWithCatalog(t, h, popupCatalog(t, launcherCatalogYAML))

	home, err := os.UserHomeDir()
	require.NoError(t, err)

	term, err := svc.Open(t.Context(), OpenPopupTerminal{Launcher: "dotfiles", SessionSlug: "review-81"})
	require.NoError(t, err)
	require.Equal(t, home, term.Dir, "a leading ~ is expanded: chdir takes a path, not a shell word")
	require.Equal(t, "$EDITOR .", term.Command)

	pinned, err := svc.Open(t.Context(), OpenPopupTerminal{Launcher: "dotfiles"})
	require.NoError(t, err)
	require.Equal(t, home, pinned.Dir)
}

func TestPopupTerminalsService_ListsLaunchersInCatalogOrder(t *testing.T) {
	h := activeHarness(t)
	svc := newPopupHarnessWithCatalog(t, h, popupCatalog(t, launcherCatalogYAML))

	launchers, err := svc.Launchers(t.Context())
	require.NoError(t, err)
	require.Equal(t, []PopupLauncher{
		// A configured cwd is the difference between the two, so it is what the
		// menu is told: one needs a terminal, the other carries its own directory.
		{ID: "lazygit", Label: "lazygit", Icon: "git-branch", RequiresSession: true},
		{ID: "dotfiles", Label: "Edit dotfiles"},
	}, launchers, "the launchers list, in file order; the actions beside it are not launchers")
}

func TestPopupTerminalsService_RefusesALaunchThatIsNotOne(t *testing.T) {
	h := activeHarness(t)
	svc := newPopupHarnessWithCatalog(t, h, popupCatalog(t, launcherCatalogYAML))

	_, err := svc.Open(t.Context(), OpenPopupTerminal{Launcher: "nope"})
	require.Equal(t, KindNotFound, KindOf(err))

	// An action id is not a launcher id: the lists are separate namespaces, so
	// naming an action here finds nothing rather than opening one.
	_, err = svc.Open(t.Context(), OpenPopupTerminal{Launcher: "run-tests"})
	require.Equal(t, KindNotFound, KindOf(err))

	// What a launcher runs is the catalog's answer, so a caller cannot send an
	// id and a command line and have both honoured.
	_, err = svc.Open(t.Context(), OpenPopupTerminal{Launcher: "lazygit", Command: "rm -rf /"})
	require.Equal(t, KindInvalid, KindOf(err))
}

func TestPopupTerminalsService_ClassifiesFailures(t *testing.T) {
	h := activeHarness(t)
	svc := newPopupHarness(t, h)

	_, err := svc.Open(t.Context(), OpenPopupTerminal{SessionSlug: "no-such-session"})
	require.Equal(t, KindNotFound, KindOf(err))

	_, err = svc.Open(t.Context(), OpenPopupTerminal{Dir: t.TempDir() + "/gone"})
	require.Equal(t, KindInvalid, KindOf(err), "a directory that cannot be entered is the caller's error")

	require.Equal(t, KindNotFound, KindOf(svc.Resize(t.Context(), "t99", 80, 24)))

	// Closing a terminal that is already gone is success with nothing done, so a
	// caller that saw the exit notice does not have to special-case the race.
	closed, err := svc.Close(t.Context(), "t99")
	require.NoError(t, err)
	require.False(t, closed)
}
