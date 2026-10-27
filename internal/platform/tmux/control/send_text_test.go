//go:build !server

package tmuxcc

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/platform/tmuxtest"
	"github.com/colonyops/hive/pkg/executil"
)

type fakeSend struct {
	calls     [][]string
	stdin     string
	pasteErr  error
	listPanes []string
}

func (f *fakeSend) run(_ context.Context, _ string, _ []string, args ...string) ([]string, error) {
	f.calls = append(f.calls, args)
	switch args[0] {
	case "paste-buffer":
		return nil, f.pasteErr
	case "list-panes":
		return f.listPanes, nil
	}
	return nil, nil
}

func (f *fakeSend) input(_ context.Context, _ string, _ []string, stdin io.Reader, args ...string) error {
	f.calls = append(f.calls, args)
	b, err := io.ReadAll(stdin)
	f.stdin = string(b)
	return err
}

func TestManagerSendTextPastesThenSubmits(t *testing.T) {
	t.Parallel()

	f := &fakeSend{}
	m := newTestManager(t, nil, ManagerOptions{runTmux: f.run, inputTmux: f.input})

	require.NoError(t, m.SendText(t.Context(), "%3", []byte("line1\nline2"), 0))

	require.Equal(t, "line1\nline2", f.stdin)
	require.Len(t, f.calls, 3)
	buffer := f.calls[0][2]
	require.True(t, strings.HasPrefix(buffer, "hive-send-"))
	require.Equal(t, []string{"load-buffer", "-b", buffer, "-"}, f.calls[0])
	require.Equal(t, []string{"paste-buffer", "-d", "-p", "-b", buffer, "-t", "%3"}, f.calls[1])
	require.Equal(t, []string{"send-keys", "-t", "%3", "Enter"}, f.calls[2])
}

func TestManagerSendTextDeletesTheBufferWhenThePasteFails(t *testing.T) {
	t.Parallel()

	f := &fakeSend{pasteErr: errors.New("can't find pane: %9")}
	m := newTestManager(t, nil, ManagerOptions{runTmux: f.run, inputTmux: f.input})

	err := m.SendText(t.Context(), "%9", []byte("hi"), 0)
	require.ErrorContains(t, err, "can't find pane")
	require.Len(t, f.calls, 3)
	require.Equal(t, []string{"delete-buffer", "-b", f.calls[0][2]}, f.calls[2], "no Enter after a failed paste")
}

func TestManagerSendTextRejectsANonPaneTarget(t *testing.T) {
	t.Parallel()

	f := &fakeSend{}
	m := newTestManager(t, nil, ManagerOptions{runTmux: f.run, inputTmux: f.input})

	require.Error(t, m.SendText(t.Context(), "hive-demo:0", []byte("hi"), 0))
	require.Empty(t, f.calls)
}

func TestManagerFirstPanePicksTheLowestWindowAndPane(t *testing.T) {
	t.Parallel()

	f := &fakeSend{listPanes: []string{"1 0 %7", "0 1 %5", "0 0 %2"}}
	m := newTestManager(t, nil, ManagerOptions{runTmux: f.run})

	pane, err := m.FirstPane(t.Context(), "agentws-x")
	require.NoError(t, err)
	require.Equal(t, "%2", pane)
	require.Equal(t, []string{"list-panes", "-s", "-t", "=agentws-x", "-F", "#{window_index} #{pane_index} #{pane_id}"}, f.calls[0])
}

func TestManagerSendTextRealTmux(t *testing.T) {
	socket := tmuxtest.Private(t)
	tmux := func(args ...string) string {
		cmd := exec.Command("tmux", append([]string{"-S", socket}, args...)...) //nolint:noctx // fixture commands
		cmd.Env = executil.WithoutEnv(nil, "TMUX", "TMUX_PANE")
		out, err := cmd.Output()
		require.NoError(t, err, "tmux %v", args)
		return strings.TrimSpace(string(out))
	}

	tmux("new-session", "-d", "-s", "send", "-x", "120", "-y", "20", `stty -echo; printf '\033[?2004h'; exec cat -v`)
	pane := tmux("display-message", "-p", "-t", "=send:", "#{pane_id}")

	m := NewManager(t.Context(), ManagerOptions{Logger: zerolog.Nop()})
	t.Cleanup(func() { _ = m.Stop(context.WithoutCancel(t.Context())) })

	require.Eventually(t, func() bool {
		return m.SendText(t.Context(), pane, []byte("line1\nline2"), 100*time.Millisecond) == nil
	}, 5*time.Second, 200*time.Millisecond)

	require.Eventually(t, func() bool {
		screen := tmux("capture-pane", "-p", "-t", pane)
		return strings.Contains(screen, "^[[200~line1") && strings.Contains(screen, "line2^[[201~")
	}, 5*time.Second, 100*time.Millisecond)
	require.Empty(t, sendBuffers(tmux("list-buffers", "-F", "#{buffer_name}")))

	require.Error(t, m.SendText(t.Context(), "%999", []byte("lost"), 0))
	require.Empty(t, sendBuffers(tmux("list-buffers", "-F", "#{buffer_name}")))

	tmux("split-window", "-t", "=send:")
	first, err := m.FirstPane(t.Context(), "send")
	require.NoError(t, err)
	require.Equal(t, pane, first)
	screen, err := m.CapturePaneID(t.Context(), first)
	require.NoError(t, err)
	require.Contains(t, screen, "^[[200~line1", "the first pane, not the split that is now active")
}

func sendBuffers(names string) []string {
	var found []string
	for name := range strings.FieldsSeq(names) {
		if strings.HasPrefix(name, "hive-send-") {
			found = append(found, name)
		}
	}
	return found
}
