package tmpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderer_Render(t *testing.T) {
	r := New(Config{})

	tests := []struct {
		name    string
		tmpl    string
		data    any
		want    string
		wantErr bool
	}{
		{
			name: "simple substitution",
			tmpl: "hello {{ .Name }}",
			data: map[string]string{"Name": "world"},
			want: "hello world",
		},
		{
			name: "multiple variables",
			tmpl: `cd "{{ .Path }}" && echo "{{ .Prompt }}"`,
			data: map[string]string{
				"Path":   "/tmp/session",
				"Prompt": "implement feature X",
			},
			want: `cd "/tmp/session" && echo "implement feature X"`,
		},
		{
			name: "struct data",
			tmpl: "{{ .Name }} at {{ .Path }}",
			data: struct {
				Name string
				Path string
			}{Name: "test", Path: "/tmp"},
			want: "test at /tmp",
		},
		{
			name: "no variables",
			tmpl: "static string",
			data: nil,
			want: "static string",
		},
		{
			name:    "missing key errors",
			tmpl:    "{{ .Missing }}",
			data:    map[string]string{"Name": "test"},
			wantErr: true,
		},
		{
			name:    "invalid template syntax",
			tmpl:    "{{ .Name }",
			data:    map[string]string{"Name": "test"},
			wantErr: true,
		},
		{
			name: "empty value is valid",
			tmpl: "prefix{{ .Name }}suffix",
			data: map[string]string{"Name": ""},
			want: "prefixsuffix",
		},
		{
			name: "shq function with spaces",
			tmpl: "echo {{ .Prompt | shq }}",
			data: map[string]string{"Prompt": "hello world"},
			want: "echo 'hello world'",
		},
		{
			name: "shq function with single quotes",
			tmpl: "echo {{ .Prompt | shq }}",
			data: map[string]string{"Prompt": "it's a test"},
			want: `echo 'it'\''s a test'`,
		},
		{
			name: "shq function with double quotes",
			tmpl: "echo {{ .Prompt | shq }}",
			data: map[string]string{"Prompt": `say "hello"`},
			want: `echo 'say "hello"'`,
		},
		{
			name: "shq function with empty string",
			tmpl: "echo {{ .Prompt | shq }}",
			data: map[string]string{"Prompt": ""},
			want: "echo ''",
		},
		{
			name: "shq function with special chars",
			tmpl: "echo {{ .Prompt | shq }}",
			data: map[string]string{"Prompt": "$(whoami) && rm -rf /"},
			want: "echo '$(whoami) && rm -rf /'",
		},
		{
			name: "shq function with multiline prompt",
			tmpl: "echo {{ .Prompt | shq }}",
			data: map[string]string{"Prompt": "line one\nline two\nline three"},
			want: "echo 'line one\nline two\nline three'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := r.Render(tt.tmpl, tt.data)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRenderer_Defaults(t *testing.T) {
	r := New(Config{})

	got, err := r.Render("{{ agentCommand }}", nil)
	require.NoError(t, err)
	assert.Equal(t, "claude", got)

	got, err = r.Render("{{ agentWindow }}", nil)
	require.NoError(t, err)
	assert.Equal(t, "claude", got)

	got, err = r.Render("{{ agentFlags }}", nil)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestRenderer_Configured(t *testing.T) {
	r := New(Config{
		AgentCommand: "aider",
		AgentWindow:  "aider",
		AgentFlags:   "--model sonnet",
	})

	got, err := r.Render("{{ agentCommand }}", nil)
	require.NoError(t, err)
	assert.Equal(t, "aider", got)

	got, err = r.Render("{{ agentWindow }}", nil)
	require.NoError(t, err)
	assert.Equal(t, "aider", got)

	got, err = r.Render("{{ agentFlags }}", nil)
	require.NoError(t, err)
	assert.Equal(t, "--model sonnet", got)
}

func TestRenderer_ScriptPaths(t *testing.T) {
	r := New(Config{
		ScriptPaths: map[string]string{
			"hive-tmux":  "/usr/local/bin/hive-tmux",
			"agent-send": "/usr/local/bin/agent-send",
		},
	})

	got, err := r.Render("{{ hiveTmux }}", nil)
	require.NoError(t, err)
	assert.Equal(t, "/usr/local/bin/hive-tmux", got)

	got, err = r.Render("{{ agentSend }}", nil)
	require.NoError(t, err)
	assert.Equal(t, "/usr/local/bin/agent-send", got)
}

func TestRenderer_ScriptPaths_FallbackToName(t *testing.T) {
	r := New(Config{})

	got, err := r.Render("{{ hiveTmux }}", nil)
	require.NoError(t, err)
	assert.Equal(t, "hive-tmux", got)
}

func TestRenderer_SpawnCommand(t *testing.T) {
	r := New(Config{
		ScriptPaths: map[string]string{
			"hive-tmux": "/bin/hive-tmux",
		},
		AgentCommand: "aider",
		AgentWindow:  "aider",
		AgentFlags:   "--model sonnet",
	})

	tmplStr := `HIVE_AGENT_COMMAND={{ agentCommand | shq }} HIVE_AGENT_WINDOW={{ agentWindow | shq }} HIVE_AGENT_FLAGS={{ agentFlags | shq }} {{ hiveTmux }} {{ .Name | shq }} {{ .Path | shq }}`
	data := struct {
		Name string
		Path string
	}{Name: "test-session", Path: "/tmp/work"}

	got, err := r.Render(tmplStr, data)
	require.NoError(t, err)
	assert.Equal(t, "HIVE_AGENT_COMMAND='aider' HIVE_AGENT_WINDOW='aider' HIVE_AGENT_FLAGS='--model sonnet' /bin/hive-tmux 'test-session' '/tmp/work'", got)
}

func TestNewValidation(t *testing.T) {
	r := NewValidation()

	// Functions that return placeholder values
	for _, fn := range []string{"hiveTmux", "agentSend", "agentCommand", "agentWindow"} {
		got, err := r.Render("{{ "+fn+" }}", nil)
		require.NoError(t, err, "function %s failed", fn)
		assert.NotEmpty(t, got, "function %s returned empty", fn)
	}

	// agentFlags is empty by default
	got, err := r.Render("{{ agentFlags }}", nil)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestRenderers_Isolated(t *testing.T) {
	r1 := New(Config{AgentCommand: "claude"})
	r2 := New(Config{AgentCommand: "aider"})

	got1, err := r1.Render("{{ agentCommand }}", nil)
	require.NoError(t, err)
	assert.Equal(t, "claude", got1)

	got2, err := r2.Render("{{ agentCommand }}", nil)
	require.NoError(t, err)
	assert.Equal(t, "aider", got2)
}

func TestRenderer_WithAgent(t *testing.T) {
	base := New(Config{AgentCommand: "claude", AgentWindow: "claude"})
	override := base.WithAgent("aider", "aider", "--model sonnet")

	got, err := override.Render("{{ agentCommand }}", nil)
	require.NoError(t, err)
	assert.Equal(t, "aider", got)

	got, err = override.Render("{{ agentWindow }}", nil)
	require.NoError(t, err)
	assert.Equal(t, "aider", got)

	got, err = override.Render("{{ agentFlags }}", nil)
	require.NoError(t, err)
	assert.Equal(t, "--model sonnet", got)

	got, err = base.Render("{{ agentCommand }}", nil)
	require.NoError(t, err)
	assert.Equal(t, "claude", got)
}

func TestRenderer_WithAgent_PreservesOtherFuncs(t *testing.T) {
	base := New(Config{
		ScriptPaths:  map[string]string{"hive-tmux": "/bin/hive-tmux"},
		AgentCommand: "claude",
	})
	override := base.WithAgent("aider", "aider", "")

	got, err := override.Render("{{ hiveTmux }}", nil)
	require.NoError(t, err)
	assert.Equal(t, "/bin/hive-tmux", got)

	got, err = override.Render("{{ agentCommand | shq }}", nil)
	require.NoError(t, err)
	assert.Equal(t, "'aider'", got)
}
