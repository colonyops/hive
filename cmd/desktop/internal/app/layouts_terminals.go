package app

import (
	"context"
	"regexp"
	"strings"

	tmuxcc "github.com/colonyops/hive/internal/platform/tmux/control"
)

// sessionTerminals is what the tab tools need of tmux beside a Code session.
// slug is the session's tmux session name.
type sessionTerminals interface {
	HasSession(ctx context.Context, slug string) (bool, error)
	// NewWindow starts a detached login shell in dir and answers the window's id.
	NewWindow(ctx context.Context, slug, dir, name string) (string, error)
	// FindWindow answers the id of the session's window that idOrName names:
	// a tmux id (@3) or an exact window name. A miss is KindNotFound.
	FindWindow(ctx context.Context, slug, idOrName string) (string, error)
	// WindowNames answers the name of each of the session's windows by id.
	WindowNames(ctx context.Context, slug string) (map[string]string, error)
	// CloseWindow kills a window and everything running in it.
	CloseWindow(ctx context.Context, slug, windowID string) error
}

// tmuxWindowSource is the part of tmuxcc.Manager the tab tools drive.
type tmuxWindowSource interface {
	HasSession(ctx context.Context, slug string) (bool, error)
	ListAllWindows(ctx context.Context, slugs []string) (map[string][]tmuxcc.Window, error)
	NewDetachedWindow(ctx context.Context, slug, dir, name string) (string, error)
	KillWindow(ctx context.Context, slug, windowID string) error
}

var windowIDPattern = regexp.MustCompile(`^@[0-9]+$`)

// tmuxWindows resolves the windows an agent names, by id or exact name, over
// the engine's tmux manager.
type tmuxWindows struct {
	tmux tmuxWindowSource
}

func newTmuxWindows(tmux tmuxWindowSource) *tmuxWindows {
	return &tmuxWindows{tmux: tmux}
}

func (t *tmuxWindows) HasSession(ctx context.Context, slug string) (bool, error) {
	return t.tmux.HasSession(ctx, slug)
}

func (t *tmuxWindows) NewWindow(ctx context.Context, slug, dir, name string) (string, error) {
	return t.tmux.NewDetachedWindow(ctx, slug, dir, name)
}

func (t *tmuxWindows) CloseWindow(ctx context.Context, slug, windowID string) error {
	return t.tmux.KillWindow(ctx, slug, windowID)
}

func (t *tmuxWindows) listWindows(ctx context.Context, slug string) ([]tmuxcc.Window, error) {
	all, err := t.tmux.ListAllWindows(ctx, []string{slug})
	if err != nil {
		return nil, terminalError(err, "listing the windows of session %q", slug)
	}
	return all[slug], nil
}

func (t *tmuxWindows) WindowNames(ctx context.Context, slug string) (map[string]string, error) {
	windows, err := t.listWindows(ctx, slug)
	if err != nil {
		return nil, err
	}
	names := make(map[string]string, len(windows))
	for _, w := range windows {
		names[w.ID] = w.Name
	}
	return names, nil
}

// FindWindow matches an id against ids only, so a window named like an id
// cannot make the id ambiguous.
func (t *tmuxWindows) FindWindow(ctx context.Context, slug, idOrName string) (string, error) {
	windows, err := t.listWindows(ctx, slug)
	if err != nil {
		return "", err
	}
	byID := windowIDPattern.MatchString(idOrName)
	var matches []tmuxcc.Window
	for _, w := range windows {
		if (byID && w.ID == idOrName) || (!byID && w.Name == idOrName) {
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
