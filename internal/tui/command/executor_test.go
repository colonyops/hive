package command

import (
	"context"
	"errors"
	"io"
	"testing"
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
			if output != nil {
				t.Error("Expected nil output channel for non-streaming executor")
			}

			// Wait for completion
			err := <-done

			if (err != nil) != tt.wantErr {
				t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
			}

			if len(mock.deleted) != 1 || mock.deleted[0] != tt.sessionID {
				t.Errorf("Expected delete called with %q, got %v", tt.sessionID, mock.deleted)
			}
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
			if output == nil {
				t.Error("Expected non-nil output channel for streaming executor")
			}

			// Collect all output
			var outputs []string
			for line := range output {
				outputs = append(outputs, line)
			}

			// Wait for completion
			err := <-done

			if (err != nil) != tt.wantErr {
				t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
			}

			if len(mock.recycled) != 1 || mock.recycled[0] != tt.sessionID {
				t.Errorf("Expected recycle called with %q, got %v", tt.sessionID, mock.recycled)
			}

			// Check output was received
			if tt.output != "" && len(outputs) == 0 {
				t.Error("Expected output but got none")
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
			if output != nil {
				t.Error("Expected nil output channel for non-streaming executor")
			}

			// Wait for completion
			err := <-done

			if (err != nil) != tt.wantErr {
				t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
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
		if err != nil {
			t.Errorf("ExecuteSync() error = %v", err)
		}

		if len(mock.deleted) != 1 {
			t.Error("Expected delete to be called")
		}
	})

	t.Run("streaming executor drains output", func(t *testing.T) {
		mock := &mockRecycler{output: "line1\nline2\nline3"}
		exec := &RecycleExecutor{
			recycler:  mock,
			sessionID: "test-123",
		}

		err := ExecuteSync(context.Background(), exec)
		if err != nil {
			t.Errorf("ExecuteSync() error = %v", err)
		}

		if len(mock.recycled) != 1 {
			t.Error("Expected recycle to be called")
		}
	})

	t.Run("returns error", func(t *testing.T) {
		mock := &mockDeleter{deleteErr: errors.New("failed")}
		exec := &DeleteExecutor{
			deleter:   mock,
			sessionID: "test-123",
		}

		err := ExecuteSync(context.Background(), exec)
		if err == nil {
			t.Error("Expected error")
		}
	})
}

// channelWriter tests

func TestChannelWriter_Write(t *testing.T) {
	t.Run("sends data to channel", func(t *testing.T) {
		ch := make(chan string, 1)
		ctx := context.Background()
		w := &channelWriter{ch: ch, ctx: ctx}

		n, err := w.Write([]byte("hello"))
		if err != nil {
			t.Fatalf("Write() error = %v", err)
		}
		if n != 5 {
			t.Errorf("Write() n = %d, want 5", n)
		}

		select {
		case msg := <-ch:
			if msg != "hello" {
				t.Errorf("got message %q, want %q", msg, "hello")
			}
		default:
			t.Fatal("expected message in channel")
		}
	})

	t.Run("returns error on cancelled context", func(t *testing.T) {
		ch := make(chan string) // unbuffered, will block
		ctx, cancel := context.WithCancel(context.Background())
		w := &channelWriter{ch: ch, ctx: ctx}

		// Cancel immediately so write can't succeed
		cancel()

		_, err := w.Write([]byte("hello"))
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Write() error = %v, want context.Canceled", err)
		}
	})

	t.Run("handles multiple writes", func(t *testing.T) {
		ch := make(chan string, 3)
		ctx := context.Background()
		w := &channelWriter{ch: ch, ctx: ctx}

		_, _ = w.Write([]byte("one"))
		_, _ = w.Write([]byte("two"))
		_, _ = w.Write([]byte("three"))

		if msg := <-ch; msg != "one" {
			t.Errorf("got %q, want %q", msg, "one")
		}
		if msg := <-ch; msg != "two" {
			t.Errorf("got %q, want %q", msg, "two")
		}
		if msg := <-ch; msg != "three" {
			t.Errorf("got %q, want %q", msg, "three")
		}
	})
}

// Service tests

func TestService_CreateExecutor(t *testing.T) {
	svc := NewService(&mockDeleter{}, &mockRecycler{})

	tests := []struct {
		name    string
		action  Action
		wantErr bool
	}{
		{
			name:    "delete action",
			action:  Action{Type: ActionTypeDelete, SessionID: "test-123"},
			wantErr: false,
		},
		{
			name:    "recycle action",
			action:  Action{Type: ActionTypeRecycle, SessionID: "test-123"},
			wantErr: false,
		},
		{
			name:    "shell action",
			action:  Action{Type: ActionTypeShell, ShellCmd: "echo test"},
			wantErr: false,
		},
		{
			name:    "unsupported action",
			action:  Action{Type: ActionTypeNone},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec, err := svc.CreateExecutor(tt.action)

			if (err != nil) != tt.wantErr {
				t.Errorf("CreateExecutor() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && exec == nil {
				t.Error("Expected executor but got nil")
			}
		})
	}
}
