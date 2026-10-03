package tmux

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/colonyops/hive/internal/domain/multiplexer"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type runnerCall struct {
	mode  string
	args  []string
	input string
}

type runnerResult struct {
	stdout string
	stderr string
	err    error
}

type fakeRunner struct {
	available bool
	calls     []runnerCall
	results   []runnerResult
}

func (r *fakeRunner) Available() bool { return r.available }

func (r *fakeRunner) Capture(_ context.Context, args ...string) ([]byte, []byte, error) {
	r.calls = append(r.calls, runnerCall{mode: "capture", args: append([]string(nil), args...)})
	return r.next()
}

func (r *fakeRunner) Input(_ context.Context, input io.Reader, args ...string) ([]byte, []byte, error) {
	data, err := io.ReadAll(input)
	if err != nil {
		return nil, nil, err
	}
	r.calls = append(r.calls, runnerCall{mode: "input", args: append([]string(nil), args...), input: string(data)})
	return r.next()
}

func (r *fakeRunner) Interactive(_ context.Context, _ multiplexer.AttachStreams, args ...string) error {
	r.calls = append(r.calls, runnerCall{mode: "interactive", args: append([]string(nil), args...)})
	_, _, err := r.next()
	return err
}

func (r *fakeRunner) next() ([]byte, []byte, error) {
	if len(r.results) == 0 {
		return nil, nil, nil
	}
	result := r.results[0]
	r.results = r.results[1:]
	return []byte(result.stdout), []byte(result.stderr), result.err
}

func TestRenderTargets(t *testing.T) {
	session, err := renderSessionTarget(multiplexer.Target{Session: "work"})
	require.NoError(t, err)
	assert.Equal(t, "work", session)

	window, err := renderWindowTarget(multiplexer.Target{Session: "work", Window: "2"})
	require.NoError(t, err)
	assert.Equal(t, "work:2", window)

	pane, err := renderPaneTarget(multiplexer.Target{Session: "work", Window: "2", Pane: "1"})
	require.NoError(t, err)
	assert.Equal(t, "work:2.1", pane)
}

func TestListPanesParsesEscapedFreeFormFields(t *testing.T) {
	runner := &fakeRunner{results: []runnerResult{{stdout: `sess|||2|||1|||win\|\|\|name|||/tmp/a\|\|\|b|||42|||%7|||123|||title\|\|\|text|||hive-slug|||1
`}}}
	client := New(runner, zerolog.Nop())

	panes, err := client.ListPanes(context.Background())
	require.NoError(t, err)
	require.Len(t, panes, 1)
	assert.Equal(t, multiplexer.Target{Session: "sess", Window: "2", Pane: "1"}, panes[0].Target)
	assert.Equal(t, "win|||name", panes[0].WindowName)
	assert.Equal(t, "/tmp/a|||b", panes[0].WorkingDirectory)
	assert.Equal(t, "title|||text", panes[0].Title)
	assert.Equal(t, "%7", panes[0].NativeID)
	assert.True(t, panes[0].InMode)
}

func TestListPanesRejectsMalformedRow(t *testing.T) {
	runner := &fakeRunner{results: []runnerResult{{stdout: "bad|||row\n"}}}
	_, err := New(runner, zerolog.Nop()).ListPanes(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected 11 fields")
}

func TestSendLiteralAndNamedKeyUseSingleArgvTokens(t *testing.T) {
	runner := &fakeRunner{}
	client := New(runner, zerolog.Nop())
	target := multiplexer.Target{Session: "s", Window: "0", Pane: "1"}

	require.NoError(t, client.SendLiteral(context.Background(), target, `-- "x"; λ`))
	key, err := multiplexer.NewNamedKey("C-c")
	require.NoError(t, err)
	require.NoError(t, client.SendKey(context.Background(), target, key))

	assert.Equal(t, []string{"send-keys", "-t", "s:0.1", "-l", "--", `-- "x"; λ`}, runner.calls[0].args)
	assert.Equal(t, []string{"send-keys", "-t", "s:0.1", "--", "C-c"}, runner.calls[1].args)
}

func TestPasteSuccessUsesBytePreservingFlagsAndReliesOnDeleteAfterPaste(t *testing.T) {
	runner := &fakeRunner{}
	client := New(runner, zerolog.Nop())
	target := multiplexer.Target{Session: "s", Window: "0", Pane: "1"}

	require.NoError(t, client.Paste(context.Background(), target, []byte("a\nb\tλ"), multiplexer.PasteOptions{}))
	require.Len(t, runner.calls, 2)
	assert.Equal(t, "a\nb\tλ", runner.calls[0].input)
	assert.Contains(t, runner.calls[1].args, "-d")
	assert.Contains(t, runner.calls[1].args, "-r")
	assert.Contains(t, runner.calls[1].args, "-S")
	assert.NotContains(t, runner.calls[1].args, "-p")
}

func TestPasteRetriesWithoutSanitizationFlagForPreSanitizationTmux(t *testing.T) {
	runner := &fakeRunner{results: []runnerResult{{}, {stderr: "command paste-buffer: unknown flag -S", err: assert.AnError}, {}}}
	client := New(runner, zerolog.Nop())
	target := multiplexer.Target{Session: "s", Window: "0", Pane: "1"}

	require.NoError(t, client.Paste(context.Background(), target, []byte("data"), multiplexer.PasteOptions{}))
	require.Len(t, runner.calls, 3)
	assert.Contains(t, runner.calls[1].args, "-S")
	assert.NotContains(t, runner.calls[2].args, "-S")
	assert.Contains(t, runner.calls[2].args, "-r")
}

func TestPasteLoadFailureDoesNotAttemptCleanup(t *testing.T) {
	loadErr := errors.New("load failed")
	runner := &fakeRunner{results: []runnerResult{{err: loadErr}}}
	client := New(runner, zerolog.Nop())
	target := multiplexer.Target{Session: "s", Window: "0", Pane: "1"}

	err := client.Paste(context.Background(), target, []byte("data"), multiplexer.PasteOptions{})
	require.ErrorIs(t, err, loadErr)
	require.Len(t, runner.calls, 1)
	assert.Equal(t, "input", runner.calls[0].mode)
}

func TestPasteCallerCancellationStillAttemptsCleanup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runner := &fakeRunner{results: []runnerResult{{}, {err: context.Canceled}, {}}}
	client := New(runner, zerolog.Nop())
	target := multiplexer.Target{Session: "s", Window: "0", Pane: "1"}

	err := client.Paste(ctx, target, []byte("data"), multiplexer.PasteOptions{})
	require.ErrorIs(t, err, context.Canceled)
	require.Len(t, runner.calls, 3)
	assert.Equal(t, "delete-buffer", runner.calls[2].args[0])
}

func TestPasteCleansBufferAndPreservesPrimaryError(t *testing.T) {
	primary := errors.New("paste failed")
	cleanup := errors.New("cleanup failed")
	runner := &fakeRunner{results: []runnerResult{{}, {err: primary}, {err: cleanup}}}
	var logs bytes.Buffer
	client := New(runner, zerolog.New(&logs))
	target := multiplexer.Target{Session: "s", Window: "0", Pane: "1"}

	err := client.Paste(context.Background(), target, []byte("a\nβ\x00"), multiplexer.PasteOptions{Bracketed: true})
	require.ErrorIs(t, err, primary)
	require.Len(t, runner.calls, 3)
	assert.Equal(t, "input", runner.calls[0].mode)
	assert.Equal(t, "a\nβ\x00", runner.calls[0].input)
	assert.Equal(t, "load-buffer", runner.calls[0].args[0])
	assert.Equal(t, "paste-buffer", runner.calls[1].args[0])
	assert.Contains(t, runner.calls[1].args, "-p")
	assert.Equal(t, "delete-buffer", runner.calls[2].args[0])
	assert.True(t, strings.HasPrefix(runner.calls[0].args[2], "hive-"))
	assert.Contains(t, logs.String(), "cleanup failed")
}

func TestCapturePaneOptions(t *testing.T) {
	start, end := -20, 4
	runner := &fakeRunner{results: []runnerResult{{stdout: "captured"}}}
	client := New(runner, zerolog.Nop())

	got, err := client.CapturePane(context.Background(), multiplexer.Target{Session: "s", Window: "2", Pane: "1"}, multiplexer.CaptureOptions{
		JoinWrappedLines: true,
		StartLine:        &start,
		EndLine:          &end,
	})
	require.NoError(t, err)
	assert.Equal(t, "captured", got)
	assert.Equal(t, []string{"capture-pane", "-p", "-t", "s:2.1", "-J", "-S", "-20", "-E", "4"}, runner.calls[0].args)
}

func TestCapturePaneUsesStableNativeTarget(t *testing.T) {
	runner := &fakeRunner{results: []runnerResult{{stdout: "captured"}}}
	client := New(runner, zerolog.Nop())

	_, err := client.CapturePane(context.Background(), multiplexer.Target{Pane: "%7"}, multiplexer.CaptureOptions{JoinWrappedLines: true})
	require.NoError(t, err)
	assert.Equal(t, []string{"capture-pane", "-p", "-t", "%7", "-J"}, runner.calls[0].args)
}

func TestResolveTargetReturnsQualifiedPane(t *testing.T) {
	runner := &fakeRunner{results: []runnerResult{{stdout: "work|||3|||2|||agent|||/repo|||42|||%7|||123|||title|||work|||0\n"}}}
	client := New(runner, zerolog.Nop())

	pane, err := client.ResolveTarget(context.Background(), "%7")
	require.NoError(t, err)
	assert.Equal(t, multiplexer.Target{Session: "work", Window: "3", Pane: "2"}, pane.Target)
	assert.Equal(t, "%7", pane.NativeID)
	assert.Equal(t, []string{"display-message", "-p", "-t", "%7", paneFormat}, runner.calls[0].args)
}

func TestLifecycleCommandsUseRenderedTargets(t *testing.T) {
	runner := &fakeRunner{}
	client := New(runner, zerolog.Nop())
	ctx := context.Background()

	require.NoError(t, client.RenameSession(ctx, multiplexer.Target{Session: "old"}, "new"))
	require.NoError(t, client.KillSession(ctx, multiplexer.Target{Session: "new"}))
	require.NoError(t, client.KillWindow(ctx, multiplexer.Target{Session: "new", Window: "2"}))

	assert.Equal(t, []string{"rename-session", "-t", "old", "new"}, runner.calls[0].args)
	assert.Equal(t, []string{"kill-session", "-t", "new"}, runner.calls[1].args)
	assert.Equal(t, []string{"kill-window", "-t", "new:2"}, runner.calls[2].args)
}

func TestCurrentSessionOutsideTmuxDoesNotRunCommand(t *testing.T) {
	runner := &fakeRunner{}
	client := New(runner, zerolog.Nop())
	client.getenv = func(string) string { return "" }

	target, err := client.CurrentSession(context.Background())
	require.NoError(t, err)
	assert.Empty(t, target)
	assert.Empty(t, runner.calls)
}

func TestCurrentSessionInsideTmux(t *testing.T) {
	runner := &fakeRunner{results: []runnerResult{{stdout: "work\n"}}}
	client := New(runner, zerolog.Nop())
	client.getenv = func(string) string { return "/tmp/tmux" }

	target, err := client.CurrentSession(context.Background())
	require.NoError(t, err)
	assert.Equal(t, multiplexer.Target{Session: "work"}, target)
}
