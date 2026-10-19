package tui

import (
	"errors"
	"testing"
	"testing/synctest"

	tea "charm.land/bubbletea/v2"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	act "github.com/colonyops/hive/cmd/hive/internal/action"
	"github.com/colonyops/hive/cmd/hive/internal/config"
	"github.com/colonyops/hive/internal/domain/terminal"
	"github.com/colonyops/hive/internal/hive/gitstatus"
	statussvc "github.com/colonyops/hive/internal/hive/status"
)

func TestReloadCommandWorksWithoutASelectionAndRetainsConfigOnFailure(t *testing.T) {
	m := newKeybindingPrecedenceModel(t, nil)
	m.bus = nil
	original := m.cfg
	m.reloadConfig = func() (Deps, error) { return Deps{}, errors.New("invalid YAML") }
	m.modals.CommandPalette = NewCommandPalette(map[string]config.UserCommand{
		"ReloadConfig": config.DefaultUserCommands()["ReloadConfig"],
	}, nil, 80, 24, ViewSessions)
	m.state = stateCommandPalette
	updated, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	require.NotNil(t, cmd)
	loading := updated.(Model)
	assert.True(t, loading.reloading)
	_, duplicate := loading.startConfigReload()
	assert.Nil(t, duplicate)
	updated, _ = loading.Update(cmd())
	result := updated.(Model)
	assert.False(t, result.reloading)
	assert.Same(t, original, result.cfg)
	assert.Contains(t, notificationMessages(result)[0], "invalid YAML")
}

func TestReloadUpdatesBindingsAndServicesWithoutResettingViews(t *testing.T) {
	m := newKeybindingPrecedenceModel(t, nil)
	m.bus = nil
	m.sessionsView.ApplyStatusFilter(act.TypeFilterActive)
	cfg, err := config.Load("", m.cfg.DataDir)
	require.NoError(t, err)
	cfg.Views.Global.Keybindings["ctrl+r"] = config.Keybinding{Cmd: "ReloadConfig"}
	cfg.CopyCommand = "new-clipboard"
	service := newMouseTestSessionService(t)
	originalView := m.sessionsView
	originalHandler := m.handler
	updated, _ := m.Update(configReloadedMsg{deps: Deps{
		Config: cfg, Service: service, Renderer: testRenderer, TodoService: m.todoService,
		Status:    statussvc.NewService(zerolog.Nop(), terminal.NewManager(nil), 1),
		GitStatus: gitstatus.NewService(zerolog.Nop(), nil, 1),
	}})
	result := updated.(Model)
	assert.Same(t, originalView, result.sessionsView)
	assert.Same(t, originalHandler, result.handler)
	assert.Same(t, service, result.service)
	assert.Equal(t, terminal.StatusActive, result.sessionsView.StatusFilter())
	assert.Equal(t, "new-clipboard", result.copyCommand)
	assert.True(t, result.handler.IsAction("ctrl+r", act.TypeReloadConfig))
	assert.Contains(t, notificationMessages(result), "Configuration reloaded")
	result.reloadConfig = func() (Deps, error) { return Deps{}, errors.New("test") }
	updated, cmd := result.Update(tea.KeyPressMsg(tea.Key{Code: 'r', Mod: tea.ModCtrl}))
	require.NotNil(t, cmd)
	assert.True(t, updated.(Model).reloading)
}

func TestReloadStoreTransitionsKeepOnePollerAndLeaveDisabledTab(t *testing.T) {
	m := newKeybindingPrecedenceModel(t, nil)
	m.bus = nil
	for _, step := range []struct {
		enabled   bool
		wantTicks int
	}{
		{true, 1}, {true, 0}, {false, 0}, {true, 0},
	} {
		synctest.Test(t, func(t *testing.T) {
			cfg := *m.cfg
			cfg.TUI.Store = step.enabled
			if !step.enabled {
				m.activeView = ViewStore
			}
			updated, cmd := m.Update(configReloadedMsg{deps: Deps{
				Config: &cfg, Service: m.service, Renderer: m.renderer, TodoService: m.todoService,
				Status:    statussvc.NewService(zerolog.Nop(), terminal.NewManager(nil), 1),
				GitStatus: gitstatus.NewService(zerolog.Nop(), nil, 1),
			}})
			m = updated.(Model)
			if !step.enabled {
				assert.Equal(t, ViewSessions, m.activeView)
			}
			ticks := 0
			var run func(tea.Cmd)
			run = func(cmd tea.Cmd) {
				if cmd == nil {
					return
				}
				switch msg := cmd().(type) {
				case tea.BatchMsg:
					for _, child := range msg {
						run(child)
					}
				case kvPollTickMsg:
					ticks++
				}
			}
			run(cmd)
			assert.Equal(t, step.wantTicks, ticks)
		})
	}
}
