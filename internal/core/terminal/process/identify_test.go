package process

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// defaultTools is the list used by tests that exercise the full detection stack.
var defaultTools = []string{"claude", "gemini", "aider", "codex", "cursor", "opencode", "cline", "pi"}

func TestLooksLikeClaude(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		argv []string
		want bool
	}{
		{
			name: "CLAUDECODE=1 in env",
			env:  map[string]string{envClaudeCode: "1"},
			argv: nil,
			want: true,
		},
		{
			name: "CLAUDECODE absent but argv contains claude",
			env:  map[string]string{"PATH": "/usr/bin"},
			argv: []string{"/usr/local/bin/claude"},
			want: true,
		},
		{
			name: "neither env nor argv match",
			env:  map[string]string{"PATH": "/usr/bin"},
			argv: []string{"/bin/bash"},
			want: false,
		},
		{
			name: "nil env but argv has claude",
			env:  nil,
			argv: []string{"/usr/local/bin/claude"},
			want: true,
		},
		{
			name: "nil env and nil argv",
			env:  nil,
			argv: nil,
			want: false,
		},
		{
			name: "CLAUDECODE not equal to 1",
			env:  map[string]string{envClaudeCode: "0"},
			argv: []string{"/bin/bash"},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := looksLikeClaude(tt.env, tt.argv)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestToolFromArgv(t *testing.T) {
	tests := []struct {
		name string
		argv []string
		want string
	}{
		{name: "npx claude", argv: []string{"npx", "claude"}, want: "claude"},
		{name: "node with gemini path", argv: []string{"node", "/path/to/gemini"}, want: "gemini"},
		{name: "direct aider", argv: []string{"/usr/local/bin/aider"}, want: "aider"},
		{name: "direct pi", argv: []string{"/usr/local/bin/pi"}, want: "pi"},
		{name: "python module aider", argv: []string{"python3", "-m", "aider"}, want: "aider"},
		{name: "empty argv", argv: []string{}, want: ""},
		{name: "nil argv", argv: nil, want: ""},
		{name: "bash shell", argv: []string{"/bin/bash"}, want: ""},
		{name: "bash command string containing claude", argv: []string{"bash", "-lc", "echo claude"}, want: ""},
		{name: "zsh command string containing gemini", argv: []string{"zsh", "-c", "grep gemini file"}, want: ""},
		{name: "npx with unknown tool", argv: []string{"npx", "some-unknown-tool"}, want: ""},
		{name: "direct claude binary", argv: []string{"/usr/local/bin/claude"}, want: "claude"},
		{name: "env prefix claude", argv: []string{"env", "DEBUG=1", "claude"}, want: "claude"},
		{name: "codex", argv: []string{"/usr/bin/codex"}, want: "codex"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toolFromArgv(tt.argv, defaultTools)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIdentifyWith(t *testing.T) {
	errPermission := errors.New("permission denied")
	tests := []struct {
		name     string
		panePID  int
		reader   fakeReader
		wantPID  int
		wantTool string
		wantComm string
	}{
		{
			name:    "direct claude binary",
			panePID: 100,
			reader: fakeReader{procs: map[int]fakeProc{
				100: {tpgid: 200},
				200: {comm: "claude", argv: []string{"/usr/local/bin/claude"}, env: map[string]string{envClaudeCode: "1"}},
			}},
			wantPID: 200, wantTool: "claude", wantComm: "claude",
		},
		{
			name:    "direct pi binary",
			panePID: 100,
			reader: fakeReader{procs: map[int]fakeProc{
				100: {tpgid: 200},
				200: {comm: "pi", argv: []string{"/usr/local/bin/pi"}},
			}},
			wantPID: 200, wantTool: "pi", wantComm: "pi",
		},
		{
			name:    "node wrapper claude child depth 1",
			panePID: 100,
			reader: fakeReader{
				procs: map[int]fakeProc{
					100: {tpgid: 200},
					200: {comm: "node", argv: []string{"node", "wrapper.js"}},
					201: {comm: "claude", argv: []string{"/opt/bin/claude"}},
				},
				children: map[int][]int{200: {201}},
			},
			wantPID: 201, wantTool: "claude", wantComm: "claude",
		},
		{
			name:    "sh bash claude depth 2",
			panePID: 100,
			reader: fakeReader{
				procs: map[int]fakeProc{
					100: {tpgid: 200},
					200: {comm: "sh", argv: []string{"sh"}},
					201: {comm: "bash", argv: []string{"bash"}},
					202: {comm: "claude", argv: []string{"claude"}},
				},
				children: map[int][]int{200: {201}, 201: {202}},
			},
			wantPID: 202, wantTool: "claude", wantComm: "claude",
		},
		{
			name:    "shell with no agent children",
			panePID: 100,
			reader: fakeReader{
				procs: map[int]fakeProc{
					100: {tpgid: 200},
					200: {comm: "zsh", argv: []string{"zsh"}},
					201: {comm: "sleep", argv: []string{"sleep", "600"}},
				},
				children: map[int][]int{200: {201}},
			},
			wantPID: 200, wantTool: ToolShell, wantComm: "zsh",
		},
		{
			name:    "bash command string containing claude with no agent child",
			panePID: 100,
			reader: fakeReader{procs: map[int]fakeProc{
				100: {tpgid: 200},
				200: {comm: "bash", argv: []string{"bash", "-lc", "echo claude"}},
			}},
			wantPID: 200, wantTool: ToolShell, wantComm: "bash",
		},
		{
			name:    "zsh command string containing codex with no agent child",
			panePID: 100,
			reader: fakeReader{procs: map[int]fakeProc{
				100: {tpgid: 200},
				200: {comm: "zsh", argv: []string{"zsh", "-c", "grep codex file"}},
			}},
			wantPID: 200, wantTool: ToolShell, wantComm: "zsh",
		},
		{
			name:    "tpgid error fallback to pane pid",
			panePID: 100,
			reader: fakeReader{tpgidErr: errPermission, procs: map[int]fakeProc{
				100: {comm: "claude", argv: []string{"claude"}},
			}},
			wantPID: 100, wantTool: "claude", wantComm: "claude",
		},
		{
			name:    "empty comm process exited between reads",
			panePID: 100,
			reader: fakeReader{procs: map[int]fakeProc{
				100: {tpgid: 200},
				200: {comm: "", argv: nil},
			}},
			wantPID: 200, wantTool: ToolShell, wantComm: "",
		},
		{
			name:    "cmdline error permission denied falls back to comm",
			panePID: 100,
			reader: fakeReader{procs: map[int]fakeProc{
				100: {tpgid: 200},
				200: {comm: "codex", argvErr: errPermission},
			}},
			wantPID: 200, wantTool: "codex", wantComm: "codex",
		},
		{
			name:    "nil environ with argv fallback",
			panePID: 100,
			reader: fakeReader{procs: map[int]fakeProc{
				100: {tpgid: 200},
				200: {comm: "node", argv: []string{"node", "/opt/claude"}, env: nil},
			}},
			wantPID: 200, wantTool: "claude", wantComm: "node",
		},
		{
			name:    "mise shim argv names claude",
			panePID: 100,
			reader: fakeReader{procs: map[int]fakeProc{
				100: {tpgid: 200},
				200: {comm: "mise", argv: []string{"mise", "exec", "--", "claude"}},
			}},
			wantPID: 200, wantTool: "claude", wantComm: "mise",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := IdentifyWith(tt.panePID, tt.reader, defaultTools)
			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, tt.wantPID, got.PID)
			assert.Equal(t, tt.wantTool, got.Tool)
			assert.Equal(t, tt.wantComm, got.Comm)
		})
	}
}

func TestIdentifyWithInvalidPID(t *testing.T) {
	got, err := IdentifyWith(0, fakeReader{}, nil)
	require.NoError(t, err)
	assert.Nil(t, got)
}

type fakeReader struct {
	procs    map[int]fakeProc
	children map[int][]int
	tpgidErr error
}

type fakeProc struct {
	tpgid   int
	comm    string
	argv    []string
	env     map[string]string
	argvErr error
}

func (f fakeReader) TPGID(pid int) (int, error) {
	if f.tpgidErr != nil {
		return 0, f.tpgidErr
	}
	proc := f.procs[pid]
	return proc.tpgid, nil
}

func (f fakeReader) Comm(pid int) string { return f.procs[pid].comm }

func (f fakeReader) Cmdline(pid int) ([]string, error) {
	proc := f.procs[pid]
	if proc.argvErr != nil {
		return nil, proc.argvErr
	}
	return proc.argv, nil
}

func (f fakeReader) Environ(pid int) map[string]string { return f.procs[pid].env }

func (f fakeReader) Children(pid int) ([]int, error) { return f.children[pid], nil }

func TestToolFromBasename(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "claude exact", input: "claude", want: "claude"},
		{name: "claude with version suffix", input: "claude-3.5", want: "claude"},
		{name: "aider", input: "aider", want: "aider"},
		{name: "gemini", input: "gemini", want: "gemini"},
		{name: "codex", input: "codex", want: "codex"},
		{name: "cursor", input: "cursor", want: "cursor"},
		{name: "opencode", input: "opencode", want: "opencode"},
		{name: "cline", input: "cline", want: "cline"},
		{name: "pi exact", input: "pi", want: "pi"},
		{name: "pipeline does not match pi", input: "pipeline", want: ""},
		{name: "pip does not match pi", input: "pip", want: ""},
		{name: "bash returns empty", input: "bash", want: ""},
		{name: "zsh returns empty", input: "zsh", want: ""},
		{name: "empty string", input: "", want: ""},
		{name: "uppercase CLAUDE", input: "CLAUDE", want: "claude"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toolFromBasename(tt.input, defaultTools)
			assert.Equal(t, tt.want, got)
		})
	}
}
