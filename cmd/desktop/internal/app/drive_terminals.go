package app

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	tmuxcc "github.com/colonyops/hive/internal/platform/tmux/control"
	tmuxexec "github.com/colonyops/hive/internal/platform/tmux/exec"
)

// sessionTerminals is what open_tab needs of tmux to place a terminal beside a
// Code session. slug is the session's tmux session name.
type sessionTerminals interface {
	HasSession(ctx context.Context, slug string) (bool, error)
	// NewWindow starts a detached window in dir and answers its id. A command
	// runs first and leaves the window as a shell.
	NewWindow(ctx context.Context, slug, dir, name, command string) (string, error)
	// FindWindow answers the id of the session's window that idOrName names:
	// a tmux id (@3) or an exact window name. A miss is KindNotFound.
	FindWindow(ctx context.Context, slug, idOrName string) (string, error)
}

var windowIDPattern = regexp.MustCompile(`^@[0-9]+$`)

// tmuxWindows runs open_tab's window verbs on the engine's tmux. The runner is
// the one the engine's own tmux client uses, so the configured binary and the
// resolved environment apply.
type tmuxWindows struct {
	runner      tmuxexec.Runner
	hasSession  func(ctx context.Context, slug string) (bool, error)
	listWindows func(ctx context.Context, slug string) ([]tmuxcc.Window, error)
}

func newTmuxWindows(manager *tmuxcc.Manager, runner tmuxexec.Runner) *tmuxWindows {
	return &tmuxWindows{
		runner:     runner,
		hasSession: manager.HasSession,
		listWindows: func(ctx context.Context, slug string) ([]tmuxcc.Window, error) {
			all, err := manager.ListAllWindows(ctx, []string{slug})
			return all[slug], err
		},
	}
}

func (t *tmuxWindows) HasSession(ctx context.Context, slug string) (bool, error) {
	return t.hasSession(ctx, slug)
}

func (t *tmuxWindows) NewWindow(ctx context.Context, slug, dir, name, command string) (string, error) {
	stdout, stderr, err := t.runner.Capture(ctx, newWindowArgs(slug, dir, name, command)...)
	if err != nil {
		return "", fmt.Errorf("tmux new-window in %s: %w: %s", slug, err, strings.TrimSpace(string(stderr)))
	}
	id := strings.TrimSpace(string(stdout))
	if !windowIDPattern.MatchString(id) {
		return "", fmt.Errorf("tmux new-window in %s returned %q, not a window id", slug, id)
	}
	return id, nil
}

// newWindowArgs is detached (-d) so tmux does not make the new window the
// session's active one: a control client follows the active window, and an
// agent's request must not move the screen from under the user. -c is spelled
// out because tmux resolves a missing directory against the calling process.
//
// A command runs through sh -c, as hive's spawn runs a window command, and ends
// with exec of the user's shell so the window outlives the command and its
// output stays readable. A newline, not ";", separates them so a trailing
// comment or "&" in the command cannot swallow the exec.
func newWindowArgs(slug, dir, name, command string) []string {
	args := []string{"new-window", "-d", "-t", "=" + slug + ":", "-c", dir}
	if name != "" {
		args = append(args, "-n", name)
	}
	args = append(args, "-P", "-F", "#{window_id}")
	if command != "" {
		args = append(args, "--", "sh", "-c", command+"\n"+`exec "${SHELL:-sh}" -l`)
	}
	return args
}

func (t *tmuxWindows) FindWindow(ctx context.Context, slug, idOrName string) (string, error) {
	windows, err := t.listWindows(ctx, slug)
	if err != nil {
		return "", terminalError(err, "listing the windows of session %q", slug)
	}
	var matches []tmuxcc.Window
	for _, w := range windows {
		if w.ID == idOrName || w.Name == idOrName {
			matches = append(matches, w)
		}
	}
	switch len(matches) {
	case 0:
		return "", Errorf(KindNotFound, "the session has no window %q; name one of its windows by id (@3) or exact name", idOrName)
	case 1:
		return matches[0].ID, nil
	}
	ids := make([]string, len(matches))
	for i, w := range matches {
		ids[i] = w.ID
	}
	return "", Errorf(KindInvalid, "%d windows are named %q (%s); name one by id", len(matches), idOrName, strings.Join(ids, ", "))
}
