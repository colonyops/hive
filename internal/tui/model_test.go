package tui

import (
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
	"unsafe"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	act "github.com/colonyops/hive/internal/core/action"
	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/internal/core/eventbus/testbus"
	"github.com/colonyops/hive/internal/core/hc"
	"github.com/colonyops/hive/internal/core/session"
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

func TestOpenNewSessionFormReadsEnvironmentDefaultAgentAtOpen(t *testing.T) {
	m := newKeybindingPrecedenceModel(t, func(cfg *config.Config) {
		cfg.Agents.AgentSelector = true
		cfg.Agents.Default = "claude"
		cfg.Agents.Profiles = map[string]config.AgentProfile{
			"claude": {},
			"pi":     {},
		}
	})
	t.Setenv(config.EnvDefaultAgent, "pi")

	model, _ := m.openNewSessionForm()
	opened := model.(Model)
	require.NotNil(t, opened.modals.NewSession)
	require.True(t, opened.modals.NewSession.hasAgentSelector)
	require.NotEmpty(t, opened.modals.NewSession.agent.keys)
	assert.Equal(t, "pi", opened.modals.NewSession.agent.keys[opened.modals.NewSession.agent.selected])
}

func TestOpenNewSessionFormUsesEnvironmentDefaultAgent(t *testing.T) {
	dataDir := t.TempDir()
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`agents:
  agent_selector: true
  default: claude
  claude: {}
  pi: {}
`), 0o600))
	t.Setenv(config.EnvDefaultAgent, "pi")

	cfg, err := config.Load(configPath, dataDir)
	require.NoError(t, err)
	require.Equal(t, "pi", cfg.Agents.Default)

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

	model, _ := m.openNewSessionForm()
	opened := model.(Model)
	require.NotNil(t, opened.modals.NewSession)
	require.True(t, opened.modals.NewSession.hasAgentSelector)
	require.NotEmpty(t, opened.modals.NewSession.agent.keys)
	assert.Equal(t, "pi", opened.modals.NewSession.agent.keys[opened.modals.NewSession.agent.selected])
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

func setUnexportedField[T any](t *testing.T, target any, field string, value T) {
	t.Helper()

	v := reflect.ValueOf(target)
	require.Equal(t, reflect.Pointer, v.Kind())

	f := v.Elem().FieldByName(field)
	require.True(t, f.IsValid(), "field %s not found", field)

	reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Elem().Set(reflect.ValueOf(value))
}

func getUnexportedField[T any](t *testing.T, target any, field string) T {
	t.Helper()

	v := reflect.ValueOf(target)
	require.Equal(t, reflect.Pointer, v.Kind())

	f := v.Elem().FieldByName(field)
	require.True(t, f.IsValid(), "field %s not found", field)

	return reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Elem().Interface().(T)
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

func TestPromotedKeysDefaultBehavior(t *testing.T) {
	t.Run("sessions g refreshes git statuses", func(t *testing.T) {
		m := newKeybindingPrecedenceModel(t, nil)
		m = switchTestView(t, m, ViewSessions)

		sessionItem := sessions.TreeItem{
			Session: session.Session{
				ID:    "s1",
				Name:  "alpha",
				Path:  "/tmp/alpha",
				State: session.StateActive,
			},
		}
		listModel := getUnexportedField[list.Model](t, m.sessionsView, "list")
		listModel.SetItems([]list.Item{sessionItem})
		setUnexportedField(t, m.sessionsView, "list", listModel)

		model, cmd := m.Update(keyPressMsg("g"))
		m = model.(Model)

		status, ok := m.sessionsView.GitStatuses().Get("/tmp/alpha")
		require.True(t, ok)
		assert.True(t, status.IsLoading)
		assert.NotNil(t, cmd)
	})

	t.Run("sessions slash enters focus mode", func(t *testing.T) {
		m := newKeybindingPrecedenceModel(t, nil)
		m = switchTestView(t, m, ViewSessions)

		model, _ := m.Update(keyPressMsg("/"))
		m = model.(Model)

		assert.True(t, m.sessionsView.FocusMode())
	})

	t.Run("tasks g goes to top", func(t *testing.T) {
		m := newKeybindingPrecedenceModel(t, nil)
		m = switchTestView(t, m, ViewTasks)

		nodes := []tasks.FlatNode{
			{Node: &tasks.TreeNode{Item: hc.Item{ID: "task-1", Type: hc.ItemTypeTask, CreatedAt: time.Now()}}},
			{Node: &tasks.TreeNode{Item: hc.Item{ID: "task-2", Type: hc.ItemTypeTask, CreatedAt: time.Now()}}},
		}
		setUnexportedField(t, m.tasksView, "flatNodes", nodes)
		setUnexportedField(t, m.tasksView, "cursor", 1)

		model, _ := m.Update(keyPressMsg("g"))
		m = model.(Model)

		assert.Equal(t, 0, getUnexportedField[int](t, m.tasksView, "cursor"))
	})

	t.Run("review G goes to bottom", func(t *testing.T) {
		m := newKeybindingPrecedenceModel(t, nil)
		m = switchTestView(t, m, ViewReview)

		doc := &review.Document{
			Path:          "/tmp/doc.md",
			RelPath:       "plans/doc.md",
			Content:       "line 1\nline 2\nline 3",
			RenderedLines: []string{"line 1", "line 2", "line 3"},
		}
		m.reviewView.SetSize(80, 24)
		setUnexportedField(t, m.reviewView, "fullScreen", true)
		setUnexportedField(t, m.reviewView, "selectedDoc", doc)
		setUnexportedField(t, m.reviewView, "cursorLine", 1)

		model, _ := m.Update(keyPressMsg("G"))
		m = model.(Model)

		assert.Equal(t, 3, getUnexportedField[int](t, m.reviewView, "cursorLine"))
	})
}

func TestReviewQuestionMarkDoesNotOpenGlobalHelpDialog(t *testing.T) {
	m := newKeybindingPrecedenceModel(t, nil)
	m = switchTestView(t, m, ViewReview)

	model, cmd := m.Update(keyPressMsg("?"))
	m = model.(Model)

	assert.Equal(t, stateNormal, m.state)
	assert.Nil(t, cmd)
	assert.False(t, m.quitting)
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

func TestKVFilterDoesNotLeakAcrossViews(t *testing.T) {
	newModel := func(t *testing.T) Model {
		return newKeybindingPrecedenceModel(t, func(cfg *config.Config) {
			cfg.UserCommands["SentinelTodoPanel"] = config.UserCommand{
				Action: act.TypeTodoPanel,
				Help:   "sentinel todo panel",
				Scope:  []string{"sessions"},
			}
			cfg.Views.Sessions.Keybindings["g"] = config.Keybinding{Cmd: "SentinelTodoPanel"}
		})
	}

	t.Run("switching away from store cancels the filter", func(t *testing.T) {
		m := newModel(t)
		m = switchTestView(t, m, ViewStore)
		m.kvView.StartFilter()
		require.True(t, m.kvView.IsFiltering())

		m = switchTestView(t, m, ViewSessions)
		assert.False(t, m.kvView.IsFiltering())

		// Keybindings resolve normally on the sessions view after switching.
		m = driveKeyUpdate(t, m, "g")
		assert.Equal(t, stateShowingTodos, m.state)
	})

	t.Run("active kv filter does not capture keys on other views", func(t *testing.T) {
		m := newModel(t)
		m = switchTestView(t, m, ViewSessions)

		// Simulate stale KV filter state while another view is active.
		m.kvView.StartFilter()
		require.True(t, m.kvView.IsFiltering())

		m = driveKeyUpdate(t, m, "g")
		assert.Equal(t, stateShowingTodos, m.state, "key must resolve via keybindings, not the KV filter")
	})
}

func TestUnknownCommandDoesNotFallBack(t *testing.T) {
	tests := []struct {
		name string
		bind func(*config.Config)
	}{
		{
			name: "missing command lookup",
			bind: func(cfg *config.Config) {
				cfg.Views.Sessions.Keybindings["q"] = config.Keybinding{Cmd: "DoesNotExist"}
			},
		},
		{
			name: "existing command out of scope",
			bind: func(cfg *config.Config) {
				cfg.UserCommands["TasksOnlyQuit"] = config.UserCommand{
					Action: act.TypeQuit,
					Help:   "tasks only quit",
					Scope:  []string{"tasks"},
				}
				cfg.Views.Global.Keybindings["q"] = config.Keybinding{Cmd: "TasksOnlyQuit"}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newKeybindingPrecedenceModel(t, tt.bind)

			m = driveKeyUpdate(t, m, "q")

			assert.False(t, m.quitting)
			assert.Equal(t, stateNormal, m.state)
		})
	}
}

func TestModalDismissalQIgnoresNormalModeOverride(t *testing.T) {
	tests := []struct {
		name       string
		open       func(t *testing.T, m Model) Model
		wantBefore UIState
	}{
		{
			name: "help dialog",
			open: func(t *testing.T, m Model) Model {
				t.Helper()
				model, _ := m.showHelpDialog()
				return model.(Model)
			},
			wantBefore: stateShowingHelp,
		},
		{
			name: "notifications modal",
			open: func(t *testing.T, m Model) Model {
				t.Helper()
				model, _ := m.handleGlobalAction(Action{Type: act.TypeNotifications})
				return model.(Model)
			},
			wantBefore: stateShowingNotifications,
		},
		{
			name: "info dialog",
			open: func(t *testing.T, m Model) Model {
				t.Helper()
				model, _ := m.showHiveInfo()
				return model.(Model)
			},
			wantBefore: stateShowingInfo,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newKeybindingPrecedenceModel(t, func(cfg *config.Config) {
				cfg.UserCommands["SentinelNotifications"] = config.UserCommand{
					Action: act.TypeNotifications,
					Help:   "sentinel notifications",
				}
				cfg.Views.Global.Keybindings["q"] = config.Keybinding{Cmd: "SentinelNotifications"}
			})

			m = tt.open(t, m)
			assert.Equal(t, tt.wantBefore, m.state)

			m = driveKeyUpdate(t, m, "q")

			assert.Equal(t, stateNormal, m.state)
			assert.NotEqual(t, stateShowingNotifications, m.state)
		})
	}
}
