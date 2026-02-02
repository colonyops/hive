package tui

import (
	"testing"

	"github.com/hay-kot/hive/internal/core/config"
	"github.com/hay-kot/hive/internal/core/session"
)

func TestKeybindingHandler_Resolve_RecycledSession(t *testing.T) {
	commands := map[string]config.UserCommand{
		"Delete":  {Action: config.ActionDelete, Help: "delete"},
		"Recycle": {Action: config.ActionRecycle, Help: "recycle"},
		"open":    {Sh: "code {{ .Path }}", Help: "open in vscode"},
	}
	keybindings := map[string]config.Keybinding{
		"d": {Cmd: "Delete"},
		"r": {Cmd: "Recycle"},
		"o": {Cmd: "open"},
	}

	handler := NewKeybindingHandler(keybindings, commands, nil)

	activeSession := session.Session{
		ID:    "test-id",
		Path:  "/test/path",
		State: session.StateActive,
	}

	recycledSession := session.Session{
		ID:    "test-id",
		Path:  "/test/path",
		State: session.StateRecycled,
	}

	tests := []struct {
		name    string
		key     string
		sess    session.Session
		wantOK  bool
		wantTyp ActionType
	}{
		{
			name:    "active session allows delete",
			key:     "d",
			sess:    activeSession,
			wantOK:  true,
			wantTyp: ActionTypeDelete,
		},
		{
			name:    "active session allows recycle",
			key:     "r",
			sess:    activeSession,
			wantOK:  true,
			wantTyp: ActionTypeRecycle,
		},
		{
			name:    "active session allows shell command",
			key:     "o",
			sess:    activeSession,
			wantOK:  true,
			wantTyp: ActionTypeShell,
		},
		{
			name:    "recycled session allows delete",
			key:     "d",
			sess:    recycledSession,
			wantOK:  true,
			wantTyp: ActionTypeDelete,
		},
		{
			name:   "recycled session blocks recycle",
			key:    "r",
			sess:   recycledSession,
			wantOK: false,
		},
		{
			name:   "recycled session blocks shell command",
			key:    "o",
			sess:   recycledSession,
			wantOK: false,
		},
		{
			name:   "unknown key returns false",
			key:    "x",
			sess:   activeSession,
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			action, ok := handler.Resolve(tt.key, tt.sess)
			if ok != tt.wantOK {
				t.Errorf("Resolve() ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && action.Type != tt.wantTyp {
				t.Errorf("Resolve() action.Type = %v, want %v", action.Type, tt.wantTyp)
			}
		})
	}
}

func TestKeybindingHandler_ResolveUserCommand(t *testing.T) {
	handler := NewKeybindingHandler(nil, nil, nil)

	sess := session.Session{
		ID:     "test-id",
		Path:   "/test/path",
		Name:   "test-name",
		Remote: "https://github.com/test/repo",
	}

	tests := []struct {
		name        string
		cmdName     string
		cmd         config.UserCommand
		args        []string
		wantKey     string
		wantHelp    string
		wantExit    bool
		wantType    ActionType
		wantCmdPart string
	}{
		{
			name:    "basic shell command",
			cmdName: "review",
			cmd: config.UserCommand{
				Sh:   "send-claude {{ .Name }} /review",
				Help: "Send to Claude for review",
			},
			wantKey:  ":review",
			wantHelp: "Send to Claude for review",
			wantExit: false,
			wantType: ActionTypeShell,
		},
		{
			name:    "shell command with exit",
			cmdName: "open",
			cmd: config.UserCommand{
				Sh:   "open {{ .Path }}",
				Exit: "true",
			},
			wantKey:  ":open",
			wantExit: true,
			wantType: ActionTypeShell,
		},
		{
			name:    "shell command with args",
			cmdName: "deploy",
			cmd: config.UserCommand{
				Sh:   "deploy {{ index .Args 0 }} {{ index .Args 1 }}",
				Help: "Deploy to environment",
			},
			args:        []string{"staging", "--force"},
			wantKey:     ":deploy",
			wantHelp:    "Deploy to environment",
			wantExit:    false,
			wantType:    ActionTypeShell,
			wantCmdPart: "deploy staging --force",
		},
		{
			name:    "action-based recycle command",
			cmdName: "Recycle",
			cmd: config.UserCommand{
				Action: config.ActionRecycle,
				Help:   "custom recycle help",
			},
			wantKey:  ":Recycle",
			wantHelp: "custom recycle help",
			wantType: ActionTypeRecycle,
		},
		{
			name:    "action-based delete command",
			cmdName: "Delete",
			cmd: config.UserCommand{
				Action: config.ActionDelete,
			},
			wantKey:  ":Delete",
			wantHelp: "delete", // default help for delete action
			wantType: ActionTypeDelete,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			action := handler.ResolveUserCommand(tt.cmdName, tt.cmd, sess, tt.args)

			if action.Key != tt.wantKey {
				t.Errorf("ResolveUserCommand() Key = %v, want %v", action.Key, tt.wantKey)
			}
			if action.Help != tt.wantHelp {
				t.Errorf("ResolveUserCommand() Help = %v, want %v", action.Help, tt.wantHelp)
			}
			if action.Exit != tt.wantExit {
				t.Errorf("ResolveUserCommand() Exit = %v, want %v", action.Exit, tt.wantExit)
			}
			if action.Type != tt.wantType {
				t.Errorf("ResolveUserCommand() Type = %v, want %v", action.Type, tt.wantType)
			}
			if tt.wantCmdPart != "" && action.ShellCmd != tt.wantCmdPart {
				t.Errorf("ResolveUserCommand() ShellCmd = %v, want %v", action.ShellCmd, tt.wantCmdPart)
			}
		})
	}
}

func TestKeybindingHandler_Resolve_Overrides(t *testing.T) {
	commands := map[string]config.UserCommand{
		"Recycle": {
			Action:  config.ActionRecycle,
			Help:    "command help",
			Confirm: "command confirm",
		},
		"shell-cmd": {
			Sh:      "echo test",
			Help:    "shell help",
			Confirm: "shell confirm",
			Silent:  true,
		},
	}

	sess := session.Session{
		ID:    "test-id",
		Path:  "/test/path",
		State: session.StateActive,
	}

	t.Run("keybinding help overrides command help", func(t *testing.T) {
		keybindings := map[string]config.Keybinding{
			"r": {Cmd: "Recycle", Help: "keybinding help"},
		}
		handler := NewKeybindingHandler(keybindings, commands, nil)

		action, ok := handler.Resolve("r", sess)
		if !ok {
			t.Fatal("expected ok = true")
		}
		if action.Help != "keybinding help" {
			t.Errorf("Help = %q, want %q", action.Help, "keybinding help")
		}
	})

	t.Run("keybinding confirm overrides command confirm", func(t *testing.T) {
		keybindings := map[string]config.Keybinding{
			"r": {Cmd: "Recycle", Confirm: "keybinding confirm"},
		}
		handler := NewKeybindingHandler(keybindings, commands, nil)

		action, ok := handler.Resolve("r", sess)
		if !ok {
			t.Fatal("expected ok = true")
		}
		if action.Confirm != "keybinding confirm" {
			t.Errorf("Confirm = %q, want %q", action.Confirm, "keybinding confirm")
		}
	})

	t.Run("command values used when keybinding doesn't override", func(t *testing.T) {
		keybindings := map[string]config.Keybinding{
			"s": {Cmd: "shell-cmd"},
		}
		handler := NewKeybindingHandler(keybindings, commands, nil)

		action, ok := handler.Resolve("s", sess)
		if !ok {
			t.Fatal("expected ok = true")
		}
		if action.Help != "shell help" {
			t.Errorf("Help = %q, want %q", action.Help, "shell help")
		}
		if action.Confirm != "shell confirm" {
			t.Errorf("Confirm = %q, want %q", action.Confirm, "shell confirm")
		}
		if !action.Silent {
			t.Error("Silent = false, want true")
		}
	})

	t.Run("invalid command reference returns false", func(t *testing.T) {
		keybindings := map[string]config.Keybinding{
			"x": {Cmd: "NonExistent"},
		}
		handler := NewKeybindingHandler(keybindings, commands, nil)

		_, ok := handler.Resolve("x", sess)
		if ok {
			t.Error("expected ok = false for invalid command reference")
		}
	})

	t.Run("command with neither action nor sh returns false", func(t *testing.T) {
		// This shouldn't happen with valid config, but test defensive behavior
		emptyCommands := map[string]config.UserCommand{
			"empty": {Help: "an empty command"},
		}
		keybindings := map[string]config.Keybinding{
			"e": {Cmd: "empty"},
		}
		handler := NewKeybindingHandler(keybindings, emptyCommands, nil)

		_, ok := handler.Resolve("e", sess)
		if ok {
			t.Error("expected ok = false for command with neither action nor sh")
		}
	})

	t.Run("template error sets Err field", func(t *testing.T) {
		badTemplateCommands := map[string]config.UserCommand{
			"bad": {Sh: "echo {{ .InvalidField }}"},
		}
		keybindings := map[string]config.Keybinding{
			"b": {Cmd: "bad"},
		}
		handler := NewKeybindingHandler(keybindings, badTemplateCommands, nil)

		action, ok := handler.Resolve("b", sess)
		if !ok {
			t.Fatal("expected ok = true even with template error")
		}
		if action.Err == nil {
			t.Error("expected action.Err to be non-nil for template error")
		}
		if action.ShellCmd != "" {
			t.Errorf("expected empty ShellCmd, got %q", action.ShellCmd)
		}
	})
}

func TestKeybindingHandler_HelpEntries(t *testing.T) {
	commands := map[string]config.UserCommand{
		"Recycle": {Action: config.ActionRecycle, Help: "recycle session"},
		"Delete":  {Action: config.ActionDelete}, // no help, should use action name
		"open":    {Sh: "code {{ .Path }}", Help: "open in editor"},
	}

	t.Run("uses command help text", func(t *testing.T) {
		keybindings := map[string]config.Keybinding{
			"o": {Cmd: "open"},
			"r": {Cmd: "Recycle"},
		}
		handler := NewKeybindingHandler(keybindings, commands, nil)

		entries := handler.HelpEntries()
		// Entries are sorted by key
		if len(entries) != 2 {
			t.Fatalf("expected 2 entries, got %d", len(entries))
		}
		if entries[0] != "[o] open in editor" {
			t.Errorf("entries[0] = %q, want %q", entries[0], "[o] open in editor")
		}
		if entries[1] != "[r] recycle session" {
			t.Errorf("entries[1] = %q, want %q", entries[1], "[r] recycle session")
		}
	})

	t.Run("keybinding help overrides command help", func(t *testing.T) {
		keybindings := map[string]config.Keybinding{
			"r": {Cmd: "Recycle", Help: "custom help"},
		}
		handler := NewKeybindingHandler(keybindings, commands, nil)

		entries := handler.HelpEntries()
		if len(entries) != 1 {
			t.Fatalf("expected 1 entry, got %d", len(entries))
		}
		if entries[0] != "[r] custom help" {
			t.Errorf("entries[0] = %q, want %q", entries[0], "[r] custom help")
		}
	})

	t.Run("uses action name when help empty", func(t *testing.T) {
		keybindings := map[string]config.Keybinding{
			"d": {Cmd: "Delete"},
		}
		handler := NewKeybindingHandler(keybindings, commands, nil)

		entries := handler.HelpEntries()
		if len(entries) != 1 {
			t.Fatalf("expected 1 entry, got %d", len(entries))
		}
		if entries[0] != "[d] delete" {
			t.Errorf("entries[0] = %q, want %q", entries[0], "[d] delete")
		}
	})

	t.Run("unknown fallback for invalid command reference", func(t *testing.T) {
		keybindings := map[string]config.Keybinding{
			"x": {Cmd: "NonExistent"},
		}
		handler := NewKeybindingHandler(keybindings, commands, nil)

		entries := handler.HelpEntries()
		if len(entries) != 1 {
			t.Fatalf("expected 1 entry, got %d", len(entries))
		}
		if entries[0] != "[x] unknown" {
			t.Errorf("entries[0] = %q, want %q", entries[0], "[x] unknown")
		}
	})
}
