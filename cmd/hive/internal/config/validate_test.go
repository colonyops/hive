package config

import (
	"testing"

	"github.com/colonyops/hive/cmd/hive/internal/action"
	"github.com/colonyops/hive/cmd/hive/internal/theme"
	"github.com/colonyops/hive/internal/config"
	"github.com/hay-kot/criterio"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// validConfig returns a Config with all required fields set for testing.
func validConfig(t *testing.T) *Config {
	t.Helper()
	return &Config{
		Config: config.Config{
			GitPath: "git",
			DataDir: t.TempDir(),
			Git:     config.GitConfig{StatusWorkers: 1},
			Agents: config.AgentsConfig{
				Default:  "claude",
				Profiles: map[string]config.AgentProfile{"claude": {}},
			},
			Database: config.DatabaseConfig{
				MaxOpenConns: 2,
				MaxIdleConns: 2,
				BusyTimeout:  5000,
			},
		},
		TUI:   TUIConfig{Theme: theme.Default},
		Views: ViewsConfig{Sessions: SessionsViewConfig{GroupBy: GroupByRepo}},
	}
}

func TestValidateDeep_ValidConfig(t *testing.T) {
	cfg := validConfig(t)
	cfg.Rules = []config.Rule{
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

func TestValidateUserKeybindings_EmptyCmd(t *testing.T) {
	cfg := validConfig(t)
	cfg.Keybindings = map[string]Keybinding{
		"x": {Help: "does nothing"},
	}

	err := cfg.validateUserKeybindings()

	var fieldErrs criterio.FieldErrors
	require.ErrorAs(t, err, &fieldErrs)
	assert.Len(t, fieldErrs, 1)
	assert.Contains(t, fieldErrs[0].Err.Error(), "cmd is required")
}

func TestValidateUserKeybindings_PluginCommandAllowed(t *testing.T) {
	cfg := validConfig(t)
	// Plugin commands (registered at runtime) must be allowed at config-load time.
	// Both commands in default keybindings and arbitrary plugin commands should pass.
	cfg.Keybindings = map[string]Keybinding{
		"t": {Cmd: "TmuxOpen"},        // in default keybindings
		"g": {Cmd: "GithubOpenPR"},    // plugin command not in defaults
		"l": {Cmd: "LazyGitOpen"},     // plugin command not in defaults
		"u": {Cmd: "UnknownFuturCmd"}, // unknown — allowed, caught at invocation time
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
		"bad": {Action: action.TypeRecycle, Sh: "echo test"},
	}

	err := cfg.ValidateDeep("")

	var fieldErrs criterio.FieldErrors
	require.ErrorAs(t, err, &fieldErrs)
	assert.Len(t, fieldErrs, 1)
	assert.Contains(t, fieldErrs[0].Err.Error(), "action is mutually exclusive with sh and windows")
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
		"my-recycle": {Action: action.TypeRecycle, Help: "custom recycle"},
		"my-delete":  {Action: action.TypeDelete, Help: "custom delete"},
		"my-source":  {Action: action.TypeOpenSourcePicker, Args: []string{"issues"}, Help: "custom source"},
	}

	err := cfg.ValidateDeep("")
	assert.NoError(t, err)
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
	assert.Contains(t, fieldErrs[0].Err.Error(), "must have at least one of: action, sh, windows")
}

func TestValidate_UserCommandWindowsOnly(t *testing.T) {
	cfg := validConfig(t)
	cfg.UserCommands = map[string]UserCommand{
		"spawn": {
			Help: "spawn windows",
			Windows: []config.WindowConfig{
				{Name: "agent", Command: "claude", Focus: true},
			},
		},
	}

	err := cfg.Validate()
	assert.NoError(t, err)
}

func TestValidate_UserCommandWindowCommandAndPanes(t *testing.T) {
	cfg := validConfig(t)
	cfg.UserCommands = map[string]UserCommand{
		"spawn": {
			Windows: []config.WindowConfig{{Name: "agent", Command: "claude", Panes: []config.PaneConfig{{Command: "npm test"}}}},
		},
	}

	err := cfg.Validate()
	var fieldErrs criterio.FieldErrors
	require.ErrorAs(t, err, &fieldErrs)
	assert.Contains(t, fieldErrs[0].Field, "windows[0]")
	assert.Contains(t, fieldErrs[0].Err.Error(), "command and panes are mutually exclusive")
}

func TestValidate_UserCommandWindowInvalidPaneSplit(t *testing.T) {
	cfg := validConfig(t)
	cfg.UserCommands = map[string]UserCommand{
		"spawn": {
			Windows: []config.WindowConfig{{Name: "agent", Panes: []config.PaneConfig{{Split: "diagonal"}}}},
		},
	}

	err := cfg.Validate()
	var fieldErrs criterio.FieldErrors
	require.ErrorAs(t, err, &fieldErrs)
	assert.Contains(t, fieldErrs[0].Field, "windows[0].panes[0].split")
	assert.Contains(t, fieldErrs[0].Err.Error(), "must be one of: horizontal, vertical")
}

func TestValidate_UserCommandWindowsMissingName(t *testing.T) {
	cfg := validConfig(t)
	cfg.UserCommands = map[string]UserCommand{
		"spawn": {
			Windows: []config.WindowConfig{
				{Command: "claude"},
			},
		},
	}

	err := cfg.Validate()
	var fieldErrs criterio.FieldErrors
	require.ErrorAs(t, err, &fieldErrs)
	assert.Contains(t, fieldErrs[0].Field, "windows[0]")
	assert.Contains(t, fieldErrs[0].Err.Error(), "name is required")
}

func TestValidate_UserCommandOptionsRemoteRequiresSessionName(t *testing.T) {
	cfg := validConfig(t)
	cfg.UserCommands = map[string]UserCommand{
		"spawn": {
			Windows: []config.WindowConfig{{Name: "agent"}},
			Options: UserCommandOptions{Remote: "https://github.com/org/repo"},
		},
	}

	err := cfg.Validate()
	var fieldErrs criterio.FieldErrors
	require.ErrorAs(t, err, &fieldErrs)
	assert.Contains(t, fieldErrs[0].Field, "options.remote")
	assert.Contains(t, fieldErrs[0].Err.Error(), "remote requires session_name to be set")
}

func TestValidate_UserCommandOptionsSessionNameRequiresWindows(t *testing.T) {
	cfg := validConfig(t)
	cfg.UserCommands = map[string]UserCommand{
		"cmd": {Sh: "echo hi", Options: UserCommandOptions{SessionName: "my-session"}},
	}

	err := cfg.Validate()
	var fieldErrs criterio.FieldErrors
	require.ErrorAs(t, err, &fieldErrs)
	assert.Contains(t, fieldErrs[0].Field, "options.session_name")
	assert.Contains(t, fieldErrs[0].Err.Error(), "session_name only applies when windows are defined")
}

func TestValidate_UserCommandOptionsBackgroundRequiresWindows(t *testing.T) {
	cfg := validConfig(t)
	cfg.UserCommands = map[string]UserCommand{
		"cmd": {Sh: "echo hi", Options: UserCommandOptions{Background: true}},
	}

	err := cfg.Validate()
	var fieldErrs criterio.FieldErrors
	require.ErrorAs(t, err, &fieldErrs)
	assert.Contains(t, fieldErrs[0].Field, "options.background")
	assert.Contains(t, fieldErrs[0].Err.Error(), "background only applies when windows are defined")
}

func TestValidateDeep_UserCommandWindowTemplates(t *testing.T) {
	t.Run("valid window templates", func(t *testing.T) {
		cfg := validConfig(t)
		cfg.UserCommands = map[string]UserCommand{
			"spawn": {
				Windows: []config.WindowConfig{
					{
						Name: "{{ .Name }}-agent",
						Panes: []config.PaneConfig{
							{Command: "claude --session {{ .ID }}"},
							{Command: "npm test {{ .Form.target }}", Size: "30%"},
						},
					},
				},
				Form: []FormField{{Variable: "target", Label: "Target", Type: "text"}},
			},
		}

		err := cfg.ValidateDeep("")
		assert.NoError(t, err)
	})

	t.Run("invalid name template", func(t *testing.T) {
		cfg := validConfig(t)
		cfg.UserCommands = map[string]UserCommand{
			"spawn": {
				Windows: []config.WindowConfig{
					{Name: "{{ .Invalid }}"},
				},
			},
		}

		err := cfg.ValidateDeep("")
		var fieldErrs criterio.FieldErrors
		require.ErrorAs(t, err, &fieldErrs)
		assert.Contains(t, fieldErrs[0].Field, "windows[0].name")
	})

	t.Run("invalid options.session_name template", func(t *testing.T) {
		cfg := validConfig(t)
		cfg.UserCommands = map[string]UserCommand{
			"spawn": {
				Windows: []config.WindowConfig{{Name: "agent"}},
				Options: UserCommandOptions{SessionName: "{{ .Invalid }}"},
			},
		}

		err := cfg.ValidateDeep("")
		var fieldErrs criterio.FieldErrors
		require.ErrorAs(t, err, &fieldErrs)
		assert.Contains(t, fieldErrs[0].Field, "options.session_name")
	})
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
		"tasks-cmd":    {Sh: "echo test", Scope: []string{"tasks"}},
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
			Action: action.TypeRecycle,
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
		doc, ok := data["Doc"].(map[string]any)
		require.True(t, ok, "Doc should be a map for nested template access")
		assert.NotEmpty(t, doc["Path"])
		assert.NotEmpty(t, doc["RelPath"])
		assert.NotEmpty(t, doc["Type"])
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

func TestValidateDeep_UserCommandWithDocTemplate(t *testing.T) {
	cfg := validConfig(t)
	cfg.UserCommands = map[string]UserCommand{
		"copy-doc": {
			Sh:    "echo {{ .Doc.Path }} {{ .Doc.RelPath }} {{ .Doc.Type }}",
			Scope: []string{"review"},
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
	assert.Contains(t, err.Error(), "must be one of: global, sessions, messages, review, todos, tasks")
}

func TestValidate_GroupByInvalid(t *testing.T) {
	cfg := validConfig(t)
	cfg.Views.Sessions.GroupBy = "invalid"

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "views.sessions.group_by")
}

func TestValidate_GroupByValidModes(t *testing.T) {
	for _, mode := range ValidGroupByModes {
		t.Run(mode, func(t *testing.T) {
			cfg := validConfig(t)
			cfg.Views.Sessions.GroupBy = mode

			err := cfg.Validate()
			assert.NoError(t, err)
		})
	}
}
