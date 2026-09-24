package config

import (
	"testing"

	"github.com/colonyops/hive/internal/core/action"
)

func TestDefaultUserCommandsIncludesWorkspaceRefresh(t *testing.T) {
	cmd, ok := defaultUserCommands["SessionsRefreshWorkspaces"]
	if !ok {
		t.Fatal("SessionsRefreshWorkspaces command is not registered")
	}
	if cmd.Action != action.TypeSessionsRefreshWorkspaces {
		t.Fatalf("action = %q, want %q", cmd.Action, action.TypeSessionsRefreshWorkspaces)
	}
	if len(cmd.Scope) != 1 || cmd.Scope[0] != "sessions" {
		t.Fatalf("scope = %v, want [sessions]", cmd.Scope)
	}
}

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
			cmd, ok := defaultUserCommands[tt.got.Cmd]
			if !ok {
				t.Fatalf("default keybinding references unknown command %q", tt.got.Cmd)
			}
			if cmd.Help == "" {
				t.Fatalf("default command %q has empty help text", tt.got.Cmd)
			}
		})
	}
}
