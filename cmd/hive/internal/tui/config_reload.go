package tui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/colonyops/hive/cmd/hive/internal/config"
	"github.com/colonyops/hive/cmd/hive/internal/tui/command"
	"github.com/colonyops/hive/cmd/hive/internal/tui/views/sessions"
	"github.com/colonyops/hive/internal/domain/notify"
)

type configReloadedMsg struct {
	deps Deps
	err  error
}

func (m Model) startConfigReload() (tea.Model, tea.Cmd) {
	if m.reloading {
		return m, nil
	}
	if m.reloadConfig == nil {
		m.publishNotificationf(notify.LevelError, "Configuration reload is unavailable")
		return m, nil
	}
	m.reloading = true
	reload := m.reloadConfig
	return m, func() tea.Msg {
		deps, err := reload()
		return configReloadedMsg{deps: deps, err: err}
	}
}

func (m Model) handleConfigReloaded(msg configReloadedMsg) (tea.Model, tea.Cmd) {
	m.reloading = false
	if msg.err != nil {
		m.logger.Warn().Err(msg.err).Msg("configuration reload failed")
		m.publishNotificationf(notify.LevelError, "Configuration not reloaded: %s", msg.err)
		return m, nil
	}
	deps := msg.deps
	cfg := deps.Config
	old := m.cfg
	m.cfg = cfg
	m.service = deps.Service
	m.renderer = deps.Renderer
	m.todoService = deps.TodoService
	m.doctorService = deps.DoctorService
	m.sourceRegistry = deps.Sources
	m.copyCommand = cfg.CopyCommand
	m.cmdService = command.NewService(deps.Service, deps.Service, deps.Service, deps.Service, deps.Service, deps.Service)
	m.handler.viewKeybindings = map[string]map[string]config.Keybinding{
		"global":   cfg.Views.Global.Keybindings,
		"sessions": cfg.Views.Sessions.Keybindings,
		"tasks":    cfg.Views.Tasks.Keybindings,
		"review":   cfg.Views.Review.Keybindings,
	}
	m.handler.renderer = deps.Renderer
	m.handler.rebuildEffective()
	cmd := m.sessionsView.Reconfigure(sessions.ViewOpts{
		Cfg: cfg, Service: deps.Service, Status: deps.Status,
		GitStatus: deps.GitStatus, Renderer: deps.Renderer,
	})
	m.msgView.Reconfigure(deps.MsgStore, cfg.CopyCommand, cfg.Views.Messages.SplitRatio)
	m.tasksView.SetSplitRatio(cfg.Views.Tasks.SplitRatio)
	m.reviewView.SetSplitRatio(cfg.Views.Review.SplitRatio)
	if old.TUI.Theme != cfg.TUI.Theme {
		m.applyTheme(cfg.TUI.Theme)
	}
	if !m.kvPollStarted && cfg.TUI.Store {
		m.kvPollStarted = true
		cmd = tea.Batch(cmd, scheduleKVPollTick())
	}
	if !cfg.TUI.Store && m.activeView == ViewStore {
		switched, switchCmd := m.switchToView(ViewSessions)
		m = switched.(Model)
		cmd = tea.Batch(cmd, switchCmd)
	}
	m.publishNotificationf(notify.LevelInfo, "Configuration reloaded")
	return m, cmd
}
