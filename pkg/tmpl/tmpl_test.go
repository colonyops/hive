package tmpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRender(t *testing.T) {
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Render(tt.tmpl, tt.data)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestAgentTemplateFunctions_Defaults(t *testing.T) {
	// Reset agent config to ensure defaults
	agentConfig.command = ""
	agentConfig.window = ""
	agentConfig.flags = ""

	got, err := Render("{{ agentCommand }}", nil)
	require.NoError(t, err)
	assert.Equal(t, "claude", got)

	got, err = Render("{{ agentWindow }}", nil)
	require.NoError(t, err)
	assert.Equal(t, "claude", got)

	got, err = Render("{{ agentFlags }}", nil)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestAgentTemplateFunctions_Configured(t *testing.T) {
	SetAgentConfig("aider", "aider", "'--model' 'sonnet'")
	t.Cleanup(func() {
		agentConfig.command = ""
		agentConfig.window = ""
		agentConfig.flags = ""
	})

	got, err := Render("{{ agentCommand }}", nil)
	require.NoError(t, err)
	assert.Equal(t, "aider", got)

	got, err = Render("{{ agentWindow }}", nil)
	require.NoError(t, err)
	assert.Equal(t, "aider", got)

	got, err = Render("{{ agentFlags }}", nil)
	require.NoError(t, err)
	assert.Equal(t, "'--model' 'sonnet'", got)
}

func TestAgentTemplateFunctions_InSpawnCommand(t *testing.T) {
	SetAgentConfig("aider", "aider", "'--model' 'sonnet'")
	t.Cleanup(func() {
		agentConfig.command = ""
		agentConfig.window = ""
		agentConfig.flags = ""
	})

	tmplStr := `HIVE_AGENT_COMMAND={{ agentCommand | shq }} HIVE_AGENT_WINDOW={{ agentWindow | shq }} HIVE_AGENT_FLAGS={{ agentFlags }} hive-tmux {{ .Name | shq }} {{ .Path | shq }}`
	data := struct {
		Name string
		Path string
	}{Name: "test-session", Path: "/tmp/work"}

	got, err := Render(tmplStr, data)
	require.NoError(t, err)
	assert.Equal(t, "HIVE_AGENT_COMMAND='aider' HIVE_AGENT_WINDOW='aider' HIVE_AGENT_FLAGS='--model' 'sonnet' hive-tmux 'test-session' '/tmp/work'", got)
}
