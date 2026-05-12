package config

import "testing"

func TestDefaultViewsConfig_PromotedKeys(t *testing.T) {
	tests := []struct {
		name string
		got  Keybinding
		want string
	}{
		{name: "sessions g", got: defaultViewsConfig.Sessions.Keybindings["g"], want: "SessionsRefreshGitStatuses"},
		{name: "sessions v", got: defaultViewsConfig.Sessions.Keybindings["v"], want: "SessionsTogglePreview"},
		{name: "sessions up", got: defaultViewsConfig.Sessions.Keybindings["up"], want: "SessionsNavigateUp"},
		{name: "sessions k", got: defaultViewsConfig.Sessions.Keybindings["k"], want: "SessionsNavigateUp"},
		{name: "sessions down", got: defaultViewsConfig.Sessions.Keybindings["down"], want: "SessionsNavigateDown"},
		{name: "sessions j", got: defaultViewsConfig.Sessions.Keybindings["j"], want: "SessionsNavigateDown"},
		{name: "sessions /", got: defaultViewsConfig.Sessions.Keybindings["/"], want: "SessionsFilterStart"},
		{name: "sessions :", got: defaultViewsConfig.Sessions.Keybindings[":"], want: "SessionsCommandPaletteOpen"},
		{name: "global q", got: defaultViewsConfig.Global.Keybindings["q"], want: "Quit"},
		{name: "global ?", got: defaultViewsConfig.Global.Keybindings["?"], want: "ShowHelp"},
		{name: "tasks g", got: defaultViewsConfig.Tasks.Keybindings["g"], want: "GoToTop"},
		{name: "tasks G", got: defaultViewsConfig.Tasks.Keybindings["G"], want: "GoToBottom"},
		{name: "review g", got: defaultViewsConfig.Review.Keybindings["g"], want: "GoToTop"},
		{name: "review G", got: defaultViewsConfig.Review.Keybindings["G"], want: "GoToBottom"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got.Cmd != tt.want {
				t.Fatalf("keybinding cmd = %q, want %q", tt.got.Cmd, tt.want)
			}
		})
	}
}

func TestDefaultViewsConfig_PromotedKeysHaveHelp(t *testing.T) {
	tests := []struct {
		name string
		key  Keybinding
	}{
		{name: "sessions g", key: defaultViewsConfig.Sessions.Keybindings["g"]},
		{name: "sessions v", key: defaultViewsConfig.Sessions.Keybindings["v"]},
		{name: "sessions up", key: defaultViewsConfig.Sessions.Keybindings["up"]},
		{name: "sessions k", key: defaultViewsConfig.Sessions.Keybindings["k"]},
		{name: "sessions down", key: defaultViewsConfig.Sessions.Keybindings["down"]},
		{name: "sessions j", key: defaultViewsConfig.Sessions.Keybindings["j"]},
		{name: "sessions /", key: defaultViewsConfig.Sessions.Keybindings["/"]},
		{name: "sessions :", key: defaultViewsConfig.Sessions.Keybindings[":"]},
		{name: "global q", key: defaultViewsConfig.Global.Keybindings["q"]},
		{name: "global ?", key: defaultViewsConfig.Global.Keybindings["?"]},
		{name: "tasks g", key: defaultViewsConfig.Tasks.Keybindings["g"]},
		{name: "tasks G", key: defaultViewsConfig.Tasks.Keybindings["G"]},
		{name: "review g", key: defaultViewsConfig.Review.Keybindings["g"]},
		{name: "review G", key: defaultViewsConfig.Review.Keybindings["G"]},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, ok := defaultUserCommands[tt.key.Cmd]
			if !ok {
				t.Fatalf("default keybinding references unknown command %q", tt.key.Cmd)
			}
			if cmd.Help == "" {
				t.Fatalf("default command %q has empty help text", tt.key.Cmd)
			}
		})
	}
}
