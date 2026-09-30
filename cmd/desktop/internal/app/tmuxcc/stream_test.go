package tmuxcc

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// A recorded attach stream, replayed through the gateway and controller the
// way the reader goroutine drives them.
func TestRecordedAttachStream(t *testing.T) {
	t.Parallel()

	raw, err := os.Open("testdata/attach_stream.txt")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, raw.Close()) })

	ctrl := newController()
	ctrl.set([]Window{
		{ID: "@1", Name: "claude", Active: true, ActivePane: "%1"},
		{ID: "@2", Name: "shell", ActivePane: "%2"},
	})

	var (
		events     []Event
		reconciles int
		exited     string
	)
	writer := &recordingWriter{}
	gateway := NewGateway(writer, func(n Notification) {
		switch v := n.(type) {
		case OutputNotification:
			if w, ok := ctrl.windowForPane(v.Pane); ok {
				events = append(events, Output{
					At:       time.Now(),
					WindowID: w.ID,
					PaneID:   v.Pane,
					Data:     v.Data,
				})
			}
		case ExitNotification:
			exited = v.Reason
		case WindowAddNotification, LayoutChanged:
			reconciles++
		}
		events = append(events, ctrl.apply(n)...)
	}, testLogger())

	// The recording contains the replies to the attach sequence, so those
	// commands have to be in the FIFO for the guard blocks to pair up.
	commands := []string{
		"refresh-client -C 80,24",
		`list-windows -F "` + listWindowsFormat + `"`,
		"capture-pane -pe -J -t %1",
	}
	pending := make([]<-chan sendResult, 0, len(commands))
	for i, cmd := range commands {
		pending = append(pending, sendAsync(t.Context(), gateway, cmd))
		writer.await(t, i+1)
	}

	scanner := newLineScanner(raw, maxLineBytes)
	for {
		line, err := scanner.next()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		require.NoError(t, gateway.Feed(line), "line %q", line)
	}

	windowRows := <-pending[1]
	require.NoError(t, windowRows.err)
	require.Equal(t, []string{"@1 1 %1 claude", "@2 0 %2 shell"}, windowRows.lines)

	capture := <-pending[2]
	require.NoError(t, capture.err)
	require.Equal(t, []string{"claude> ready"}, capture.lines)

	require.Equal(t, 1, reconciles, "%window-add is the only reconcile trigger in the stream")
	require.Equal(t, "server exited", exited)
	require.Equal(t, []string{"@1", "@2"}, windowIDs(ctrl.Windows()), "@3 was added then closed")

	require.Equal(t, "thinking\r\n", outputData(events, "@1"))
	require.Equal(t, "$ \x1b[1mls\x1b[0m\r\n", outputData(events, "@2"),
		"output follows the window's active pane after %window-pane-changed")

	require.Contains(t, events, LifecycleChanged{Kind: LifecyclePaused, WindowID: "@1"})
	require.Contains(t, events, LifecycleChanged{Kind: LifecycleResumed, WindowID: "@1"})
}

func outputData(events []Event, windowID string) string {
	var sb strings.Builder
	for _, ev := range events {
		if out, ok := ev.(Output); ok && out.WindowID == windowID {
			sb.Write(out.Data)
		}
	}
	return sb.String()
}
