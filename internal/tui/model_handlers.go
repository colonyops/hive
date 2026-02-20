package tui

import (
	"context"
	"fmt"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/rs/zerolog/log"

	act "github.com/colonyops/hive/internal/core/action"
	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/internal/core/messaging"
	"github.com/colonyops/hive/internal/core/notify"
	"github.com/colonyops/hive/internal/core/session"
	"github.com/colonyops/hive/internal/core/todo"
	"github.com/colonyops/hive/internal/hive"
	"github.com/colonyops/hive/internal/tui/command"
	"github.com/colonyops/hive/internal/tui/views/review"
	"github.com/colonyops/hive/internal/tui/views/sessions"
)

// --- Window ---

func (m Model) handleWindowSize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.width = msg.Width
	m.height = msg.Height

	contentHeight := msg.Height - 3
	if contentHeight < 1 {
		contentHeight = 1
	}

	m.modals.SetSize(msg.Width, msg.Height)

	if m.sessionsView != nil {
		m.sessionsView.SetSize(msg.Width, msg.Height)
	}

	// msgView gets contentHeight-1 so lipgloss.Height(contentHeight) in model_render.go
	// adds one trailing blank line, matching the visual spacing of other views.
	if m.msgView != nil {
		m.msgView.SetSize(msg.Width, contentHeight-1)
	}

	if m.reviewView != nil {
		m.reviewView.SetSize(msg.Width, contentHeight)
	}

	m.kvView.SetSize(msg.Width, contentHeight)

	// Publish startup warnings on the first WindowSizeMsg
	if len(m.startupWarnings) > 0 {
		for _, w := range m.startupWarnings {
			m.notifyBus.Warnf("%s", w)
		}
		m.startupWarnings = nil
		return m, m.ensureToastTick()
	}
	return m, nil
}

// --- KV data loaded ---

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

// --- KV polling ---

func (m Model) handleKVPollTick(_ kvPollTickMsg) (tea.Model, tea.Cmd) {
	if m.isStoreFocused() && !m.isModalActive() {
		return m, tea.Batch(
			m.loadKVKeys(),
			scheduleKVPollTick(),
		)
	}
	return m, scheduleKVPollTick()
}

func (m Model) handleToastTick(_ toastTickMsg) (tea.Model, tea.Cmd) {
	m.toastController.Tick()
	if m.toastController.HasToasts() {
		return m, scheduleToastTick()
	}
	return m, nil
}

// --- Outbound messages from sessions view ---

func (m Model) handleSessionAction(msg sessions.ActionRequestMsg) (tea.Model, tea.Cmd) {
	action := msg.Action
	if action.Err != nil {
		return m, m.notifyError("keybinding error: %v", action.Err)
	}
	if action.Type == act.TypeDocReview {
		cmd := HiveDocReviewCmd{Arg: ""}
		return m, cmd.Execute(&m)
	}
	if action.Type == act.TypeRenameSession {
		sess := m.sessionsView.SelectedSession()
		if sess == nil {
			return m, nil
		}
		return m.openRenameInput(sess)
	}
	if sessions.IsFilterAction(action.Type) {
		// Tell sessionsView to apply the filter
		m.sessionsView.ApplyStatusFilter(action.Type)
		return m, nil
	}
	return m.dispatchAction(action)
}

func (m Model) handleSessionFormCommand(msg sessions.FormCommandRequestMsg) (tea.Model, tea.Cmd) {
	return m.showFormOrExecute(msg.Name, msg.Cmd, msg.Session, nil)
}

func (m Model) handleSessionCommandPalette(msg sessions.CommandPaletteRequestMsg) (tea.Model, tea.Cmd) {
	m.modals.CommandPalette = NewCommandPalette(m.mergedCommands, msg.Session, m.width, m.height, m.activeView)
	m.state = stateCommandPalette
	return m, nil
}

func (m Model) handleSessionNewSession() (tea.Model, tea.Cmd) {
	return m.openNewSessionForm()
}

func (m Model) handleSessionRename(msg sessions.RenameRequestMsg) (tea.Model, tea.Cmd) {
	return m.openRenameInput(msg.Session)
}

func (m Model) handleSessionDocReview() (tea.Model, tea.Cmd) {
	cmd := HiveDocReviewCmd{Arg: ""}
	return m, cmd.Execute(&m)
}

func (m Model) handleSessionRecycledDelete(msg sessions.RecycledDeleteRequestMsg) (tea.Model, tea.Cmd) {
	confirmMsg := fmt.Sprintf("Permanently delete %d recycled session(s)?", len(msg.Sessions))
	m.state = stateConfirming
	m.modals.Pending = Action{
		Type:    act.TypeDeleteRecycledBatch,
		Key:     "",
		Help:    "delete recycled sessions",
		Confirm: confirmMsg,
	}
	m.modals.PendingRecycledSessions = msg.Sessions
	m.modals.Confirm = NewModal("Confirm", confirmMsg)
	return m, nil
}

func (m Model) handleSessionOpenRepo(msg sessions.OpenRepoRequestMsg) (tea.Model, tea.Cmd) {
	return m.openRepoHeaderByRemote(msg.Name, msg.Remote)
}

// --- Action results ---

func (m Model) handleRenameComplete(msg renameCompleteMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		log.Error().Err(msg.err).Msg("rename failed")
		m.state = stateNormal
		return m, m.notifyError("rename failed: %v", msg.err)
	}
	return m, func() tea.Msg { return sessions.RefreshSessionsMsg{} }
}

func (m Model) handleActionComplete(msg actionCompleteMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		log.Error().Err(msg.err).Msg("action failed")
		m.state = stateNormal
		m.modals.Pending = Action{}
		return m, m.notifyError("action failed: %v", msg.err)
	}
	m.state = stateNormal
	m.modals.Pending = Action{}
	return m, func() tea.Msg { return sessions.RefreshSessionsMsg{} }
}

func (m Model) handleRecycleStarted(msg recycleStartedMsg) (tea.Model, tea.Cmd) {
	m.state = stateRunningRecycle
	m.modals.ShowOutputModal("Recycling session...")
	m.modals.RecycleOutput = msg.output
	m.modals.RecycleDone = msg.done
	m.modals.RecycleCancel = msg.cancel
	return m, tea.Batch(
		listenForRecycleOutput(msg.output, msg.done),
		m.modals.Output.Spinner().Tick,
	)
}

func (m Model) handleRecycleOutput(msg recycleOutputMsg) (tea.Model, tea.Cmd) {
	m.modals.Output.AddLine(msg.line)
	return m, listenForRecycleOutput(m.modals.RecycleOutput, m.modals.RecycleDone)
}

func (m Model) handleRecycleComplete(msg recycleCompleteMsg) (tea.Model, tea.Cmd) {
	m.modals.Output.SetComplete(msg.err)
	m.modals.RecycleOutput = nil
	m.modals.RecycleDone = nil
	m.modals.RecycleCancel = nil
	return m, nil
}

// --- Review delegation ---

func (m Model) handleReviewDocChange(msg review.DocumentChangeMsg) (tea.Model, tea.Cmd) {
	if m.reviewView != nil {
		var cmd tea.Cmd
		*m.reviewView, cmd = m.reviewView.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) handleReviewFinalized(msg review.ReviewFinalizedMsg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	// Send to agent inbox if requested, otherwise copy to clipboard
	if msg.SendToAgent != "" && m.msgService != nil {
		svc := m.msgService
		sessionID := msg.SendToAgent
		feedback := msg.Feedback
		cmds = append(cmds, func() tea.Msg {
			inboxTopic := "agent." + sessionID + ".inbox"
			pubMsg := messaging.Message{
				Topic:   inboxTopic,
				Payload: feedback,
				Sender:  "operator",
			}
			if err := svc.Publish(context.Background(), pubMsg, []string{inboxTopic}); err != nil {
				log.Error().Err(err).Str("topic", inboxTopic).Msg("failed to send feedback to agent")
				return notificationMsg{notification: notify.Notification{
					Level:   notify.LevelError,
					Message: fmt.Sprintf("failed to send feedback: %v", err),
				}}
			}
			return notificationMsg{notification: notify.Notification{
				Level:   notify.LevelInfo,
				Message: "feedback sent to agent inbox",
			}}
		})
	} else if err := m.copyToClipboard(msg.Feedback); err != nil {
		cmds = append(cmds, m.notifyError("failed to copy feedback: %v", err))
	}

	// Auto-complete TODO items associated with the finalized document
	if msg.DocumentPath != "" && m.todoService != nil {
		svc := m.todoService
		docPath := msg.DocumentPath
		cmds = append(cmds, func() tea.Msg {
			if err := svc.CompleteByPath(context.Background(), docPath); err != nil {
				log.Error().Err(err).Str("path", docPath).Msg("todo: auto-complete by path failed")
			}
			count, _ := svc.CountPending(context.Background())
			return todoCountUpdatedMsg{count: count}
		})
	}

	return m, tea.Batch(cmds...)
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
	if m.modals.DocPicker != nil {
		modal, cmd := m.modals.DocPicker.Update(msg)
		m.modals.DocPicker = modal

		if m.modals.DocPicker.SelectedDocument() != nil {
			doc := m.modals.DocPicker.SelectedDocument()
			m.modals.DocPicker = nil
			m.activeView = ViewReview
			m.handler.SetActiveView(ViewReview)
			if m.reviewView != nil {
				m.reviewView.LoadDocument(doc)
			}
			return m, cmd
		}

		if m.modals.DocPicker.Cancelled() {
			m.modals.DocPicker = nil
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
// This includes internal messages from sub-models (sessions, messages, review).
func (m Model) handleFallthrough(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.state == stateCreatingSession && m.modals.NewSession != nil {
		return m.updateNewSessionForm(msg)
	}

	var cmds []tea.Cmd

	// sessions and messages views receive ALL fallthrough messages unconditionally —
	// not gated on activeView — because both run background polling ticks
	// (terminal status, git refresh, message polling) that must fire regardless
	// of which tab is currently displayed.
	if m.sessionsView != nil {
		cmd := m.sessionsView.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	if m.msgView != nil {
		if cmd := m.msgView.Update(msg); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	// Forward to review view
	if m.activeView == ViewReview && m.reviewView != nil {
		var cmd tea.Cmd
		*m.reviewView, cmd = m.reviewView.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	return m, tea.Batch(cmds...)
}

// --- Helper for repo header opening ---

func (m Model) openRepoHeaderByRemote(name, remote string) (tea.Model, tea.Cmd) {
	var repoPath string
	for _, repo := range m.sessionsView.DiscoveredRepos() {
		if repo.Remote == remote {
			repoPath = repo.Path
			break
		}
	}
	if repoPath == "" {
		return m, m.notifyError("no local repository found for %s", name)
	}

	shellCmd, err := m.renderer.Render(
		`{{ hiveTmux }} {{ .Name | shq }} {{ .Path | shq }}`,
		struct{ Name, Path string }{Name: name, Path: repoPath},
	)
	if err != nil {
		return m, m.notifyError("template error: %v", err)
	}

	action := Action{
		Type:     act.TypeShell,
		Key:      "enter",
		Help:     "open repo",
		ShellCmd: shellCmd,
		Silent:   true,
		Exit:     config.ParseExitCondition("$HIVE_POPUP"),
	}

	if action.Exit {
		exec, err := m.cmdService.CreateExecutor(action)
		if err != nil {
			log.Error().Err(err).Msg("failed to create executor for repo open")
		} else if err := command.ExecuteSync(context.Background(), exec); err != nil {
			log.Error().Err(err).Msg("repo open command failed")
		}
		return m.quit()
	}

	return m, m.executeAction(action)
}

// refreshSessions returns a command that tells the sessions view to reload.
func (m Model) refreshSessions() tea.Cmd {
	return func() tea.Msg { return sessions.RefreshSessionsMsg{} }
}

// handleRecycleModalKey handles keys when recycle modal is shown.
func (m Model) handleRecycleModalKey(keyStr string) (tea.Model, tea.Cmd) {
	switch keyStr {
	case keyCtrlC:
		if m.modals.RecycleCancel != nil {
			m.modals.RecycleCancel()
		}
		return m.quit()
	case "esc":
		if m.modals.Output.IsRunning() && m.modals.RecycleCancel != nil {
			m.modals.RecycleCancel()
		}
		m.state = stateNormal
		m.modals.Pending = Action{}
		return m, m.refreshSessions()
	case keyEnter:
		if !m.modals.Output.IsRunning() {
			m.state = stateNormal
			m.modals.Pending = Action{}
			return m, m.refreshSessions()
		}
	}
	return m, nil
}

// handleConfirmModalKey handles keys when confirmation modal is shown.
func (m Model) handleConfirmModalKey(keyStr string) (tea.Model, tea.Cmd) {
	switch keyStr {
	case keyEnter:
		m.state = stateNormal
		if m.modals.Confirm.ConfirmSelected() {
			action := m.modals.Pending
			if action.Type == act.TypeRecycle {
				return m, m.startRecycle(action.SessionID)
			}
			if action.Type == act.TypeDeleteRecycledBatch {
				recycled := m.modals.PendingRecycledSessions
				m.modals.Pending = Action{}
				m.modals.PendingRecycledSessions = nil
				return m, m.deleteRecycledSessionsBatch(recycled)
			}
			return m, m.executeAction(action)
		}
		m.modals.Pending = Action{}
		m.modals.PendingRecycledSessions = nil
		return m, nil
	case "esc":
		m.state = stateNormal
		m.modals.Pending = Action{}
		m.modals.PendingRecycledSessions = nil
		return m, nil
	case "left", "right", "h", "l", "tab":
		m.modals.Confirm.ToggleSelection()
		return m, nil
	}
	return m, nil
}

// --- TODO panel ---

func (m Model) handleTodoPanelKey(msg tea.KeyMsg, keyStr string) (tea.Model, tea.Cmd) {
	if keyStr == keyCtrlC {
		return m.quit()
	}

	panel := m.modals.TodoPanel
	if panel == nil {
		m.state = stateNormal
		return m, nil
	}

	panel, _ = panel.Update(msg)
	m.modals.TodoPanel = panel

	if panel.Cancelled() {
		m.state = stateNormal
		m.modals.DismissTodoPanel()
		return m, nil
	}

	if result := panel.Result(); result != nil {
		m.state = stateNormal
		m.modals.DismissTodoPanel()

		switch result.Action { //nolint:exhaustive // TodoPanelNone is a no-op
		case TodoPanelSelect:
			// Open the file in review tab if it has a file path
			if result.Item.FilePath != "" && m.reviewView != nil {
				// Set source session so "Send to agent" option is available
				m.reviewView.SetSourceSession(result.Item.SessionID)
				m.activeView = ViewReview
				m.handler.SetActiveView(ViewReview)
				m.sessionsView.SetActive(false)
				return m, m.reviewView.OpenDocumentByPath(result.Item.FilePath)
			}
		case TodoPanelDismiss:
			return m, m.dismissTodoItem(result.Item.ID)
		case TodoPanelComplete:
			return m, m.completeTodoItem(result.Item.ID)
		}
	}

	return m, nil
}

func (m Model) openTodoPanel() (tea.Model, tea.Cmd) {
	return m, m.loadTodosAndShowPanel()
}

func (m Model) loadTodosAndShowPanel() tea.Cmd {
	svc := m.todoService
	return func() tea.Msg {
		items, err := svc.ListPending(context.Background(), todo.ListFilter{})
		if err != nil {
			log.Error().Err(err).Msg("todo: list pending for panel")
			return todoPanelLoadedMsg{items: nil}
		}
		return todoPanelLoadedMsg{items: items}
	}
}

func (m Model) dismissTodoItem(id string) tea.Cmd {
	svc := m.todoService
	return func() tea.Msg {
		if err := svc.Dismiss(context.Background(), id); err != nil {
			log.Error().Err(err).Str("id", id).Msg("todo: dismiss failed")
		}
		// Refresh count
		count, _ := svc.CountPending(context.Background())
		return todoCountUpdatedMsg{count: count}
	}
}

func (m Model) completeTodoItem(id string) tea.Cmd {
	svc := m.todoService
	return func() tea.Msg {
		if err := svc.Complete(context.Background(), id); err != nil {
			log.Error().Err(err).Str("id", id).Msg("todo: complete failed")
		}
		count, _ := svc.CountPending(context.Background())
		return todoCountUpdatedMsg{count: count}
	}
}

// --- TODO tracking ---

func (m Model) handleTodoFileChange(msg hive.TodoFileChangeMsg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	ctx := context.Background()
	var newItems []todo.Item
	for _, path := range msg.Changed {
		item, err := m.todoService.HandleFileEvent(ctx, path, m.todoRepoRemote)
		if err != nil {
			log.Error().Err(err).Str("path", path).Msg("todo: handle file event")
			continue
		}
		if item != nil {
			newItems = append(newItems, *item)
		}
	}
	for _, path := range msg.Deleted {
		if err := m.todoService.HandleFileDelete(ctx, path); err != nil {
			log.Error().Err(err).Str("path", path).Msg("todo: handle file delete")
		}
	}

	// Notify for new items
	for _, item := range newItems {
		if m.cfg.Todo.Notifications.ToastEnabled() {
			m.notifyBus.Infof("New todo: %s", item.Title)
		}
		if m.cfg.Todo.Notifications.TerminalEnabled() {
			cmds = append(cmds, sendTerminalNotification(item))
		}
	}
	if len(newItems) > 0 && m.cfg.Todo.Notifications.ToastEnabled() {
		cmds = append(cmds, m.ensureToastTick())
	}

	// Refresh count and re-subscribe to watcher
	cmds = append(cmds, m.loadTodoCount())
	if m.todoWatcher != nil {
		cmds = append(cmds, m.todoWatcher.Start())
	}
	return m, tea.Batch(cmds...)
}

func (m Model) handleTodoCountUpdated(msg todoCountUpdatedMsg) (tea.Model, tea.Cmd) {
	m.todoPendingCount = msg.count
	return m, nil
}

func (m Model) handleTodoPanelLoaded(msg todoPanelLoadedMsg) (tea.Model, tea.Cmd) {
	if len(msg.items) == 0 {
		return m, m.notifyError("no pending TODO items")
	}
	m.modals.ShowTodoPanel(msg.items)
	m.state = stateShowingTodoPanel
	return m, nil
}

func (m Model) loadTodoCount() tea.Cmd {
	svc := m.todoService
	return func() tea.Msg {
		count, err := svc.CountPending(context.Background())
		if err != nil {
			log.Error().Err(err).Msg("todo: count pending")
			return todoCountUpdatedMsg{count: 0}
		}
		return todoCountUpdatedMsg{count: count}
	}
}

// selectedSession returns the session selected in the sessions view, or nil.
func (m Model) selectedSession() *session.Session {
	if m.sessionsView == nil {
		return nil
	}
	return m.sessionsView.SelectedSession()
}
