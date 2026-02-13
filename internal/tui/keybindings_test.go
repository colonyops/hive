package tui

import (
	"testing"

	"github.com/hay-kot/hive/internal/core/config"
	"github.com/hay-kot/hive/internal/core/session"
	"github.com/hay-kot/hive/pkg/tmpl"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testRenderer = tmpl.New(tmpl.Config{})

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

	handler := NewKeybindingResolver(keybindings, commands, testRenderer)

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
			assert.Equal(t, tt.wantOK, ok, "Resolve() ok = %v, want %v", ok, tt.wantOK)
			if ok {
				assert.Equal(t, tt.wantTyp, action.Type, "Resolve() action.Type = %v, want %v", action.Type, tt.wantTyp)
			}
		})
	}
}

func TestKeybindingHandler_ResolveUserCommand(t *testing.T) {
	handler := NewKeybindingResolver(nil, nil, testRenderer)

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

			assert.Equal(t, tt.wantKey, action.Key, "ResolveUserCommand() Key = %v, want %v", action.Key, tt.wantKey)
			assert.Equal(t, tt.wantHelp, action.Help, "ResolveUserCommand() Help = %v, want %v", action.Help, tt.wantHelp)
			assert.Equal(t, tt.wantExit, action.Exit, "ResolveUserCommand() Exit = %v, want %v", action.Exit, tt.wantExit)
			assert.Equal(t, tt.wantType, action.Type, "ResolveUserCommand() Type = %v, want %v", action.Type, tt.wantType)
			if tt.wantCmdPart != "" {
				assert.Equal(t, tt.wantCmdPart, action.ShellCmd, "ResolveUserCommand() ShellCmd = %v, want %v", action.ShellCmd, tt.wantCmdPart)
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
		handler := NewKeybindingResolver(keybindings, commands, testRenderer)

		action, ok := handler.Resolve("r", sess)
		require.True(t, ok, "expected ok = true")
		assert.Equal(t, "keybinding help", action.Help)
	})

	t.Run("keybinding confirm overrides command confirm", func(t *testing.T) {
		keybindings := map[string]config.Keybinding{
			"r": {Cmd: "Recycle", Confirm: "keybinding confirm"},
		}
		handler := NewKeybindingResolver(keybindings, commands, testRenderer)

		action, ok := handler.Resolve("r", sess)
		require.True(t, ok, "expected ok = true")
		assert.Equal(t, "keybinding confirm", action.Confirm)
	})

	t.Run("command values used when keybinding doesn't override", func(t *testing.T) {
		keybindings := map[string]config.Keybinding{
			"s": {Cmd: "shell-cmd"},
		}
		handler := NewKeybindingResolver(keybindings, commands, testRenderer)

		action, ok := handler.Resolve("s", sess)
		require.True(t, ok, "expected ok = true")
		assert.Equal(t, "shell help", action.Help)
		assert.Equal(t, "shell confirm", action.Confirm)
		assert.True(t, action.Silent, "Silent = false, want true")
	})

	t.Run("invalid command reference returns false", func(t *testing.T) {
		keybindings := map[string]config.Keybinding{
			"x": {Cmd: "NonExistent"},
		}
		handler := NewKeybindingResolver(keybindings, commands, testRenderer)

		_, ok := handler.Resolve("x", sess)
		assert.False(t, ok, "expected ok = false for invalid command reference")
	})

	t.Run("command with neither action nor sh returns false", func(t *testing.T) {
		// This shouldn't happen with valid config, but test defensive behavior
		emptyCommands := map[string]config.UserCommand{
			"empty": {Help: "an empty command"},
		}
		keybindings := map[string]config.Keybinding{
			"e": {Cmd: "empty"},
		}
		handler := NewKeybindingResolver(keybindings, emptyCommands, testRenderer)

		_, ok := handler.Resolve("e", sess)
		assert.False(t, ok, "expected ok = false for command with neither action nor sh")
	})

	t.Run("template error sets Err field", func(t *testing.T) {
		badTemplateCommands := map[string]config.UserCommand{
			"bad": {Sh: "echo {{ .InvalidField }}"},
		}
		keybindings := map[string]config.Keybinding{
			"b": {Cmd: "bad"},
		}
		handler := NewKeybindingResolver(keybindings, badTemplateCommands, testRenderer)

		action, ok := handler.Resolve("b", sess)
		require.True(t, ok, "expected ok = true even with template error")
		require.Error(t, action.Err, "expected action.Err to be non-nil for template error")
		assert.Empty(t, action.ShellCmd, "expected empty ShellCmd, got %q", action.ShellCmd)
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
		handler := NewKeybindingResolver(keybindings, commands, testRenderer)

		entries := handler.HelpEntries()
		// Entries are sorted by key
		require.Len(t, entries, 2, "expected 2 entries, got %d", len(entries))
		assert.Equal(t, "[o] open in editor", entries[0])
		assert.Equal(t, "[r] recycle session", entries[1])
	})

	t.Run("keybinding help overrides command help", func(t *testing.T) {
		keybindings := map[string]config.Keybinding{
			"r": {Cmd: "Recycle", Help: "custom help"},
		}
		handler := NewKeybindingResolver(keybindings, commands, testRenderer)

		entries := handler.HelpEntries()
		require.Len(t, entries, 1, "expected 1 entry, got %d", len(entries))
		assert.Equal(t, "[r] custom help", entries[0])
	})

	t.Run("uses action name when help empty", func(t *testing.T) {
		keybindings := map[string]config.Keybinding{
			"d": {Cmd: "Delete"},
		}
		handler := NewKeybindingResolver(keybindings, commands, testRenderer)

		entries := handler.HelpEntries()
		require.Len(t, entries, 1, "expected 1 entry, got %d", len(entries))
		assert.Equal(t, "[d] delete", entries[0])
	})

	t.Run("invalid command reference is filtered out", func(t *testing.T) {
		keybindings := map[string]config.Keybinding{
			"x": {Cmd: "NonExistent"},
		}
		handler := NewKeybindingResolver(keybindings, commands, testRenderer)

		entries := handler.HelpEntries()
		// Invalid command references are filtered out (not shown in help)
		assert.Empty(t, entries, "expected 0 entries, got %d", len(entries))
	})
}

func TestKeybindingResolver_TmuxWindowAndTool(t *testing.T) {
	commands := map[string]config.UserCommand{
		"window-cmd": {Sh: "echo {{ .TmuxWindow }} {{ .Tool }}", Help: "test"},
	}
	keybindings := map[string]config.Keybinding{
		"w": {Cmd: "window-cmd"},
	}

	sess := session.Session{
		ID:    "test-id",
		Path:  "/test/path",
		State: session.StateActive,
	}

	t.Run("uses lookup functions for TmuxWindow and Tool", func(t *testing.T) {
		handler := NewKeybindingResolver(keybindings, commands, testRenderer)
		handler.SetTmuxWindowLookup(func(id string) string { return "claude-window" })
		handler.SetToolLookup(func(id string) string { return "claude" })

		action, ok := handler.Resolve("w", sess)
		require.True(t, ok, "expected ok = true")
		assert.Equal(t, "echo claude-window claude", action.ShellCmd)
	})

	t.Run("SetSelectedWindow overrides lookup", func(t *testing.T) {
		handler := NewKeybindingResolver(keybindings, commands, testRenderer)
		handler.SetTmuxWindowLookup(func(id string) string { return "default-window" })
		handler.SetToolLookup(func(id string) string { return "claude" })
		handler.SetSelectedWindow("override-window")

		action, ok := handler.Resolve("w", sess)
		require.True(t, ok, "expected ok = true")
		assert.Equal(t, "echo override-window claude", action.ShellCmd)
	})

	t.Run("override is consumed after resolve", func(t *testing.T) {
		handler := NewKeybindingResolver(keybindings, commands, testRenderer)
		handler.SetTmuxWindowLookup(func(id string) string { return "default-window" })
		handler.SetToolLookup(func(id string) string { return "claude" })
		handler.SetSelectedWindow("override-window")

		// First resolve consumes the override
		action, ok := handler.Resolve("w", sess)
		require.True(t, ok, "expected ok = true")
		assert.Equal(t, "echo override-window claude", action.ShellCmd, "first resolve")

		// Second resolve should use the lookup (override was consumed)
		action, ok = handler.Resolve("w", sess)
		require.True(t, ok, "expected ok = true")
		assert.Equal(t, "echo default-window claude", action.ShellCmd, "second resolve")
	})

	t.Run("ResolveUserCommand also consumes override", func(t *testing.T) {
		handler := NewKeybindingResolver(nil, nil, testRenderer)
		handler.SetTmuxWindowLookup(func(id string) string { return "default-window" })
		handler.SetToolLookup(func(id string) string { return "claude" })
		handler.SetSelectedWindow("override-window")

		cmd := config.UserCommand{Sh: "echo {{ .TmuxWindow }}", Help: "test"}
		action := handler.ResolveUserCommand("test", cmd, sess, nil)
		assert.Equal(t, "echo override-window", action.ShellCmd)

		// Override should be consumed
		action = handler.ResolveUserCommand("test", cmd, sess, nil)
		assert.Equal(t, "echo default-window", action.ShellCmd, "after consume")
	})
}

func TestKeybindingResolver_RenderWithFormData(t *testing.T) {
	handler := NewKeybindingResolver(nil, nil, testRenderer)

	sess := session.Session{
		ID:     "test-id",
		Path:   "/test/path",
		Name:   "test-name",
		Remote: "https://github.com/test/repo",
	}

	t.Run("injects form data under .Form namespace", func(t *testing.T) {
		cmd := config.UserCommand{
			Sh:   "echo {{ .Form.message }}",
			Help: "test",
		}
		formData := map[string]any{
			"message": "hello world",
		}

		action := handler.RenderWithFormData("test", cmd, sess, nil, formData)
		assert.Equal(t, ActionTypeShell, action.Type)
		assert.Equal(t, "echo hello world", action.ShellCmd)
		assert.NoError(t, action.Err)
	})

	t.Run("form data coexists with session fields", func(t *testing.T) {
		cmd := config.UserCommand{
			Sh:   "echo {{ .Name }} {{ .Form.env }}",
			Help: "test",
		}
		formData := map[string]any{
			"env": "staging",
		}

		action := handler.RenderWithFormData("test", cmd, sess, nil, formData)
		assert.Equal(t, "echo test-name staging", action.ShellCmd)
	})

	t.Run("template error sets Err", func(t *testing.T) {
		cmd := config.UserCommand{
			Sh:   "echo {{ .Form.missing }}",
			Help: "test",
		}
		formData := map[string]any{}

		action := handler.RenderWithFormData("test", cmd, sess, nil, formData)
		require.Error(t, action.Err)
		assert.Empty(t, action.ShellCmd)
	})
}

func TestKeybindingResolver_Scope(t *testing.T) {
	commands := map[string]config.UserCommand{
		"global-cmd":   {Sh: "echo global", Help: "global command"},
		"review-cmd":   {Sh: "echo review", Help: "review command", Scope: []string{"review"}},
		"sessions-cmd": {Sh: "echo sessions", Help: "sessions command", Scope: []string{"sessions"}},
		"multi-cmd":    {Sh: "echo multi", Help: "multi command", Scope: []string{"review", "messages"}},
	}
	keybindings := map[string]config.Keybinding{
		"g": {Cmd: "global-cmd"},
		"r": {Cmd: "review-cmd"},
		"s": {Cmd: "sessions-cmd"},
		"m": {Cmd: "multi-cmd"},
	}

	handler := NewKeybindingResolver(keybindings, commands, testRenderer)
	sess := session.Session{
		ID:    "test-id",
		Path:  "/test/path",
		State: session.StateActive,
	}

	t.Run("global command works in all views", func(t *testing.T) {
		for _, view := range []ViewType{ViewSessions, ViewMessages, ViewReview} {
			handler.SetActiveView(view)
			action, ok := handler.Resolve("g", sess)
			assert.True(t, ok, "global command should work in view %s", view.String())
			assert.Equal(t, ActionTypeShell, action.Type)
		}
	})

	t.Run("scoped command only works in its scope", func(t *testing.T) {
		// review-cmd should only work in review view
		handler.SetActiveView(ViewReview)
		action, ok := handler.Resolve("r", sess)
		assert.True(t, ok, "review-cmd should work in review view")
		assert.Equal(t, ActionTypeShell, action.Type)

		// Should not work in sessions view
		handler.SetActiveView(ViewSessions)
		_, ok = handler.Resolve("r", sess)
		assert.False(t, ok, "review-cmd should not work in sessions view")

		// Should not work in messages view
		handler.SetActiveView(ViewMessages)
		_, ok = handler.Resolve("r", sess)
		assert.False(t, ok, "review-cmd should not work in messages view")
	})

	t.Run("multi-scope command works in multiple views", func(t *testing.T) {
		// Should work in review view
		handler.SetActiveView(ViewReview)
		action, ok := handler.Resolve("m", sess)
		assert.True(t, ok, "multi-cmd should work in review view")
		assert.Equal(t, ActionTypeShell, action.Type)

		// Should work in messages view
		handler.SetActiveView(ViewMessages)
		action, ok = handler.Resolve("m", sess)
		assert.True(t, ok, "multi-cmd should work in messages view")
		assert.Equal(t, ActionTypeShell, action.Type)

		// Should not work in sessions view
		handler.SetActiveView(ViewSessions)
		_, ok = handler.Resolve("m", sess)
		assert.False(t, ok, "multi-cmd should not work in sessions view")
	})

	t.Run("help entries filtered by scope", func(t *testing.T) {
		// In sessions view, should only see global and sessions commands
		handler.SetActiveView(ViewSessions)
		entries := handler.HelpEntries()
		assert.Len(t, entries, 2, "expected 2 entries in sessions view, got %d", len(entries))

		// In review view, should see global, review, and multi commands
		handler.SetActiveView(ViewReview)
		entries = handler.HelpEntries()
		assert.Len(t, entries, 3, "expected 3 entries in review view, got %d", len(entries))

		// In messages view, should see global and multi commands
		handler.SetActiveView(ViewMessages)
		entries = handler.HelpEntries()
		assert.Len(t, entries, 2, "expected 2 entries in messages view, got %d", len(entries))
	})
}
