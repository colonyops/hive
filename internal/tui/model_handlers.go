package tui

import (
	"fmt"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/rs/zerolog/log"

	"github.com/colonyops/hive/internal/core/eventbus"
	"github.com/colonyops/hive/internal/core/notify"
	"github.com/colonyops/hive/internal/core/session"
	"github.com/colonyops/hive/internal/core/terminal"
	"github.com/colonyops/hive/internal/tui/views/review"
)

// --- Window ---

func (m Model) handleWindowSize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.width = msg.Width
	m.height = msg.Height

	// Account for: top divider (1) + header (1) + bottom divider (1) = 3 lines
	contentHeight := msg.Height - 3
	if contentHeight < 1 {
		contentHeight = 1
	}

	// Set list size based on preview mode
	if m.previewEnabled && m.width >= 80 && m.activeView == ViewSessions {
		// In dual-column mode, list takes 25% of width
		listWidth := int(float64(m.width) * 0.25)
		m.list.SetSize(listWidth, contentHeight)
	} else {
		// In single-column mode, list takes full width
		m.list.SetSize(msg.Width, contentHeight)
	}

	// msgView gets -1 because we prepend a blank line for consistent spacing
	if m.msgView != nil {
		m.msgView.SetSize(msg.Width, contentHeight-1)
	}

	// Set review view size
	if m.reviewView != nil {
		m.reviewView.SetSize(msg.Width, contentHeight)
	}

	// Set KV view size
	m.kvView.SetSize(msg.Width, contentHeight)

	// Publish startup warnings on the first WindowSizeMsg so they flow
	// through the Update loop with the render cycle already running.
	if len(m.startupWarnings) > 0 {
		for _, w := range m.startupWarnings {
			m.notifyBus.Warnf("%s", w)
		}
		m.startupWarnings = nil
		return m, m.ensureToastTick()
	}
	return m, nil
}

// --- Data loaded ---

func (m Model) handleKVKeysLoaded(msg kvKeysLoadedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		log.Debug().Err(msg.err).Msg("failed to load kv keys")
		return m, nil
	}
	m.kvView.SetKeys(msg.keys)
	if key := m.kvView.SelectedKey(); key != "" {
		return m, m.loadKVEntry(key)
	}
	m.kvView.SetPreview(nil)
	return m, nil
}

func (m Model) handleKVEntryLoaded(msg kvEntryLoadedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		log.Debug().Err(msg.err).Msg("failed to load kv entry")
		m.kvView.SetPreview(nil)
		return m, nil
	}
	m.kvView.SetPreview(&msg.entry)
	return m, nil
}

func (m Model) handleSessionsLoaded(msg sessionsLoadedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		log.Error().Err(msg.err).Msg("failed to load sessions")
		m.state = stateNormal
		return m, m.notifyError("failed to load sessions: %v", msg.err)
	}
	m.allSessions = msg.sessions
	filteredModel, cmd := m.applyFilter()
	if m.pluginManager != nil && len(m.pluginStatuses) > 0 {
		sessions := make([]*session.Session, len(m.allSessions))
		for i := range m.allSessions {
			sessions[i] = &m.allSessions[i]
		}
		m.pluginManager.UpdateSessions(sessions)
		log.Debug().Int("sessionCount", len(sessions)).Msg("updated plugin manager sessions")
	}
	return filteredModel, cmd
}

func (m Model) handleGitStatusComplete(msg gitStatusBatchCompleteMsg) (tea.Model, tea.Cmd) {
	m.gitStatuses.SetBatch(msg.Results)
	m.refreshing = false
	return m, nil
}

func (m Model) handleTerminalStatusComplete(msg terminalStatusBatchCompleteMsg) (tea.Model, tea.Cmd) {
	if m.terminalStatuses != nil {
		if m.bus != nil {
			for sessionID, newStatus := range msg.Results {
				oldStatus, exists := m.terminalStatuses.Get(sessionID)
				prevStatus := oldStatus.Status
				if !exists {
					prevStatus = terminal.StatusMissing
				}
				if prevStatus != newStatus.Status {
					sess := m.findSessionByID(sessionID)
					if sess == nil {
						continue
					}
					m.bus.PublishAgentStatusChanged(eventbus.AgentStatusChangedPayload{
						Session:   sess,
						OldStatus: prevStatus,
						NewStatus: newStatus.Status,
					})
				}
			}
		}

		m.terminalStatuses.SetBatch(msg.Results)
		m.rebuildWindowItems()
	}
	return m, nil
}

func (m Model) handlePluginWorkerStarted(msg pluginWorkerStartedMsg) (tea.Model, tea.Cmd) {
	m.pluginResultsChan = msg.resultsChan
	log.Debug().Msg("plugin background worker started")
	return m, listenForPluginResult(m.pluginResultsChan)
}

func (m Model) handlePluginStatusUpdate(msg pluginStatusUpdateMsg) (tea.Model, tea.Cmd) {
	if msg.Err == nil {
		if store, ok := m.pluginStatuses[msg.PluginName]; ok {
			store.Set(msg.SessionID, msg.Status)
			log.Debug().
				Str("plugin", msg.PluginName).
				Str("session", msg.SessionID).
				Str("label", msg.Status.Label).
				Msg("plugin status updated")
		}
	}
	m.treeDelegate.PluginStatuses = m.pluginStatuses
	m.list.SetDelegate(m.treeDelegate)
	return m, listenForPluginResult(m.pluginResultsChan)
}

func (m Model) handleReposDiscovered(msg reposDiscoveredMsg) (tea.Model, tea.Cmd) {
	m.discoveredRepos = msg.repos
	if len(m.discoveredRepos) == 0 {
		m.toastController.Push(notify.Notification{
			Level:   notify.LevelError,
			Message: fmt.Sprintf("No repositories found in directories: %v", m.repoDirs),
		})
	}
	return m, nil
}

// --- Polling ticks ---

func (m Model) handleSessionRefreshTick(_ sessionRefreshTickMsg) (tea.Model, tea.Cmd) {
	if m.activeView == ViewSessions && !m.isModalActive() {
		m.refreshing = true
		return m, tea.Batch(
			m.loadSessions(),
			m.scheduleSessionRefresh(),
		)
	}
	return m, m.scheduleSessionRefresh()
}

func (m Model) handleKVPollTick(_ kvPollTickMsg) (tea.Model, tea.Cmd) {
	if m.isStoreFocused() && !m.isModalActive() {
		return m, tea.Batch(
			m.loadKVKeys(),
			scheduleKVPollTick(),
		)
	}
	return m, scheduleKVPollTick()
}

func (m Model) handleTerminalPollTick(_ terminalPollTickMsg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	sessions := make([]*session.Session, len(m.allSessions))
	for i := range m.allSessions {
		sessions[i] = &m.allSessions[i]
	}
	cmds = append(cmds, fetchTerminalStatusBatch(m.terminalManager, sessions, m.gitWorkers))
	if m.terminalManager != nil && m.terminalManager.HasEnabledIntegrations() {
		cmds = append(cmds, startTerminalPollTicker(m.cfg.Tmux.PollInterval))
	}
	return m, tea.Batch(cmds...)
}

func (m Model) handleAnimationTick(_ animationTickMsg) (tea.Model, tea.Cmd) {
	m.animationFrame = (m.animationFrame + 1) % AnimationFrameCount
	m.treeDelegate.AnimationFrame = m.animationFrame
	m.list.SetDelegate(m.treeDelegate)
	return m, scheduleAnimationTick()
}

func (m Model) handleToastTick(_ toastTickMsg) (tea.Model, tea.Cmd) {
	m.toastController.Tick()
	if m.toastController.HasToasts() {
		return m, scheduleToastTick()
	}
	return m, nil
}

// --- Action results ---

func (m Model) handleRenameComplete(msg renameCompleteMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		log.Error().Err(msg.err).Msg("rename failed")
		m.state = stateNormal
		return m, m.notifyError("rename failed: %v", msg.err)
	}
	return m, m.loadSessions()
}

func (m Model) handleActionComplete(msg actionCompleteMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		log.Error().Err(msg.err).Msg("action failed")
		m.state = stateNormal
		m.pending = Action{}
		return m, m.notifyError("action failed: %v", msg.err)
	}
	m.state = stateNormal
	m.pending = Action{}
	return m, m.loadSessions()
}

func (m Model) handleRecycleStarted(msg recycleStartedMsg) (tea.Model, tea.Cmd) {
	m.state = stateRunningRecycle
	m.outputModal = NewOutputModal("Recycling session...")
	m.recycleOutput = msg.output
	m.recycleDone = msg.done
	m.recycleCancel = msg.cancel
	return m, tea.Batch(
		listenForRecycleOutput(msg.output, msg.done),
		m.outputModal.Spinner().Tick,
	)
}

func (m Model) handleRecycleOutput(msg recycleOutputMsg) (tea.Model, tea.Cmd) {
	m.outputModal.AddLine(msg.line)
	return m, listenForRecycleOutput(m.recycleOutput, m.recycleDone)
}

func (m Model) handleRecycleComplete(msg recycleCompleteMsg) (tea.Model, tea.Cmd) {
	m.outputModal.SetComplete(msg.err)
	m.recycleOutput = nil
	m.recycleDone = nil
	m.recycleCancel = nil
	return m, nil
}

// --- Review delegation ---

func (m Model) handleReviewDocChange(msg review.DocumentChangeMsg) (tea.Model, tea.Cmd) {
	if m.reviewView != nil {
		*m.reviewView, _ = m.reviewView.Update(msg)
	}
	return m, nil
}

func (m Model) handleReviewFinalized(msg review.ReviewFinalizedMsg) (tea.Model, tea.Cmd) {
	if err := m.copyToClipboard(msg.Feedback); err != nil {
		return m, m.notifyError("failed to copy feedback: %v", err)
	}
	return m, nil
}

func (m Model) handleReviewOpenDoc(msg review.OpenDocumentMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		return m, m.notifyError("open document: %v", msg.Err)
	}
	if m.reviewView != nil {
		for _, item := range m.reviewView.List().Items() {
			if treeItem, ok := item.(review.TreeItem); ok && !treeItem.IsHeader {
				if treeItem.Document.Path == msg.Path {
					m.reviewView.LoadDocument(&treeItem.Document)
					break
				}
			}
		}
	}
	return m, nil
}

// --- Notifications ---

func (m Model) handleNotification(msg notificationMsg) (tea.Model, tea.Cmd) {
	m.notifyBus.Publish(msg.notification)
	return m, m.ensureToastTick()
}

// --- Input ---

func (m Model) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Handle document picker modal if active (on Sessions view)
	if m.docPickerModal != nil {
		modal, cmd := m.docPickerModal.Update(msg)
		m.docPickerModal = modal

		if m.docPickerModal.SelectedDocument() != nil {
			doc := m.docPickerModal.SelectedDocument()
			m.docPickerModal = nil
			m.activeView = ViewReview
			m.handler.SetActiveView(ViewReview)
			if m.reviewView != nil {
				m.reviewView.LoadDocument(doc)
			}
			return m, cmd
		}

		if m.docPickerModal.Cancelled() {
			m.docPickerModal = nil
			return m, cmd
		}

		return m, cmd
	}

	return m.handleKey(msg)
}

func (m Model) handleSpinnerTick(msg spinner.TickMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.spinner, cmd = m.spinner.Update(msg)
	return m, cmd
}

// handleFallthrough routes messages that don't match any typed case.
func (m Model) handleFallthrough(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Route all other messages to the form when creating session
	if m.state == stateCreatingSession && m.newSessionForm != nil {
		return m.updateNewSessionForm(msg)
	}

	// Always forward to messages view for its internal polling/loaded messages
	var cmds []tea.Cmd
	if m.msgView != nil {
		var cmd tea.Cmd
		*m.msgView, cmd = m.msgView.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	// Update the appropriate view based on active view
	switch m.activeView {
	case ViewSessions:
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	case ViewMessages:
		// Handled above — messages view always receives fallthrough messages
	case ViewStore:
		// KV view handles its own updates via explicit method calls
	case ViewReview:
		if m.reviewView != nil {
			var cmd tea.Cmd
			*m.reviewView, cmd = m.reviewView.Update(msg)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
	}
	return m, tea.Batch(cmds...)
}
