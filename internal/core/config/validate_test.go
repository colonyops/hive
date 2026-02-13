package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hay-kot/criterio"
	"github.com/hay-kot/hive/internal/core/styles"
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
		TUI:     TUIConfig{Theme: styles.DefaultTheme},
		Agents: AgentsConfig{
			Default:  "claude",
			Profiles: map[string]AgentProfile{"claude": {}},
		},
		Database: DatabaseConfig{
			MaxOpenConns: 2,
			MaxIdleConns: 2,
			BusyTimeout:  5000,
		},
	}
}

func TestValidateDeep_ValidConfig(t *testing.T) {
	cfg := validConfig(t)
	cfg.Rules = []Rule{
		{
			Pattern:    "^https://github.com/.*",
			Commands:   []string{"echo hello"},
			Spawn:      []string{"echo {{.Path}}", "echo {{.Name}} {{.Slug}}"},
			BatchSpawn: []string{"echo {{.Path}}", "echo {{.Name}} {{.Prompt}}"},
			Recycle:    []string{"git reset --hard", "git checkout main"},
		},
	}
	// With the new model, keybindings reference commands
	cfg.UserCommands = map[string]UserCommand{
		"open": {Sh: "open {{.Path}}", Help: "open"},
	}
	cfg.Keybindings = map[string]Keybinding{
		"r": {Cmd: "Recycle"}, // System default
		"o": {Cmd: "open"},
	}

	err := cfg.ValidateDeep("")
	assert.NoError(t, err, "expected valid config")
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

func TestValidateDeep_KeybindingMissingCmd(t *testing.T) {
	cfg := validConfig(t)
	cfg.Keybindings = map[string]Keybinding{
		"x": {Help: "does nothing"},
	}

	err := cfg.ValidateDeep("")

	var fieldErrs criterio.FieldErrors
	require.ErrorAs(t, err, &fieldErrs)
	assert.Len(t, fieldErrs, 1)
	assert.Contains(t, fieldErrs[0].Err.Error(), "cmd is required")
}

func TestValidateUserKeybindings_InvalidCmdReference(t *testing.T) {
	cfg := validConfig(t)
	cfg.Keybindings = map[string]Keybinding{
		"x": {Cmd: "NonExistentCommand"},
	}

	err := cfg.validateUserKeybindings()

	var fieldErrs criterio.FieldErrors
	require.ErrorAs(t, err, &fieldErrs)
	assert.Len(t, fieldErrs, 1)
	assert.Contains(t, fieldErrs[0].Err.Error(), "does not reference a valid user command")
}

func TestValidateUserKeybindings_PluginCommandFromDefaults(t *testing.T) {
	cfg := validConfig(t)
	// TmuxOpen is a plugin command referenced by defaultKeybindings — should be valid
	cfg.Keybindings = map[string]Keybinding{
		"x": {Cmd: "TmuxOpen"},
	}

	err := cfg.validateUserKeybindings()
	assert.NoError(t, err)
}

func TestValidateDeep_KeybindingValidCmdReference(t *testing.T) {
	cfg := validConfig(t)
	cfg.UserCommands = map[string]UserCommand{
		"open": {Sh: "open {{.Path}} {{.Remote}} {{.ID}} {{.Name}}"},
	}
	cfg.Keybindings = map[string]Keybinding{
		"o": {Cmd: "open"},
		"r": {Cmd: "Recycle"}, // System default
		"d": {Cmd: "Delete"},  // System default
	}

	err := cfg.ValidateDeep("")
	assert.NoError(t, err)
}

func TestValidateDeep_UserCommandBothActionAndSh(t *testing.T) {
	cfg := validConfig(t)
	cfg.UserCommands = map[string]UserCommand{
		"bad": {Action: ActionRecycle, Sh: "echo test"},
	}

	err := cfg.ValidateDeep("")

	var fieldErrs criterio.FieldErrors
	require.ErrorAs(t, err, &fieldErrs)
	assert.Len(t, fieldErrs, 1)
	assert.Contains(t, fieldErrs[0].Err.Error(), "cannot have both")
}

func TestValidateDeep_UserCommandInvalidAction(t *testing.T) {
	cfg := validConfig(t)
	cfg.UserCommands = map[string]UserCommand{
		"bad": {Action: "invalid"},
	}

	err := cfg.ValidateDeep("")

	var fieldErrs criterio.FieldErrors
	require.ErrorAs(t, err, &fieldErrs)
	assert.Len(t, fieldErrs, 1)
	assert.Contains(t, fieldErrs[0].Err.Error(), "invalid action")
}

func TestValidateDeep_UserCommandValidAction(t *testing.T) {
	cfg := validConfig(t)
	cfg.UserCommands = map[string]UserCommand{
		"my-recycle": {Action: ActionRecycle, Help: "custom recycle"},
		"my-delete":  {Action: ActionDelete, Help: "custom delete"},
	}

	err := cfg.ValidateDeep("")
	assert.NoError(t, err)
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

func TestGetSpawnCommands(t *testing.T) {
	tests := []struct {
		name     string
		rules    []Rule
		remote   string
		batch    bool
		expected []string
	}{
		{
			name:     "no rules returns defaults",
			rules:    nil,
			remote:   "https://github.com/foo/bar",
			batch:    false,
			expected: DefaultSpawnCommands,
		},
		{
			name: "catch-all rule provides spawn",
			rules: []Rule{
				{Pattern: "", Spawn: []string{"echo spawn"}},
			},
			remote:   "https://github.com/foo/bar",
			batch:    false,
			expected: []string{"echo spawn"},
		},
		{
			name: "batch_spawn returns batch commands",
			rules: []Rule{
				{
					Pattern:    "",
					Spawn:      []string{"echo spawn"},
					BatchSpawn: []string{"echo batch"},
				},
			},
			remote:   "https://github.com/foo/bar",
			batch:    true,
			expected: []string{"echo batch"},
		},
		{
			name: "batch falls back to defaults when no batch_spawn",
			rules: []Rule{
				{Pattern: "", Spawn: []string{"echo spawn"}},
			},
			remote:   "https://github.com/foo/bar",
			batch:    true,
			expected: DefaultBatchSpawnCommands,
		},
		{
			name: "specific rule overrides catch-all",
			rules: []Rule{
				{Pattern: "", Spawn: []string{"echo default"}},
				{Pattern: "github.com/foo/.*", Spawn: []string{"echo foo"}},
			},
			remote:   "https://github.com/foo/bar",
			batch:    false,
			expected: []string{"echo foo"},
		},
		{
			name: "non-matching rule uses catch-all",
			rules: []Rule{
				{Pattern: "", Spawn: []string{"echo default"}},
				{Pattern: "github.com/other/.*", Spawn: []string{"echo other"}},
			},
			remote:   "https://github.com/foo/bar",
			batch:    false,
			expected: []string{"echo default"},
		},
		{
			name: "last matching rule wins",
			rules: []Rule{
				{Pattern: "", Spawn: []string{"echo default"}},
				{Pattern: "github.com/.*", Spawn: []string{"echo github"}},
				{Pattern: "github.com/foo/.*", Spawn: []string{"echo foo"}},
			},
			remote:   "https://github.com/foo/bar",
			batch:    false,
			expected: []string{"echo foo"},
		},
		{
			name: "rule without spawn inherits from previous",
			rules: []Rule{
				{Pattern: "", Spawn: []string{"echo default"}},
				{Pattern: "github.com/foo/.*", Commands: []string{"setup"}}, // no spawn
			},
			remote:   "https://github.com/foo/bar",
			batch:    false,
			expected: []string{"echo default"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig(t)
			cfg.Rules = tt.rules

			result := cfg.GetSpawnCommands(tt.remote, tt.batch)
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

func TestValidate_UserCommandNeitherActionNorSh(t *testing.T) {
	cfg := validConfig(t)
	cfg.UserCommands = map[string]UserCommand{
		"test": {Help: "no action or sh command"},
	}

	err := cfg.Validate()

	var fieldErrs criterio.FieldErrors
	require.ErrorAs(t, err, &fieldErrs)
	assert.Len(t, fieldErrs, 1)
	assert.Contains(t, fieldErrs[0].Err.Error(), "must have either action or sh")
}

func TestValidateDeep_UserCommandInvalidShTemplate(t *testing.T) {
	cfg := validConfig(t)
	cfg.UserCommands = map[string]UserCommand{
		"bad": {Sh: "open {{.Invalid}}"},
	}

	err := cfg.ValidateDeep("")

	var fieldErrs criterio.FieldErrors
	require.ErrorAs(t, err, &fieldErrs)
	assert.Len(t, fieldErrs, 1)
	assert.Contains(t, fieldErrs[0].Err.Error(), "template error")
}

func TestValidateDeep_UserCommandValidShTemplate(t *testing.T) {
	cfg := validConfig(t)
	cfg.UserCommands = map[string]UserCommand{
		"open":       {Sh: "open {{.Path}}"},
		"review":     {Sh: "send-claude {{.Name}} /review"},
		"with-args":  {Sh: `echo {{.Name}} {{ range .Args }}{{ . }} {{ end }}`},
		"all-vars":   {Sh: "cmd {{.Path}} {{.Remote}} {{.ID}} {{.Name}}"},
		"index-args": {Sh: `go test {{ index .Args 0 }}`},
		"multi-args": {Sh: `cmd {{ index .Args 0 }} {{ index .Args 1 }}`},
	}

	err := cfg.ValidateDeep("")
	assert.NoError(t, err)
}

func TestValidate_UserCommandInvalidName(t *testing.T) {
	tests := []struct {
		name        string
		commandName string
		wantErr     string
	}{
		{
			name:        "name with spaces",
			commandName: "my command",
			wantErr:     "invalid command name",
		},
		{
			name:        "name with special chars",
			commandName: "test@command",
			wantErr:     "invalid command name",
		},
		{
			name:        "empty name",
			commandName: "",
			wantErr:     "cannot be empty",
		},
		{
			name:        "name with slash",
			commandName: "test/command",
			wantErr:     "invalid command name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig(t)
			cfg.UserCommands = map[string]UserCommand{
				tt.commandName: {Sh: "echo test"},
			}

			err := cfg.Validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestValidate_UserCommandValidNames(t *testing.T) {
	cfg := validConfig(t)
	cfg.UserCommands = map[string]UserCommand{
		"simple":      {Sh: "echo test"},
		"with-dash":   {Sh: "echo test"},
		"with_under":  {Sh: "echo test"},
		"MixedCase":   {Sh: "echo test"},
		"with123":     {Sh: "echo test"},
		"a":           {Sh: "echo test"},
		"long-name_2": {Sh: "echo test"},
	}

	err := cfg.Validate()
	assert.NoError(t, err)
}

func TestValidate_UserCommandValidScopes(t *testing.T) {
	cfg := validConfig(t)
	cfg.UserCommands = map[string]UserCommand{
		"global-cmd":   {Sh: "echo test", Scope: []string{"global"}},
		"review-cmd":   {Sh: "echo test", Scope: []string{"review"}},
		"sessions-cmd": {Sh: "echo test", Scope: []string{"sessions"}},
		"messages-cmd": {Sh: "echo test", Scope: []string{"messages"}},
		"multi-scope":  {Sh: "echo test", Scope: []string{"review", "sessions"}},
		"no-scope":     {Sh: "echo test"}, // nil scope = global
	}

	err := cfg.Validate()
	assert.NoError(t, err)
}

func TestIsValidIdentifier(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		errMsg  string
	}{
		{name: "simple", input: "message", wantErr: false},
		{name: "with underscore", input: "my_var", wantErr: false},
		{name: "with digits", input: "var123", wantErr: false},
		{name: "single char", input: "x", wantErr: false},
		{name: "uppercase", input: "MyVar", wantErr: false},
		{name: "starts with digit", input: "1var", wantErr: true, errMsg: "must not start with a digit"},
		{name: "has dash", input: "my-var", wantErr: true, errMsg: "must contain only letters"},
		{name: "has space", input: "my var", wantErr: true, errMsg: "must contain only letters"},
		{name: "has dot", input: "my.var", wantErr: true, errMsg: "must contain only letters"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := isValidIdentifier(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateFormField(t *testing.T) {
	tests := []struct {
		name    string
		field   FormField
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid text field",
			field:   FormField{Variable: "message", Type: FormTypeText, Label: "Message"},
			wantErr: false,
		},
		{
			name:    "valid textarea field",
			field:   FormField{Variable: "body", Type: FormTypeTextArea, Label: "Body"},
			wantErr: false,
		},
		{
			name:    "valid select field",
			field:   FormField{Variable: "env", Type: FormTypeSelect, Label: "Env", Options: []string{"dev", "prod"}},
			wantErr: false,
		},
		{
			name:    "valid multi-select field",
			field:   FormField{Variable: "tags", Type: FormTypeMultiSelect, Label: "Tags", Options: []string{"a", "b"}},
			wantErr: false,
		},
		{
			name:    "valid session preset",
			field:   FormField{Variable: "target", Preset: FormPresetSessionSelector, Label: "Target"},
			wantErr: false,
		},
		{
			name:    "valid project preset with multi",
			field:   FormField{Variable: "repos", Preset: FormPresetProjectSelector, Multi: true, Label: "Repos"},
			wantErr: false,
		},
		{
			name:    "missing variable",
			field:   FormField{Type: FormTypeText, Label: "Message"},
			wantErr: true,
			errMsg:  "variable",
		},
		{
			name:    "invalid variable (starts with digit)",
			field:   FormField{Variable: "1bad", Type: FormTypeText, Label: "X"},
			wantErr: true,
			errMsg:  "must not start with a digit",
		},
		{
			name:    "neither type nor preset",
			field:   FormField{Variable: "x", Label: "X"},
			wantErr: true,
			errMsg:  "must specify either type or preset",
		},
		{
			name:    "both type and preset",
			field:   FormField{Variable: "x", Type: FormTypeText, Preset: FormPresetSessionSelector, Label: "X"},
			wantErr: true,
			errMsg:  "cannot specify both type and preset",
		},
		{
			name:    "unknown type",
			field:   FormField{Variable: "x", Type: "unknown", Label: "X"},
			wantErr: true,
			errMsg:  "must be one of",
		},
		{
			name:    "unknown preset",
			field:   FormField{Variable: "x", Preset: "BadPreset", Label: "X"},
			wantErr: true,
			errMsg:  "must be one of",
		},
		{
			name:    "select without options",
			field:   FormField{Variable: "x", Type: FormTypeSelect, Label: "X"},
			wantErr: true,
			errMsg:  "options",
		},
		{
			name:    "multi-select without options",
			field:   FormField{Variable: "x", Type: FormTypeMultiSelect, Label: "X"},
			wantErr: true,
			errMsg:  "options",
		},
		{
			name:    "valid session selector with filter active",
			field:   FormField{Variable: "t", Preset: FormPresetSessionSelector, Label: "T", Filter: FormFilterActive},
			wantErr: false,
		},
		{
			name:    "valid session selector with filter all",
			field:   FormField{Variable: "t", Preset: FormPresetSessionSelector, Label: "T", Filter: FormFilterAll},
			wantErr: false,
		},
		{
			name:    "filter on non-session preset",
			field:   FormField{Variable: "t", Preset: FormPresetProjectSelector, Label: "T", Filter: FormFilterActive},
			wantErr: true,
			errMsg:  "only valid for SessionSelector",
		},
		{
			name:    "invalid filter value",
			field:   FormField{Variable: "t", Preset: FormPresetSessionSelector, Label: "T", Filter: "bogus"},
			wantErr: true,
			errMsg:  "must be one of",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFormField(tt.field)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidate_FormWithAction(t *testing.T) {
	cfg := validConfig(t)
	cfg.UserCommands = map[string]UserCommand{
		"bad": {
			Action: ActionRecycle,
			Form:   []FormField{{Variable: "x", Type: FormTypeText, Label: "X"}},
		},
	}

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "form can only be used with sh, not action")
}

func TestValidate_FormDuplicateVariables(t *testing.T) {
	cfg := validConfig(t)
	cfg.UserCommands = map[string]UserCommand{
		"dup": {
			Sh: "echo test",
			Form: []FormField{
				{Variable: "msg", Type: FormTypeText, Label: "A"},
				{Variable: "msg", Type: FormTypeText, Label: "B"},
			},
		},
	}

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate variable")
}

func TestValidate_FormValidConfig(t *testing.T) {
	cfg := validConfig(t)
	cfg.UserCommands = map[string]UserCommand{
		"broadcast": {
			Sh: "echo {{ .Form.message }}",
			Form: []FormField{
				{Variable: "targets", Preset: FormPresetSessionSelector, Multi: true, Label: "Recipients"},
				{Variable: "message", Type: FormTypeText, Label: "Message", Placeholder: "Type here..."},
			},
		},
	}

	err := cfg.Validate()
	assert.NoError(t, err)
}

func TestBuildValidationData(t *testing.T) {
	t.Run("no form fields returns base data", func(t *testing.T) {
		data := buildValidationData(nil)
		assert.NotEmpty(t, data["Path"])
		assert.NotEmpty(t, data["Name"])
		assert.NotEmpty(t, data["ID"])
		assert.Nil(t, data["Form"])
	})

	t.Run("text field produces string", func(t *testing.T) {
		data := buildValidationData([]FormField{
			{Variable: "msg", Type: FormTypeText},
		})
		form := data["Form"].(map[string]any)
		assert.IsType(t, "", form["msg"])
	})

	t.Run("multi-select produces string slice", func(t *testing.T) {
		data := buildValidationData([]FormField{
			{Variable: "tags", Type: FormTypeMultiSelect},
		})
		form := data["Form"].(map[string]any)
		assert.IsType(t, []string{}, form["tags"])
	})

	t.Run("preset single produces map", func(t *testing.T) {
		data := buildValidationData([]FormField{
			{Variable: "target", Preset: FormPresetSessionSelector},
		})
		form := data["Form"].(map[string]any)
		item, ok := form["target"].(map[string]any)
		require.True(t, ok)
		assert.NotEmpty(t, item["Name"])
	})

	t.Run("preset multi produces slice of maps", func(t *testing.T) {
		data := buildValidationData([]FormField{
			{Variable: "targets", Preset: FormPresetSessionSelector, Multi: true},
		})
		form := data["Form"].(map[string]any)
		items, ok := form["targets"].([]map[string]any)
		require.True(t, ok)
		require.Len(t, items, 1)
		assert.NotEmpty(t, items[0]["Name"])
	})
}

func TestValidateDeep_UserCommandWithFormTemplate(t *testing.T) {
	cfg := validConfig(t)
	cfg.UserCommands = map[string]UserCommand{
		"broadcast": {
			Sh: `{{ range .Form.targets }}echo {{ .Name }}{{ end }}`,
			Form: []FormField{
				{Variable: "targets", Preset: FormPresetSessionSelector, Multi: true, Label: "Targets"},
			},
		},
	}

	err := cfg.ValidateDeep("")
	assert.NoError(t, err)
}

func TestValidate_UserCommandInvalidScope(t *testing.T) {
	cfg := validConfig(t)
	cfg.UserCommands = map[string]UserCommand{
		"bad-scope": {Sh: "echo test", Scope: []string{"invalid"}},
	}

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid scope")
	assert.Contains(t, err.Error(), "must be one of: global, sessions, messages, review")
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
