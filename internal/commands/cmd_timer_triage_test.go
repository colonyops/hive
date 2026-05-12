package commands_test

import (
	"context"
	"fmt"
	"io"
	"testing"

	"github.com/colonyops/hive/internal/commands"
	"github.com/colonyops/hive/internal/core/tmux"
	"github.com/rs/zerolog"
)

// triageMockExecutor implements executil.Executor and dispatches on the
// tmux subcommand so we can simulate "server up but session missing"
// scenarios that RecordingExecutor (which keys off "tmux" alone) can't.
type triageMockExecutor struct {
	onInfo        error
	onHasSession  error
	onListWindows error
	listOutput    string
}

func (m *triageMockExecutor) Run(_ context.Context, cmd string, args ...string) ([]byte, error) {
	if cmd != "tmux" || len(args) == 0 {
		return nil, fmt.Errorf("unexpected cmd: %s %v", cmd, args)
	}
	switch args[0] {
	case "info":
		return nil, m.onInfo
	case "has-session":
		return nil, m.onHasSession
	case "list-windows":
		return []byte(m.listOutput), m.onListWindows
	}
	return nil, fmt.Errorf("unexpected tmux subcommand: %s", args[0])
}

func (m *triageMockExecutor) RunDir(_ context.Context, _, _ string, _ ...string) ([]byte, error) {
	panic("unused")
}

func (m *triageMockExecutor) RunStream(_ context.Context, _, _ io.Writer, _ string, _ ...string) error {
	panic("unused")
}

func (m *triageMockExecutor) RunDirStream(_ context.Context, _ string, _, _ io.Writer, _ string, _ ...string) error {
	panic("unused")
}

func newTriageClient(m *triageMockExecutor) *tmux.Client {
	return tmux.New(m, zerolog.Nop())
}

func TestRunTmuxTriage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		mock             *triageMockExecutor
		target           string
		agentSendExit    int
		agentSendStderr  string
		wantServerExists bool
		wantSessExists   bool
		wantWinExists    bool
	}{
		{
			name:             "all up window present",
			mock:             &triageMockExecutor{listOutput: "agent\nshell\nlogs\n"},
			target:           "my-session:agent",
			wantServerExists: true,
			wantSessExists:   true,
			wantWinExists:    true,
		},
		{
			name:             "server down",
			mock:             &triageMockExecutor{onInfo: fmt.Errorf("no server")},
			target:           "my-session:agent",
			wantServerExists: false,
			wantSessExists:   false,
			wantWinExists:    false,
		},
		{
			name:             "session missing",
			mock:             &triageMockExecutor{onHasSession: fmt.Errorf("no session")},
			target:           "my-session:agent",
			wantServerExists: true,
			wantSessExists:   false,
			wantWinExists:    false,
		},
		{
			name:             "window missing",
			mock:             &triageMockExecutor{listOutput: "shell\nlogs\n"},
			target:           "sess:agent",
			wantServerExists: true,
			wantSessExists:   true,
			wantWinExists:    false,
		},
		{
			name:             "target with no window",
			mock:             &triageMockExecutor{},
			target:           "sess",
			wantServerExists: true,
			wantSessExists:   true,
			wantWinExists:    true,
		},
		{
			name:             "list-windows fails",
			mock:             &triageMockExecutor{onListWindows: fmt.Errorf("boom")},
			target:           "sess:agent",
			wantServerExists: true,
			wantSessExists:   true,
			wantWinExists:    false,
		},
		{
			name:             "target with dot pane suffix",
			mock:             &triageMockExecutor{listOutput: "agent\n"},
			target:           "sess:agent.0",
			wantServerExists: true,
			wantSessExists:   true,
			wantWinExists:    true,
		},
		{
			name:             "agent_send fields propagated",
			mock:             &triageMockExecutor{listOutput: "agent\n"},
			target:           "sess:agent",
			agentSendExit:    42,
			agentSendStderr:  "oops",
			wantServerExists: true,
			wantSessExists:   true,
			wantWinExists:    true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			client := newTriageClient(tc.mock)
			rep := commands.RunTmuxTriage(context.Background(), client, tc.target,
				tc.agentSendExit, tc.agentSendStderr)

			if rep.TmuxServerExists != tc.wantServerExists {
				t.Errorf("TmuxServerExists = %v, want %v", rep.TmuxServerExists, tc.wantServerExists)
			}
			if rep.SessionExists != tc.wantSessExists {
				t.Errorf("SessionExists = %v, want %v", rep.SessionExists, tc.wantSessExists)
			}
			if rep.WindowExists != tc.wantWinExists {
				t.Errorf("WindowExists = %v, want %v", rep.WindowExists, tc.wantWinExists)
			}
			if rep.AgentSendExit != tc.agentSendExit {
				t.Errorf("AgentSendExit = %d, want %d", rep.AgentSendExit, tc.agentSendExit)
			}
			if rep.AgentSendStderr != tc.agentSendStderr {
				t.Errorf("AgentSendStderr = %q, want %q", rep.AgentSendStderr, tc.agentSendStderr)
			}
		})
	}
}
