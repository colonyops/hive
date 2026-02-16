package command

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/colonyops/hive/internal/core/action"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Mock implementations

type mockDeleter struct {
	deleteErr error
	deleted   []string
}

func (m *mockDeleter) DeleteSession(_ context.Context, id string) error {
	m.deleted = append(m.deleted, id)
	return m.deleteErr
}

type mockRecycler struct {
	recycleErr error
	recycled   []string
	output     string // What to write to the writer
}

func (m *mockRecycler) RecycleSession(_ context.Context, id string, w io.Writer) error {
	m.recycled = append(m.recycled, id)
	if m.output != "" && w != nil {
		_, _ = w.Write([]byte(m.output))
	}
	return m.recycleErr
}

// DeleteExecutor tests

func TestDeleteExecutor_Execute(t *testing.T) {
	tests := []struct {
		name      string
		sessionID string
		deleteErr error
		wantErr   bool
	}{
		{
			name:      "successful delete",
			sessionID: "test-123",
			deleteErr: nil,
			wantErr:   false,
		},
		{
			name:      "delete error",
			sessionID: "test-456",
			deleteErr: errors.New("delete failed"),
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockDeleter{deleteErr: tt.deleteErr}
			exec := &DeleteExecutor{
				deleter:   mock,
				sessionID: tt.sessionID,
			}

			output, done, cancel := exec.Execute(context.Background())
			defer cancel()

			// Non-streaming executor should return nil output
			assert.Nil(t, output, "Expected nil output channel for non-streaming executor")

			// Wait for completion
			err := <-done

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			require.Len(t, mock.deleted, 1)
			assert.Equal(t, tt.sessionID, mock.deleted[0])
		})
	}
}

// RecycleExecutor tests

func TestRecycleExecutor_Execute(t *testing.T) {
	tests := []struct {
		name       string
		sessionID  string
		output     string
		recycleErr error
		wantErr    bool
	}{
		{
			name:       "successful recycle with output",
			sessionID:  "test-123",
			output:     "Recycling...\nDone.",
			recycleErr: nil,
			wantErr:    false,
		},
		{
			name:       "recycle error",
			sessionID:  "test-456",
			output:     "",
			recycleErr: errors.New("recycle failed"),
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockRecycler{recycleErr: tt.recycleErr, output: tt.output}
			exec := &RecycleExecutor{
				recycler:  mock,
				sessionID: tt.sessionID,
			}

			output, done, cancel := exec.Execute(context.Background())
			defer cancel()

			// Streaming executor should return non-nil output
			require.NotNil(t, output, "Expected non-nil output channel for streaming executor")

			// Collect all output
			var outputs []string
			for line := range output {
				outputs = append(outputs, line)
			}

			// Wait for completion
			err := <-done

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			require.Len(t, mock.recycled, 1)
			assert.Equal(t, tt.sessionID, mock.recycled[0])

			// Check output was received
			if tt.output != "" {
				assert.NotEmpty(t, outputs, "Expected output but got none")
			}
		})
	}
}

func TestRecycleExecutor_Cancel(t *testing.T) {
	mock := &mockRecycler{output: "test"}
	exec := &RecycleExecutor{
		recycler:  mock,
		sessionID: "test-123",
	}

	_, _, cancel := exec.Execute(context.Background())

	// Cancel immediately
	cancel()

	// Should not panic or block indefinitely
}

// ShellExecutor tests

func TestShellExecutor_Execute(t *testing.T) {
	tests := []struct {
		name    string
		cmd     string
		wantErr bool
	}{
		{
			name:    "successful command",
			cmd:     "echo hello",
			wantErr: false,
		},
		{
			name:    "failing command",
			cmd:     "exit 1",
			wantErr: true,
		},
		{
			name:    "command with stderr",
			cmd:     "echo error >&2 && exit 1",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := &ShellExecutor{cmd: tt.cmd}
			output, done, cancel := exec.Execute(context.Background())
			defer cancel()

			// Non-streaming executor should return nil output
			assert.Nil(t, output, "Expected nil output channel for non-streaming executor")

			// Wait for completion
			err := <-done

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// ExecuteSync tests

func TestExecuteSync(t *testing.T) {
	t.Run("non-streaming executor", func(t *testing.T) {
		mock := &mockDeleter{}
		exec := &DeleteExecutor{
			deleter:   mock,
			sessionID: "test-123",
		}

		err := ExecuteSync(context.Background(), exec)
		require.NoError(t, err, "ExecuteSync() error")

		assert.Len(t, mock.deleted, 1, "Expected delete to be called")
	})

	t.Run("streaming executor drains output", func(t *testing.T) {
		mock := &mockRecycler{output: "line1\nline2\nline3"}
		exec := &RecycleExecutor{
			recycler:  mock,
			sessionID: "test-123",
		}

		err := ExecuteSync(context.Background(), exec)
		require.NoError(t, err, "ExecuteSync() error")

		assert.Len(t, mock.recycled, 1, "Expected recycle to be called")
	})

	t.Run("returns error", func(t *testing.T) {
		mock := &mockDeleter{deleteErr: errors.New("failed")}
		exec := &DeleteExecutor{
			deleter:   mock,
			sessionID: "test-123",
		}

		err := ExecuteSync(context.Background(), exec)
		assert.Error(t, err, "Expected error")
	})
}

// channelWriter tests

func TestChannelWriter_Write(t *testing.T) {
	t.Run("sends data to channel", func(t *testing.T) {
		ch := make(chan string, 1)
		ctx := context.Background()
		w := &channelWriter{ch: ch, ctx: ctx}

		n, err := w.Write([]byte("hello"))
		require.NoError(t, err, "Write() error")
		assert.Equal(t, 5, n, "Write() n = %d, want 5", n)

		select {
		case msg := <-ch:
			assert.Equal(t, "hello", msg)
		default:
			require.FailNow(t, "expected message in channel")
		}
	})

	t.Run("returns error on cancelled context", func(t *testing.T) {
		ch := make(chan string) // unbuffered, will block
		ctx, cancel := context.WithCancel(context.Background())
		w := &channelWriter{ch: ch, ctx: ctx}

		// Cancel immediately so write can't succeed
		cancel()

		_, err := w.Write([]byte("hello"))
		assert.ErrorIs(t, err, context.Canceled, "Write() error = %v, want context.Canceled", err)
	})

	t.Run("handles multiple writes", func(t *testing.T) {
		ch := make(chan string, 3)
		ctx := context.Background()
		w := &channelWriter{ch: ch, ctx: ctx}

		_, _ = w.Write([]byte("one"))
		_, _ = w.Write([]byte("two"))
		_, _ = w.Write([]byte("three"))

		assert.Equal(t, "one", <-ch)
		assert.Equal(t, "two", <-ch)
		assert.Equal(t, "three", <-ch)
	})
}

// Mock TmuxOpener

type mockTmuxOpener struct {
	openErr error
	calls   []tmuxOpenCall
}

type tmuxOpenCall struct {
	Name, Path, Remote, TargetWindow string
	Background                       bool
}

func (m *mockTmuxOpener) OpenTmuxSession(_ context.Context, name, path, remote, targetWindow string, background bool) error {
	m.calls = append(m.calls, tmuxOpenCall{name, path, remote, targetWindow, background})
	return m.openErr
}

// Service tests

func TestService_CreateExecutor(t *testing.T) {
	svc := NewService(&mockDeleter{}, &mockRecycler{}, &mockTmuxOpener{})

	tests := []struct {
		name    string
		action  Action
		wantErr bool
	}{
		{
			name:    "delete action",
			action:  Action{Type: action.TypeDelete, SessionID: "test-123"},
			wantErr: false,
		},
		{
			name:    "recycle action",
			action:  Action{Type: action.TypeRecycle, SessionID: "test-123"},
			wantErr: false,
		},
		{
			name:    "shell action",
			action:  Action{Type: action.TypeShell, ShellCmd: "echo test"},
			wantErr: false,
		},
		{
			name:    "tmux open action",
			action:  Action{Type: action.TypeTmuxOpen, SessionName: "sess", SessionPath: "/work"},
			wantErr: false,
		},
		{
			name:    "tmux start action",
			action:  Action{Type: action.TypeTmuxStart, SessionName: "sess", SessionPath: "/work"},
			wantErr: false,
		},
		{
			name:    "unsupported action",
			action:  Action{Type: action.TypeNone},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec, err := svc.CreateExecutor(tt.action)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, exec)
			}
		})
	}
}
