package tui

import (
	"io"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	act "github.com/colonyops/hive/internal/core/action"
	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/internal/core/eventbus/testbus"
	"github.com/colonyops/hive/internal/core/terminal"
	"github.com/colonyops/hive/internal/data/db"
	"github.com/colonyops/hive/internal/data/stores"
	"github.com/colonyops/hive/internal/hive"
	"github.com/colonyops/hive/internal/hive/plugins"
	"github.com/colonyops/hive/internal/tui/views/review"
	"github.com/colonyops/hive/internal/tui/views/sessions"
	"github.com/colonyops/hive/internal/tui/views/tasks"
)

func newKeybindingPrecedenceModel(t *testing.T, mutate func(*config.Config)) Model {
	t.Helper()

	dataDir := t.TempDir()
	cfg, err := config.Load("", dataDir)
	require.NoError(t, err)

	if cfg.UserCommands == nil {
		cfg.UserCommands = map[string]config.UserCommand{}
	}

	if mutate != nil {
		mutate(cfg)
	}

	database, err := db.Open(dataDir, db.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() {
		assert.NoError(t, database.Close())
	})

	tb := testbus.New(t)
	commandSet := plugins.NewCommandSet(config.DefaultUserCommands(), cfg.UserCommands)
	pluginManager := plugins.NewManager(plugins.NewWorkerPool(0), commandSet)
	todoService := hive.NewTodoService(
		stores.NewTodoStore(database),
		tb.EventBus,
		cfg,
		zerolog.New(io.Discard),
	)

	m := New(Deps{
		Config:          cfg,
		Service:         newMouseTestSessionService(t),
		Renderer:        testRenderer,
		TerminalManager: terminal.NewManager(nil),
		PluginManager:   pluginManager,
		CommandSet:      commandSet,
		TodoService:     todoService,
		DB:              database,
		Bus:             tb.EventBus,
	}, Opts{})

	return m
}

func keyPressMsg(key string) tea.KeyPressMsg {
	switch key {
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "ctrl+c":
		return tea.KeyPressMsg{Text: "ctrl+c", Code: 3}
	default:
		r := []rune(key)
		return tea.KeyPressMsg{Text: key, Code: r[0]}
	}
}

func driveKeyUpdate(t *testing.T, m Model, key string) Model {
	t.Helper()

	model, cmd := m.Update(keyPressMsg(key))
	next := model.(Model)
	return drainImmediateMsgs(t, next, cmd)
}

func drainImmediateMsgs(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()

	if cmd == nil {
		return m
	}

	msg := cmd()
	switch typed := msg.(type) {
	case nil:
		return m
	case tea.BatchMsg:
		for _, sub := range typed {
			m = drainImmediateMsgs(t, m, sub)
		}
		return m
	case sessions.ActionRequestMsg, sessions.CommandPaletteRequestMsg, sessions.NewSessionRequestMsg,
		sessions.FormCommandRequestMsg, sessions.RenameRequestMsg, sessions.DocReviewRequestMsg,
		sessions.RecycledDeleteRequestMsg, sessions.OpenRepoRequestMsg, tasks.ActionRequestMsg,
		tasks.CommandPaletteRequestMsg, tasks.RefreshTasksMsg, tasks.TaskActionCompleteMsg,
		review.ActionRequestMsg, review.CommandPaletteRequestMsg:
		model, nextCmd := m.Update(msg)
		return drainImmediateMsgs(t, model.(Model), nextCmd)
	default:
		return m
	}
}

func switchTestView(t *testing.T, m Model, view ViewType) Model {
	t.Helper()

	model, _ := m.switchToView(view)
	return model.(Model)
}

func TestPromotedKeysAreOverridable(t *testing.T) {
	tests := []struct {
		name      string
		view      ViewType
		key       string
		wantState UIState
		bind      func(*config.Config)
	}{
		{
			name:      "sessions g",
			view:      ViewSessions,
			key:       "g",
			wantState: stateShowingTodos,
			bind: func(cfg *config.Config) {
				cfg.Views.Sessions.Keybindings["g"] = config.Keybinding{Cmd: "SentinelTodoPanel"}
			},
		},
		{
			name:      "sessions v",
			view:      ViewSessions,
			key:       "v",
			wantState: stateShowingTodos,
			bind: func(cfg *config.Config) {
				cfg.Views.Sessions.Keybindings["v"] = config.Keybinding{Cmd: "SentinelTodoPanel"}
			},
		},
		{
			name:      "sessions up",
			view:      ViewSessions,
			key:       "up",
			wantState: stateShowingTodos,
			bind: func(cfg *config.Config) {
				cfg.Views.Sessions.Keybindings["up"] = config.Keybinding{Cmd: "SentinelTodoPanel"}
			},
		},
		{
			name:      "sessions k",
			view:      ViewSessions,
			key:       "k",
			wantState: stateShowingTodos,
			bind: func(cfg *config.Config) {
				cfg.Views.Sessions.Keybindings["k"] = config.Keybinding{Cmd: "SentinelTodoPanel"}
			},
		},
		{
			name:      "sessions down",
			view:      ViewSessions,
			key:       "down",
			wantState: stateShowingTodos,
			bind: func(cfg *config.Config) {
				cfg.Views.Sessions.Keybindings["down"] = config.Keybinding{Cmd: "SentinelTodoPanel"}
			},
		},
		{
			name:      "sessions j",
			view:      ViewSessions,
			key:       "j",
			wantState: stateShowingTodos,
			bind: func(cfg *config.Config) {
				cfg.Views.Sessions.Keybindings["j"] = config.Keybinding{Cmd: "SentinelTodoPanel"}
			},
		},
		{
			name:      "sessions slash",
			view:      ViewSessions,
			key:       "/",
			wantState: stateShowingTodos,
			bind: func(cfg *config.Config) {
				cfg.Views.Sessions.Keybindings["/"] = config.Keybinding{Cmd: "SentinelTodoPanel"}
			},
		},
		{
			name:      "sessions colon",
			view:      ViewSessions,
			key:       ":",
			wantState: stateShowingTodos,
			bind: func(cfg *config.Config) {
				cfg.Views.Sessions.Keybindings[":"] = config.Keybinding{Cmd: "SentinelTodoPanel"}
			},
		},
		{
			name:      "global q",
			view:      ViewSessions,
			key:       "q",
			wantState: stateShowingHelp,
			bind: func(cfg *config.Config) {
				cfg.Views.Global.Keybindings["q"] = config.Keybinding{Cmd: "SentinelShowHelp"}
			},
		},
		{
			name:      "global question mark",
			view:      ViewSessions,
			key:       "?",
			wantState: stateNormal,
			bind: func(cfg *config.Config) {
				cfg.Views.Global.Keybindings["?"] = config.Keybinding{Cmd: "SentinelQuit"}
			},
		},
		{
			name:      "tasks g",
			view:      ViewTasks,
			key:       "g",
			wantState: stateShowingNotifications,
			bind: func(cfg *config.Config) {
				cfg.Views.Tasks.Keybindings["g"] = config.Keybinding{Cmd: "SentinelNotifications"}
			},
		},
		{
			name:      "tasks G",
			view:      ViewTasks,
			key:       "G",
			wantState: stateShowingNotifications,
			bind: func(cfg *config.Config) {
				cfg.Views.Tasks.Keybindings["G"] = config.Keybinding{Cmd: "SentinelNotifications"}
			},
		},
		{
			name:      "review g",
			view:      ViewReview,
			key:       "g",
			wantState: stateShowingNotifications,
			bind: func(cfg *config.Config) {
				cfg.Views.Review.Keybindings["g"] = config.Keybinding{Cmd: "SentinelNotifications"}
			},
		},
		{
			name:      "review G",
			view:      ViewReview,
			key:       "G",
			wantState: stateShowingNotifications,
			bind: func(cfg *config.Config) {
				cfg.Views.Review.Keybindings["G"] = config.Keybinding{Cmd: "SentinelNotifications"}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newKeybindingPrecedenceModel(t, func(cfg *config.Config) {
				cfg.UserCommands["SentinelNotifications"] = config.UserCommand{
					Action: act.TypeNotifications,
					Help:   "sentinel notifications",
				}
				cfg.UserCommands["SentinelQuit"] = config.UserCommand{
					Action: act.TypeQuit,
					Help:   "sentinel quit",
				}
				cfg.UserCommands["SentinelShowHelp"] = config.UserCommand{
					Action: act.TypeShowHelp,
					Help:   "sentinel show help",
				}
				cfg.UserCommands["SentinelTodoPanel"] = config.UserCommand{
					Action: act.TypeTodoPanel,
					Help:   "sentinel todo panel",
					Scope:  []string{"sessions"},
				}
				tt.bind(cfg)
			})

			m = switchTestView(t, m, tt.view)
			m = driveKeyUpdate(t, m, tt.key)

			assert.Equal(t, tt.wantState, m.state)
			if tt.key == "?" {
				assert.True(t, m.quitting)
			} else {
				assert.False(t, m.quitting)
			}
		})
	}
}

func TestCtrlCAlwaysQuits(t *testing.T) {
	m := newKeybindingPrecedenceModel(t, func(cfg *config.Config) {
		cfg.UserCommands["SentinelNotifications"] = config.UserCommand{
			Action: act.TypeNotifications,
			Help:   "sentinel notifications",
		}
		cfg.Views.Global.Keybindings["ctrl+c"] = config.Keybinding{Cmd: "SentinelNotifications"}
	})

	model, cmd := m.Update(keyPressMsg("ctrl+c"))
	updated := model.(Model)

	assert.True(t, updated.quitting)
	assert.NotNil(t, cmd)
	assert.NotEqual(t, stateShowingNotifications, updated.state)
}

func TestFilterModeSwallowsKeys(t *testing.T) {
	m := newKeybindingPrecedenceModel(t, func(cfg *config.Config) {
		cfg.UserCommands["SentinelTodoPanel"] = config.UserCommand{
			Action: act.TypeTodoPanel,
			Help:   "sentinel todo panel",
			Scope:  []string{"sessions"},
		}
		cfg.Views.Sessions.Keybindings["g"] = config.Keybinding{Cmd: "SentinelTodoPanel"}
		cfg.Views.Sessions.Keybindings["v"] = config.Keybinding{Cmd: "SentinelTodoPanel"}
	})

	model, _ := m.Update(keyPressMsg("/"))
	m = model.(Model)
	require.True(t, m.sessionsView.FocusMode())

	m = driveKeyUpdate(t, m, "g")
	assert.True(t, m.sessionsView.FocusMode())
	assert.Equal(t, stateNormal, m.state)

	m = driveKeyUpdate(t, m, "v")
	assert.True(t, m.sessionsView.FocusMode())
	assert.Equal(t, stateNormal, m.state)
}

func TestUnknownCommandDoesNotFallBack(t *testing.T) {
	m := newKeybindingPrecedenceModel(t, func(cfg *config.Config) {
		cfg.Views.Sessions.Keybindings["q"] = config.Keybinding{Cmd: "DoesNotExist"}
	})

	m = driveKeyUpdate(t, m, "q")

	assert.False(t, m.quitting)
	assert.Equal(t, stateNormal, m.state)
}
