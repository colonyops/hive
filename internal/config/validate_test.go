package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hay-kot/criterio"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// validConfig returns a Config with all required fields set for testing.
func validConfig(t *testing.T) *Config {
	t.Helper()
	return &Config{
		GitPath: "git",
		DataDir: t.TempDir(),
		Git:     GitConfig{StatusWorkers: 1},
		Agents: AgentsConfig{
			Default:  "claude",
			Profiles: map[string]AgentProfile{"claude": {}},
		},
		Database: DatabaseConfig{
			MaxOpenConns: 2,
			MaxIdleConns: 2,
			BusyTimeout:  5000,
		},
		Todos: TodosConfig{},
	}
}

func TestValidateDeep_InvalidSpawnTemplate(t *testing.T) {
	cfg := validConfig(t)
	cfg.Rules = []Rule{
		{
			Pattern: "",
			Spawn:   []string{"echo {{.Path}", "echo {{.Invalid}}"},
		},
	}

	err := cfg.ValidateDeep("")

	var fieldErrs criterio.FieldErrors
	require.ErrorAs(t, err, &fieldErrs)
	assert.Len(t, fieldErrs, 2)
	assert.Contains(t, fieldErrs[0].Field, "rules[0].spawn")
	assert.Contains(t, fieldErrs[0].Err.Error(), "template error")
}

func TestValidateDeep_InvalidRecycleTemplate(t *testing.T) {
	cfg := validConfig(t)
	cfg.Rules = []Rule{
		{
			Pattern: "",
			Recycle: []string{"git checkout {{.Invalid}}"},
		},
	}

	err := cfg.ValidateDeep("")

	var fieldErrs criterio.FieldErrors
	require.ErrorAs(t, err, &fieldErrs)
	assert.Len(t, fieldErrs, 1)
	assert.Contains(t, fieldErrs[0].Field, "rules[0].recycle")
	assert.Contains(t, fieldErrs[0].Err.Error(), "template error")
}

func TestValidateDeep_ValidRecycleTemplate(t *testing.T) {
	cfg := validConfig(t)
	cfg.Rules = []Rule{
		{
			Pattern: "",
			Recycle: []string{
				"git fetch origin",
				"git checkout {{.DefaultBranch}}",
				"git reset --hard origin/{{.DefaultBranch}}",
			},
		},
	}

	err := cfg.ValidateDeep("")
	assert.NoError(t, err)
}

func TestValidateDeep_InvalidRulePattern(t *testing.T) {
	cfg := validConfig(t)
	cfg.Rules = []Rule{
		{Pattern: "[invalid", Commands: []string{"echo"}},
	}

	err := cfg.ValidateDeep("")

	var fieldErrs criterio.FieldErrors
	require.ErrorAs(t, err, &fieldErrs)
	assert.Len(t, fieldErrs, 1)
	assert.Contains(t, fieldErrs[0].Field, "rules")
	assert.Contains(t, fieldErrs[0].Err.Error(), "invalid regex")
}

func TestValidateDeep_GitPathNotFound(t *testing.T) {
	cfg := validConfig(t)
	cfg.GitPath = "/nonexistent/path/to/git"

	err := cfg.ValidateDeep("")

	var fieldErrs criterio.FieldErrors
	require.ErrorAs(t, err, &fieldErrs)

	hasGitError := false
	for _, e := range fieldErrs {
		if e.Field == "git_path" {
			hasGitError = true
			break
		}
	}
	assert.True(t, hasGitError, "expected error about git path")
}

func TestValidateDeep_DataDirIsFile(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "notadir")
	require.NoError(t, os.WriteFile(tmpFile, []byte("test"), 0o644))

	cfg := validConfig(t)
	cfg.DataDir = tmpFile

	err := cfg.ValidateDeep("")

	var fieldErrs criterio.FieldErrors
	require.ErrorAs(t, err, &fieldErrs)

	hasDataDirError := false
	for _, e := range fieldErrs {
		if e.Field == "data_dir" {
			hasDataDirError = true
			break
		}
	}
	assert.True(t, hasDataDirError, "expected error about data dir")
}

func TestValidateDeep_ConfigFileIsDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := validConfig(t)

	err := cfg.ValidateDeep(tmpDir)

	var fieldErrs criterio.FieldErrors
	require.ErrorAs(t, err, &fieldErrs)

	hasConfigError := false
	for _, e := range fieldErrs {
		if e.Field == "config_file" {
			hasConfigError = true
			break
		}
	}
	assert.True(t, hasConfigError, "expected error about config file being a directory")
}

func TestWarnings_EmptyRule(t *testing.T) {
	cfg := validConfig(t)
	cfg.Rules = []Rule{
		{Pattern: ".*"},
	}

	err := cfg.ValidateDeep("")
	require.NoError(t, err)

	warnings := cfg.Warnings()
	hasWarning := false
	for _, w := range warnings {
		if w.Category == "Rules" && strings.Contains(w.Message, "neither commands nor copy") {
			hasWarning = true
			break
		}
	}
	assert.True(t, hasWarning, "expected warning about empty rule")
}

func TestValidateDeep_ValidRulesWithCopy(t *testing.T) {
	cfg := validConfig(t)
	cfg.Rules = []Rule{
		{Pattern: "", Copy: []string{".envrc"}},
		{Pattern: "^https://github.com/.*", Copy: []string{"*.yaml"}},
	}

	err := cfg.ValidateDeep("")
	assert.NoError(t, err)
}

func TestValidateDeep_ValidRulesWithCommandsAndCopy(t *testing.T) {
	cfg := validConfig(t)
	cfg.Rules = []Rule{
		{
			Pattern:  "^https://github.com/hay-kot/.*",
			Commands: []string{"mise trust", "task dep:sync"},
			Copy:     []string{".envrc", "configs/*.yaml"},
		},
	}

	err := cfg.ValidateDeep("")
	assert.NoError(t, err)
}

func TestGetMaxRecycled(t *testing.T) {
	intPtr := func(n int) *int { return &n }

	tests := []struct {
		name     string
		rules    []Rule
		remote   string
		expected int
	}{
		{
			name:     "default when no rules",
			rules:    nil,
			remote:   "https://github.com/foo/bar",
			expected: DefaultMaxRecycled,
		},
		{
			name: "catch-all rule sets default",
			rules: []Rule{
				{Pattern: "", MaxRecycled: intPtr(10)},
			},
			remote:   "https://github.com/foo/bar",
			expected: 10,
		},
		{
			name: "catch-all unlimited (0)",
			rules: []Rule{
				{Pattern: "", MaxRecycled: intPtr(0)},
			},
			remote:   "https://github.com/foo/bar",
			expected: 0,
		},
		{
			name: "specific rule override",
			rules: []Rule{
				{Pattern: "", MaxRecycled: intPtr(10)},
				{Pattern: "github.com/foo/.*", MaxRecycled: intPtr(2)},
			},
			remote:   "https://github.com/foo/bar",
			expected: 2,
		},
		{
			name: "specific rule unlimited override",
			rules: []Rule{
				{Pattern: "", MaxRecycled: intPtr(10)},
				{Pattern: "github.com/foo/.*", MaxRecycled: intPtr(0)},
			},
			remote:   "https://github.com/foo/bar",
			expected: 0,
		},
		{
			name: "non-matching rule falls back to catch-all",
			rules: []Rule{
				{Pattern: "", MaxRecycled: intPtr(10)},
				{Pattern: "github.com/other/.*", MaxRecycled: intPtr(2)},
			},
			remote:   "https://github.com/foo/bar",
			expected: 10,
		},
		{
			name: "last matching rule wins",
			rules: []Rule{
				{Pattern: "", MaxRecycled: intPtr(10)},
				{Pattern: "github.com/.*", MaxRecycled: intPtr(5)},
				{Pattern: "github.com/foo/.*", MaxRecycled: intPtr(2)},
			},
			remote:   "https://github.com/foo/bar",
			expected: 2,
		},
		{
			name: "rule without max_recycled inherits from previous",
			rules: []Rule{
				{Pattern: "", MaxRecycled: intPtr(10)},
				{Pattern: "github.com/foo/.*", Commands: []string{"echo test"}},
			},
			remote:   "https://github.com/foo/bar",
			expected: 10,
		},
		{
			name: "later rule with max_recycled overrides earlier without",
			rules: []Rule{
				{Pattern: "github.com/foo/.*", MaxRecycled: intPtr(3)},
				{Pattern: "github.com/foo/bar", Commands: []string{"echo"}}, // no MaxRecycled
			},
			remote:   "https://github.com/foo/bar",
			expected: 3, // inherits from earlier matching rule with MaxRecycled
		},
		{
			name: "no matching rules uses default",
			rules: []Rule{
				{Pattern: "github.com/other/.*", MaxRecycled: intPtr(2)},
			},
			remote:   "https://github.com/foo/bar",
			expected: DefaultMaxRecycled,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig(t)
			cfg.Rules = tt.rules

			result := cfg.GetMaxRecycled(tt.remote)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetRecycleCommands(t *testing.T) {
	tests := []struct {
		name     string
		rules    []Rule
		remote   string
		expected []string
	}{
		{
			name:     "no rules returns defaults",
			rules:    nil,
			remote:   "https://github.com/foo/bar",
			expected: DefaultRecycleCommands,
		},
		{
			name: "catch-all rule overrides defaults",
			rules: []Rule{
				{Pattern: "", Recycle: []string{"echo recycle"}},
			},
			remote:   "https://github.com/foo/bar",
			expected: []string{"echo recycle"},
		},
		{
			name: "specific rule overrides catch-all",
			rules: []Rule{
				{Pattern: "", Recycle: []string{"echo default"}},
				{Pattern: "github.com/foo/.*", Recycle: []string{"echo foo"}},
			},
			remote:   "https://github.com/foo/bar",
			expected: []string{"echo foo"},
		},
		{
			name: "non-matching rule uses catch-all",
			rules: []Rule{
				{Pattern: "", Recycle: []string{"echo default"}},
				{Pattern: "github.com/other/.*", Recycle: []string{"echo other"}},
			},
			remote:   "https://github.com/foo/bar",
			expected: []string{"echo default"},
		},
		{
			name: "rule without recycle uses defaults",
			rules: []Rule{
				{Pattern: "", Spawn: []string{"echo spawn"}}, // no recycle
			},
			remote:   "https://github.com/foo/bar",
			expected: DefaultRecycleCommands,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig(t)
			cfg.Rules = tt.rules

			result := cfg.GetRecycleCommands(tt.remote)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValidate_MaxRecycledNegative(t *testing.T) {
	intPtr := func(n int) *int { return &n }

	t.Run("negative in rule", func(t *testing.T) {
		cfg := validConfig(t)
		cfg.Rules = []Rule{
			{Pattern: ".*", MaxRecycled: intPtr(-5)},
		}

		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "max_recycled")
	})

	t.Run("valid values pass", func(t *testing.T) {
		cfg := validConfig(t)
		cfg.Rules = []Rule{
			{Pattern: "", MaxRecycled: intPtr(0)}, // 0 is valid (unlimited)
			{Pattern: ".*", MaxRecycled: intPtr(5)},
		}

		err := cfg.Validate()
		assert.NoError(t, err)
	})
}

func TestAgentsConfig_UnmarshalYAML(t *testing.T) {
	tests := []struct {
		name     string
		yaml     string
		wantDef  string
		wantKeys []string
	}{
		{
			name: "full config",
			yaml: `
default: aider
claude:
  command: claude
  flags:
    - "--model"
    - "opus"
aider:
  command: /opt/bin/aider
  flags: ["--model", "sonnet"]
`,
			wantDef:  "aider",
			wantKeys: []string{"claude", "aider"},
		},
		{
			name: "minimal profile",
			yaml: `
default: claude
claude: {}
`,
			wantDef:  "claude",
			wantKeys: []string{"claude"},
		},
		{
			name: "command defaults to empty",
			yaml: `
default: myagent
myagent: {}
`,
			wantDef:  "myagent",
			wantKeys: []string{"myagent"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got AgentsConfig
			err := yaml.Unmarshal([]byte(tt.yaml), &got)
			require.NoError(t, err)
			assert.Equal(t, tt.wantDef, got.Default)
			for _, key := range tt.wantKeys {
				assert.Contains(t, got.Profiles, key)
			}
		})
	}
}

func TestAgentsConfig_DefaultProfile(t *testing.T) {
	cfg := AgentsConfig{
		Default: "aider",
		Profiles: map[string]AgentProfile{
			"claude": {Command: "claude"},
			"aider":  {Command: "/opt/bin/aider", Flags: []string{"--model", "sonnet"}},
		},
	}

	p := cfg.DefaultProfile()
	assert.Equal(t, "/opt/bin/aider", p.Command)
	assert.Equal(t, []string{"--model", "sonnet"}, p.Flags)
}

func TestAgentsConfig_DefaultProfileMissing(t *testing.T) {
	cfg := AgentsConfig{
		Default:  "missing",
		Profiles: map[string]AgentProfile{},
	}
	p := cfg.DefaultProfile()
	assert.Empty(t, p.Command)
}

func TestAgentProfile_CommandOrDefault(t *testing.T) {
	t.Run("uses command when set", func(t *testing.T) {
		p := AgentProfile{Command: "/usr/bin/aider"}
		assert.Equal(t, "/usr/bin/aider", p.CommandOrDefault("aider"))
	})

	t.Run("falls back to key", func(t *testing.T) {
		p := AgentProfile{}
		assert.Equal(t, "claude", p.CommandOrDefault("claude"))
	})
}

func TestAgentProfile_ShellFlags(t *testing.T) {
	t.Run("empty flags", func(t *testing.T) {
		p := AgentProfile{}
		assert.Empty(t, p.ShellFlags())
	})

	t.Run("single flag", func(t *testing.T) {
		p := AgentProfile{Flags: []string{"--verbose"}}
		assert.Equal(t, "--verbose", p.ShellFlags())
	})

	t.Run("multiple flags", func(t *testing.T) {
		p := AgentProfile{Flags: []string{"--model", "opus"}}
		assert.Equal(t, "--model opus", p.ShellFlags())
	})

	t.Run("flags with special chars", func(t *testing.T) {
		p := AgentProfile{Flags: []string{"--prompt", "it's a test"}}
		assert.Equal(t, "--prompt it's a test", p.ShellFlags())
	})
}

func TestValidate_AgentsDefaultMissing(t *testing.T) {
	cfg := validConfig(t)
	cfg.Agents = AgentsConfig{
		Default:  "nonexistent",
		Profiles: map[string]AgentProfile{"claude": {}},
	}

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "agents.default")
	assert.Contains(t, err.Error(), "nonexistent")
}

func TestValidate_AgentsDefaultValid(t *testing.T) {
	cfg := validConfig(t)
	cfg.Agents = AgentsConfig{
		Default:  "claude",
		Profiles: map[string]AgentProfile{"claude": {}},
	}

	err := cfg.Validate()
	assert.NoError(t, err)
}

func TestValidate_RuleAgentMissing(t *testing.T) {
	cfg := validConfig(t)
	cfg.Agents = AgentsConfig{
		Default:  "claude",
		Profiles: map[string]AgentProfile{"claude": {}},
	}
	cfg.Rules = []Rule{{Pattern: "", Agent: "aider"}}

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rules[0].agent")
	assert.Contains(t, err.Error(), "aider")
}

func TestApplyDefaults_AgentsEmpty(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DataDir = t.TempDir()
	cfg.applyDefaults()

	assert.Equal(t, "claude", cfg.Agents.Default)
	assert.Contains(t, cfg.Agents.Profiles, "claude")
}

func TestApplyDefaults_AgentsPreserved(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DataDir = t.TempDir()
	cfg.Agents = AgentsConfig{
		Default: "aider",
		Profiles: map[string]AgentProfile{
			"aider": {Command: "/opt/bin/aider"},
		},
	}
	cfg.applyDefaults()

	assert.Equal(t, "aider", cfg.Agents.Default)
	assert.Equal(t, "/opt/bin/aider", cfg.Agents.Profiles["aider"].Command)
	assert.NotContains(t, cfg.Agents.Profiles, "claude")
}

func TestApplyDefaults_PreviewWindowMatcherExcludesPi(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DataDir = t.TempDir()
	cfg.applyDefaults()

	assert.NotContains(t, cfg.Tmux.PreviewWindowMatcher, "\\bpi\\b")
	assert.NotContains(t, cfg.Tmux.PreviewWindowMatcher, "π")
}

func TestDefaultWindows(t *testing.T) {
	windows := DefaultWindows()
	require.Len(t, windows, 2)

	assert.Equal(t, "{{ agentWindow }}", windows[0].Name)
	assert.Contains(t, windows[0].Command, "agentCommand")
	assert.Contains(t, windows[0].Command, "shq")
	assert.True(t, windows[0].Focus)

	assert.Equal(t, "shell", windows[1].Name)
	assert.Empty(t, windows[1].Command)
	assert.False(t, windows[1].Focus)
}

func TestResolveSpawn(t *testing.T) {
	tests := []struct {
		name         string
		rules        []Rule
		remote       string
		batch        bool
		wantWindows  bool
		wantCommands []string
		wantAgent    string
	}{
		{
			name:        "no rules returns default windows",
			rules:       nil,
			remote:      "https://github.com/foo/bar",
			wantWindows: true,
		},
		{
			name: "windows-only rule",
			rules: []Rule{
				{
					Pattern: "",
					Windows: []WindowConfig{
						{Name: "editor", Command: "vim", Focus: true},
						{Name: "shell"},
					},
				},
			},
			remote:      "https://github.com/foo/bar",
			wantWindows: true,
		},
		{
			name: "spawn-only rule returns commands",
			rules: []Rule{
				{Pattern: "", Spawn: []string{"echo spawn"}},
			},
			remote:       "https://github.com/foo/bar",
			wantWindows:  false,
			wantCommands: []string{"echo spawn"},
		},
		{
			name: "batch_spawn returns batch commands",
			rules: []Rule{
				{Pattern: "", Spawn: []string{"echo spawn"}, BatchSpawn: []string{"echo batch"}},
			},
			remote:       "https://github.com/foo/bar",
			batch:        true,
			wantWindows:  false,
			wantCommands: []string{"echo batch"},
		},
		{
			name: "batch with no batch_spawn falls to default windows",
			rules: []Rule{
				{Pattern: "", Spawn: []string{"echo spawn"}},
			},
			remote:      "https://github.com/foo/bar",
			batch:       true,
			wantWindows: true,
		},
		{
			name: "windows override spawn in later rule",
			rules: []Rule{
				{Pattern: "", Spawn: []string{"echo spawn"}},
				{Pattern: "github.com/foo/.*", Windows: []WindowConfig{{Name: "agent", Focus: true}}},
			},
			remote:      "https://github.com/foo/bar",
			wantWindows: true,
		},
		{
			name: "spawn overrides windows in later rule",
			rules: []Rule{
				{Pattern: "", Windows: []WindowConfig{{Name: "agent"}}},
				{Pattern: "github.com/foo/.*", Spawn: []string{"echo override"}},
			},
			remote:       "https://github.com/foo/bar",
			wantWindows:  false,
			wantCommands: []string{"echo override"},
		},
		{
			name: "non-matching rule ignored",
			rules: []Rule{
				{Pattern: "", Windows: []WindowConfig{{Name: "agent"}}},
				{Pattern: "github.com/other/.*", Spawn: []string{"echo other"}},
			},
			remote:      "https://github.com/foo/bar",
			wantWindows: true,
		},
		{
			name: "last matching rule wins",
			rules: []Rule{
				{Pattern: "", Windows: []WindowConfig{{Name: "default"}}},
				{Pattern: "github.com/.*", Windows: []WindowConfig{{Name: "github"}}},
				{Pattern: "github.com/foo/.*", Windows: []WindowConfig{{Name: "foo"}}},
			},
			remote:      "https://github.com/foo/bar",
			wantWindows: true,
		},
		{
			name: "catch-all pattern matches all",
			rules: []Rule{
				{Pattern: "", Windows: []WindowConfig{{Name: "catch-all", Focus: true}, {Name: "shell"}}},
			},
			remote:      "https://gitlab.com/some/repo",
			wantWindows: true,
		},
		{
			name: "agent-only matching rule uses default windows",
			rules: []Rule{
				{Pattern: "", Agent: "aider"},
			},
			remote:      "https://github.com/foo/bar",
			wantWindows: true,
			wantAgent:   "aider",
		},
		{
			name: "last matching agent wins independently of windows",
			rules: []Rule{
				{Pattern: "", Agent: "claude", Windows: []WindowConfig{{Name: "default"}}},
				{Pattern: "github.com/foo/.*", Agent: "aider"},
			},
			remote:      "https://github.com/foo/bar",
			wantWindows: true,
			wantAgent:   "aider",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig(t)
			cfg.Rules = tt.rules

			strategy := ResolveSpawn(cfg.Rules, tt.remote, tt.batch)
			assert.Equal(t, tt.wantWindows, strategy.IsWindows(), "IsWindows mismatch")
			assert.Equal(t, tt.wantAgent, strategy.Agent)

			if tt.wantCommands != nil {
				assert.Equal(t, tt.wantCommands, strategy.Commands)
			}
		})
	}
}

func TestValidate_WindowsMutualExclusive(t *testing.T) {
	tests := []struct {
		name    string
		rule    Rule
		wantErr string
	}{
		{
			name: "windows and spawn",
			rule: Rule{
				Pattern: "",
				Windows: []WindowConfig{{Name: "agent"}},
				Spawn:   []string{"echo spawn"},
			},
			wantErr: "cannot have both windows and spawn",
		},
		{
			name: "windows and batch_spawn",
			rule: Rule{
				Pattern:    "",
				Windows:    []WindowConfig{{Name: "agent"}},
				BatchSpawn: []string{"echo batch"},
			},
			wantErr: "cannot have both windows and batch_spawn",
		},
		{
			name: "windows only is valid",
			rule: Rule{
				Pattern: "",
				Windows: []WindowConfig{{Name: "agent"}},
			},
		},
		{
			name: "window command and panes",
			rule: Rule{
				Pattern: "",
				Windows: []WindowConfig{{
					Name:    "agent",
					Command: "claude",
					Panes:   []PaneConfig{{Command: "npm test"}},
				}},
			},
			wantErr: "command and panes are mutually exclusive",
		},
		{
			name: "invalid pane split",
			rule: Rule{
				Pattern: "",
				Windows: []WindowConfig{{
					Name:  "agent",
					Panes: []PaneConfig{{Command: "npm test", Split: "diagonal"}},
				}},
			},
			wantErr: "must be one of: horizontal, vertical",
		},
		{
			name: "spawn only is valid",
			rule: Rule{
				Pattern: "",
				Spawn:   []string{"echo spawn"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig(t)
			cfg.Rules = []Rule{tt.rule}

			err := cfg.Validate()
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidate_WindowsNameRequired(t *testing.T) {
	cfg := validConfig(t)
	cfg.Rules = []Rule{
		{
			Pattern: "",
			Windows: []WindowConfig{
				{Name: "agent", Focus: true},
				{Name: ""},
			},
		},
	}

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "windows[1].name")
	assert.Contains(t, err.Error(), "is required")
}

func TestValidateDeep_WindowTemplates(t *testing.T) {
	t.Run("valid templates", func(t *testing.T) {
		cfg := validConfig(t)
		cfg.Rules = []Rule{
			{
				Pattern: "",
				Windows: []WindowConfig{
					{Name: "claude", Command: "claude {{ .Name }}", Focus: true},
					{
						Name: "shell",
						Dir:  "{{ .Path }}",
						Panes: []PaneConfig{
							{Command: "npm test {{ .Prompt }}", Size: "{{ .Name }}", Split: "horizontal"},
							{Dir: "{{ .ContextDir }}"},
						},
					},
				},
			},
		}

		err := cfg.ValidateDeep("")
		assert.NoError(t, err)
	})

	t.Run("invalid name template", func(t *testing.T) {
		cfg := validConfig(t)
		cfg.Rules = []Rule{
			{
				Pattern: "",
				Windows: []WindowConfig{
					{Name: "{{ .Invalid }}", Focus: true},
				},
			},
		}

		err := cfg.ValidateDeep("")
		var fieldErrs criterio.FieldErrors
		require.ErrorAs(t, err, &fieldErrs)
		assert.Contains(t, fieldErrs[0].Field, "windows[0].name")
	})

	t.Run("invalid command template", func(t *testing.T) {
		cfg := validConfig(t)
		cfg.Rules = []Rule{
			{
				Pattern: "",
				Windows: []WindowConfig{
					{Name: "agent", Command: "{{ .Missing }}", Focus: true},
				},
			},
		}

		err := cfg.ValidateDeep("")
		var fieldErrs criterio.FieldErrors
		require.ErrorAs(t, err, &fieldErrs)
		assert.Contains(t, fieldErrs[0].Field, "windows[0].command")
	})

	t.Run("invalid dir template", func(t *testing.T) {
		cfg := validConfig(t)
		cfg.Rules = []Rule{
			{
				Pattern: "",
				Windows: []WindowConfig{
					{Name: "agent", Dir: "{{ .NoSuch }}", Focus: true},
				},
			},
		}

		err := cfg.ValidateDeep("")
		var fieldErrs criterio.FieldErrors
		require.ErrorAs(t, err, &fieldErrs)
		assert.Contains(t, fieldErrs[0].Field, "windows[0].dir")
	})

	t.Run("invalid pane template", func(t *testing.T) {
		cfg := validConfig(t)
		cfg.Rules = []Rule{
			{
				Pattern: "",
				Windows: []WindowConfig{
					{Name: "agent", Panes: []PaneConfig{{Command: "{{ .Missing }}"}}},
				},
			},
		}

		err := cfg.ValidateDeep("")
		var fieldErrs criterio.FieldErrors
		require.ErrorAs(t, err, &fieldErrs)
		assert.Contains(t, fieldErrs[0].Field, "windows[0].panes[0].command")
	})

	t.Run("Prompt template valid in windows", func(t *testing.T) {
		cfg := validConfig(t)
		cfg.Rules = []Rule{
			{
				Pattern: "",
				Windows: []WindowConfig{
					{Name: "agent", Command: "claude {{ .Prompt }}", Focus: true},
				},
			},
		}

		err := cfg.ValidateDeep("")
		assert.NoError(t, err)
	})
}

func TestGetCloneStrategy(t *testing.T) {
	tests := []struct {
		name     string
		rules    []Rule
		remote   string
		expected string
	}{
		{
			name:     "no rules defaults to full",
			rules:    nil,
			remote:   "https://github.com/foo/bar",
			expected: CloneStrategyFull,
		},
		{
			name: "catch-all rule sets worktree",
			rules: []Rule{
				{Pattern: "", CloneStrategy: CloneStrategyWorktree},
			},
			remote:   "https://github.com/foo/bar",
			expected: CloneStrategyWorktree,
		},
		{
			name: "non-matching rule does not change default",
			rules: []Rule{
				{Pattern: "github.com/other/.*", CloneStrategy: CloneStrategyWorktree},
			},
			remote:   "https://github.com/foo/bar",
			expected: CloneStrategyFull,
		},
		{
			name: "matching rule overrides default",
			rules: []Rule{
				{Pattern: "github.com/foo/.*", CloneStrategy: CloneStrategyWorktree},
			},
			remote:   "https://github.com/foo/bar",
			expected: CloneStrategyWorktree,
		},
		{
			name: "last matching rule wins",
			rules: []Rule{
				{Pattern: "github.com/.*", CloneStrategy: CloneStrategyWorktree},
				{Pattern: "github.com/foo/.*", CloneStrategy: CloneStrategyFull},
			},
			remote:   "https://github.com/foo/bar",
			expected: CloneStrategyFull,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig(t)
			cfg.Rules = tt.rules

			result := cfg.GetCloneStrategy(tt.remote)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValidateCloneStrategy(t *testing.T) {
	t.Run("empty is valid", func(t *testing.T) {
		assert.NoError(t, ValidateCloneStrategy(""))
	})
	t.Run("full is valid", func(t *testing.T) {
		assert.NoError(t, ValidateCloneStrategy(CloneStrategyFull))
	})
	t.Run("worktree is valid", func(t *testing.T) {
		assert.NoError(t, ValidateCloneStrategy(CloneStrategyWorktree))
	})
	t.Run("invalid value returns error", func(t *testing.T) {
		err := ValidateCloneStrategy("invalid")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid clone_strategy")
	})
}

func TestGetBranchTemplate(t *testing.T) {
	tests := []struct {
		name     string
		rules    []Rule
		remote   string
		expected string
	}{
		{
			name:     "no rules returns empty string",
			rules:    nil,
			remote:   "https://github.com/foo/bar",
			expected: "",
		},
		{
			name: "catch-all rule sets template",
			rules: []Rule{
				{Pattern: "", BranchTemplate: "dev/{{ .Slug }}"},
			},
			remote:   "https://github.com/foo/bar",
			expected: "dev/{{ .Slug }}",
		},
		{
			name: "non-matching rule returns empty string",
			rules: []Rule{
				{Pattern: "github.com/other/.*", BranchTemplate: "dev/{{ .Slug }}"},
			},
			remote:   "https://github.com/foo/bar",
			expected: "",
		},
		{
			name: "matching rule returns template",
			rules: []Rule{
				{Pattern: "github.com/foo/.*", BranchTemplate: "dev/{{ .Slug }}-{{ .ID }}"},
			},
			remote:   "https://github.com/foo/bar",
			expected: "dev/{{ .Slug }}-{{ .ID }}",
		},
		{
			name: "last matching rule wins",
			rules: []Rule{
				{Pattern: "github.com/.*", BranchTemplate: "dev/{{ .Slug }}"},
				{Pattern: "github.com/foo/.*", BranchTemplate: "feat/{{ .Slug }}-{{ .ID }}"},
			},
			remote:   "https://github.com/foo/bar",
			expected: "feat/{{ .Slug }}-{{ .ID }}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig(t)
			cfg.Rules = tt.rules

			result := cfg.GetBranchTemplate(tt.remote)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValidateDeep_BranchTemplate(t *testing.T) {
	t.Run("valid template passes", func(t *testing.T) {
		cfg := validConfig(t)
		cfg.Rules = []Rule{
			{Pattern: "", BranchTemplate: "dev/{{ .Slug }}-{{ .ID }}"},
		}
		assert.NoError(t, cfg.ValidateDeep(""))
	})

	t.Run("invalid template syntax fails", func(t *testing.T) {
		cfg := validConfig(t)
		cfg.Rules = []Rule{
			{Pattern: "", BranchTemplate: "dev/{{ .Slug"},
		}
		err := cfg.ValidateDeep("")
		var fieldErrs criterio.FieldErrors
		require.ErrorAs(t, err, &fieldErrs)
		assert.Contains(t, fieldErrs[0].Field, "rules[0].branch_template")
		assert.Contains(t, fieldErrs[0].Err.Error(), "template error")
	})

	t.Run("empty branch_template is valid", func(t *testing.T) {
		cfg := validConfig(t)
		cfg.Rules = []Rule{
			{Pattern: "", Commands: []string{"echo hello"}},
		}
		assert.NoError(t, cfg.ValidateDeep(""))
	})
}
