package app

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	tmuxcc "github.com/colonyops/hive/internal/platform/tmux/control"
)

type fakeTabWindows struct {
	windows []tmuxcc.Window
	listErr error
	calls   []string
}

func (f *fakeTabWindows) HasSession(context.Context, string) (bool, error) { return true, nil }

func (f *fakeTabWindows) ListAllWindows(_ context.Context, slugs []string) (map[string][]tmuxcc.Window, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return map[string][]tmuxcc.Window{slugs[0]: f.windows}, nil
}

func (f *fakeTabWindows) NewDetachedWindow(_ context.Context, slug, dir, name string) (string, error) {
	f.calls = append(f.calls, "new "+slug+" "+dir+" "+name)
	return "@7", nil
}

func (f *fakeTabWindows) KillWindow(_ context.Context, slug, windowID string) error {
	f.calls = append(f.calls, "kill "+slug+" "+windowID)
	return nil
}

func TestTmuxWindowsDelegateToTheManager(t *testing.T) {
	src := &fakeTabWindows{}
	w := newTmuxWindows(src)

	id, err := w.NewWindow(t.Context(), "fix-login", "/work/site", "tests")
	require.NoError(t, err)
	assert.Equal(t, "@7", id)
	require.NoError(t, w.CloseWindow(t.Context(), "fix-login", "@7"))
	assert.Equal(t, []string{"new fix-login /work/site tests", "kill fix-login @7"}, src.calls)
}

func TestTmuxWindowsFindWindow(t *testing.T) {
	src := &fakeTabWindows{windows: []tmuxcc.Window{
		{ID: "@1", Name: "claude"}, {ID: "@3", Name: "logs"}, {ID: "@5", Name: "dup"}, {ID: "@6", Name: "dup"}, {ID: "@8", Name: "@3"},
	}}
	w := newTmuxWindows(src)

	for name, tc := range map[string]struct{ in, want string }{
		"by id":                         {"@3", "@3"},
		"by name":                       {"logs", "@3"},
		"an id, with a window so named": {"@8", "@8"},
	} {
		got, err := w.FindWindow(t.Context(), "s", tc.in)
		require.NoError(t, err, name)
		assert.Equal(t, tc.want, got, name)
	}

	_, err := w.FindWindow(t.Context(), "s", "ghost")
	assert.Equal(t, KindNotFound, KindOf(err))
	_, err = w.FindWindow(t.Context(), "s", "dup")
	assert.Equal(t, KindInvalid, KindOf(err), "a name two windows share")
	_, err = w.FindWindow(t.Context(), "s", "log")
	assert.Equal(t, KindNotFound, KindOf(err), "names match exactly")

	src.listErr = errors.New("lost server")
	_, err = w.FindWindow(t.Context(), "s", "@3")
	assert.Equal(t, KindInternal, KindOf(err), "a failed listing is not a missing window")
}
