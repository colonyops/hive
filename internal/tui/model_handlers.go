package tui

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/rs/zerolog/log"

	act "github.com/colonyops/hive/internal/core/action"
	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/internal/core/notify"
	"github.com/colonyops/hive/internal/core/session"
	"github.com/colonyops/hive/internal/core/todo"
	"github.com/colonyops/hive/internal/tui/command"
	"github.com/colonyops/hive/internal/tui/views/review"
	"github.com/colonyops/hive/internal/tui/views/sessions"
	"github.com/colonyops/hive/pkg/tmpl"
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
			m.publishNotificationf(notify.LevelWarning, "%s", w)
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

func (m Model) handleTodoPollTick() (tea.Model, tea.Cmd) {
	return m, tea.Batch(m.loadTodoCounts(), scheduleTodoPollTick())
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
	if action.Type == act.TypeTodoPanel {
		m.state = stateShowingTodos
		m.modals.ShowTodoPanel(m.todoService)
		if failures := m.modals.TodoPanel.AcknowledgeErrorCount(); failures > 0 {
			return m, m.notifyError("failed to acknowledge %d todo(s)", failures)
		}
		return m, nil
	}
	if action.Type == act.TypeRenameSession {
		sess := m.sessionsView.SelectedSession()
		if sess == nil {
			return m, nil
		}
		return m.openRenameInput(sess)
	}
	if action.Type == act.TypeGroupSet {
		sess := m.sessionsView.SelectedSession()
		if sess == nil {
			return m, nil
		}
		return m.openGroupInput(sess)
	}
	if action.Type == act.TypeGroupToggle {
		cmd := m.sessionsView.ToggleGroupBy()
		return m, cmd
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

func (m Model) handleSetGroupComplete(msg setGroupCompleteMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		log.Error().Err(msg.err).Msg("set group failed")
		m.state = stateNormal
		return m, m.notifyError("set group failed: %v", msg.err)
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

func (m Model) handleStreamStarted(msg streamStartedMsg) (tea.Model, tea.Cmd) {
	m.state = stateStreaming
	m.modals.ShowOutputModal(msg.title)
	m.modals.StreamOutput = msg.output
	m.modals.StreamDone = msg.done
	m.modals.StreamCancel = msg.cancel
	m.modals.StreamResult = msg.result
	return m, tea.Batch(
		listenForStreamingOutput(msg.output, msg.done),
		m.modals.Output.Spinner().Tick,
	)
}

func (m Model) handleStreamOutput(msg streamOutputMsg) (tea.Model, tea.Cmd) {
	m.modals.Output.AddLine(msg.line)
	return m, listenForStreamingOutput(m.modals.StreamOutput, m.modals.StreamDone)
}

func (m Model) handleStreamComplete(msg streamCompleteMsg) (tea.Model, tea.Cmd) {
	result := m.modals.StreamResult
	m.modals.StreamOutput = nil
	m.modals.StreamDone = nil
	m.modals.StreamCancel = nil
	m.modals.StreamResult = streamResult{}

	if msg.err == nil {
		// Auto-close on success
		m.state = stateNormal
		m.modals.Pending = Action{}
		if result.sessionID != nil && *result.sessionID != "" {
			m.sessionsView.SelectOnNextRefresh(*result.sessionID)
		}
		cmds := []tea.Cmd{m.refreshSessions()}
		if result.sessionName != nil && *result.sessionName != "" {
			cmds = append(cmds, switchTmuxSession(*result.sessionName))
		}
		return m, tea.Batch(cmds...)
	}

	m.modals.Output.SetComplete(msg.err)
	return m, nil
}

// handleStreamingModalKey handles keys when a streaming output modal is shown.
func (m Model) handleStreamingModalKey(keyStr string) (tea.Model, tea.Cmd) {
	switch keyStr {
	case keyCtrlC:
		if m.modals.StreamCancel != nil {
			m.modals.StreamCancel()
		}
		return m.quit()
	case "esc":
		if m.modals.Output.IsRunning() && m.modals.StreamCancel != nil {
			m.modals.StreamCancel()
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
	if err := m.copyToClipboard(msg.Feedback); err != nil {
		return m, m.notifyError("failed to copy feedback: %v", err)
	}

	// Auto-complete todos whose ref matches the finalized document
	if msg.DocumentPath != "" || msg.DocumentRel != "" {
		return m, m.completeTodosMatchingRef(msg.DocumentPath, msg.DocumentRel)
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
	n := msg.notification
	if n.CreatedAt.IsZero() {
		n.CreatedAt = time.Now()
	}

	// Persist to store
	if m.notifyStore != nil {
		if _, err := m.notifyStore.Save(context.Background(), n); err != nil {
			log.Error().Err(err).Str("message", n.Message).Msg("failed to persist notification")
		}
	}

	// Push to toast controller
	m.toastController.Push(n)

	// Re-subscribe for the next notification
	var cmds []tea.Cmd
	if msg.ch != nil {
		cmds = append(cmds, m.waitForNotification(msg.ch))
	}
	if tickCmd := m.ensureToastTick(); tickCmd != nil {
		cmds = append(cmds, tickCmd)
	}
	return m, tea.Batch(cmds...)
}

func (m Model) handleUpdateAvailable(msg updateAvailableMsg) (tea.Model, tea.Cmd) {
	if msg.result == nil {
		return m, nil
	}

	m.updateInfo = msg.result
	m.notifyBus.Infof("Update available: %s -> %s", msg.result.Current, msg.result.Latest)
	return m, m.ensureToastTick()
}

// --- Todo Panel ---

func (m Model) handleTodoPanelKey(keyStr string) (tea.Model, tea.Cmd) {
	switch keyStr {
	case keyCtrlC:
		return m.quit()
	case "esc", "q":
		m.state = stateNormal
		m.todoBadge.clearPendingWithOpen(m.modals.TodoPanel.OpenCount())
		m.modals.DismissTodoPanel()
		return m, nil
	case "j", "down":
		m.modals.TodoPanel.MoveDown()
	case "k", "up":
		m.modals.TodoPanel.MoveUp()
	case keyEnter:
		item := m.modals.TodoPanel.CurrentItem()
		if item == nil {
			return m, nil
		}
		if item.Status == todo.StatusCompleted {
			return m, nil
		}
		if item.URI.IsEmpty() {
			m.notifyBus.Infof("no URI on this todo")
			return m, m.ensureToastTick()
		}
		switch item.URI.Scheme() {
		case "session":
			sessionID := item.URI.Value()
			var found *session.Session
			for _, s := range m.sessionsView.AllSessions() {
				if s.ID == sessionID {
					found = &s
					break
				}
			}
			if found == nil {
				return m, m.notifyError("session %q not found", sessionID)
			}
			if err := m.modals.TodoPanel.CompleteCurrent(); err != nil {
				return m, m.notifyError("complete todo: %v", err)
			}
			m.state = stateNormal
			m.todoBadge.clearPendingWithOpen(m.modals.TodoPanel.OpenCount())
			m.modals.DismissTodoPanel()
			return m, m.executeAction(act.Action{
				Type:        act.TypeTmuxOpen,
				SessionName: found.Name,
				SessionPath: found.Path,
			})
		case "review":
			if m.reviewView != nil {
				m.state = stateNormal
				m.todoBadge.clearPendingWithOpen(m.modals.TodoPanel.OpenCount())
				m.modals.DismissTodoPanel()
				m.activeView = ViewReview
				m.handler.SetActiveView(ViewReview)
				return m, m.reviewView.OpenDocumentByPath(item.URI.Value())
			}
			return m, m.notifyError("review view unavailable")
		case "http", "https":
			return m, launchAction(item.ID, osOpenCmd(item.URI.String()))
		default:
			var actionCmd *exec.Cmd
			if action, ok := m.cfg.Todos.Actions[item.URI.Scheme()]; ok {
				var err error
				actionCmd, err = renderCustomAction(action, item.URI)
				if err != nil {
					log.Warn().Err(err).Str("scheme", item.URI.Scheme()).Msg("failed to render custom action")
					return m, m.notifyError("render action: %v", err)
				}
			} else {
				actionCmd = osOpenCmd(item.URI.String())
			}
			return m, launchAction(item.ID, actionCmd)
		}
	case "tab":
		m.modals.TodoPanel.CycleFilter()
	case "c":
		if err := m.modals.TodoPanel.CompleteCurrent(); err != nil {
			return m, m.notifyError("complete todo: %v", err)
		}
	case "d":
		if err := m.modals.TodoPanel.DismissCurrent(); err != nil {
			return m, m.notifyError("dismiss todo: %v", err)
		}
	}
	return m, nil
}

// completeTodosMatchingRef completes all open review todos whose URI value matches any of the given paths,
// then returns updated todo counts directly (avoiding a double loadTodoCounts invocation).
func (m Model) completeTodosMatchingRef(paths ...string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		items, err := m.todoService.List(ctx, todo.ListFilter{})
		if err != nil {
			log.Warn().Err(err).Msg("failed to list todos for auto-complete")
			return notificationMsg{notification: notify.Notification{Level: notify.LevelWarning, Message: "Auto-complete failed: unable to list todos"}}
		}
		failed := 0
		for _, item := range items {
			if item.Status != todo.StatusPending && item.Status != todo.StatusAcknowledged {
				continue
			}
			if item.URI.Scheme() != "review" {
				continue
			}
			for _, p := range paths {
				if p != "" && (item.URI.Value() == p || strings.HasSuffix(p, "/"+item.URI.Value()) || strings.HasSuffix(item.URI.Value(), "/"+p)) {
					if _, err := m.todoService.Complete(ctx, item.ID); err != nil {
						failed++
						log.Warn().Err(err).Str("id", item.ID).Msg("failed to auto-complete todo")
					}
					break
				}
			}
		}

		// Inline count refresh instead of calling loadTodoCounts()()
		pending, err := m.todoService.CountPending(ctx)
		if err != nil {
			log.Warn().Err(err).Msg("failed to load todo pending count after auto-complete")
			return notificationMsg{notification: notify.Notification{Level: notify.LevelWarning, Message: "Auto-complete finished, but todo counts could not be refreshed"}}
		}
		open, err := m.todoService.CountOpen(ctx)
		if err != nil {
			log.Warn().Err(err).Msg("failed to load todo open count after auto-complete")
			return notificationMsg{notification: notify.Notification{Level: notify.LevelWarning, Message: "Auto-complete finished, but todo counts could not be refreshed"}}
		}
		return todoAutoCompleteResultMsg{pendingCount: pending, openCount: open, failed: failed}
	}
}

// --- Input ---

func (m Model) handleKeyMsg(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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
	var cmds []tea.Cmd

	var cmd tea.Cmd
	m.spinner, cmd = m.spinner.Update(msg)
	if cmd != nil {
		cmds = append(cmds, cmd)
	}

	// Route spinner ticks to the output modal when active
	if m.state == stateStreaming {
		s := m.modals.Output.Spinner()
		s, cmd = s.Update(msg)
		if cmd != nil {
			m.modals.Output.SetSpinner(s)
			m.modals.Output.AdvanceFrame()
			cmds = append(cmds, cmd)
		}
	}

	return m, tea.Batch(cmds...)
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

// switchTmuxSession returns a command that switches the tmux client to the
// named session. Errors are logged but not surfaced — the session is already
// created and the user can switch manually.
func switchTmuxSession(name string) tea.Cmd {
	return func() tea.Msg {
		if err := exec.Command("tmux", "switch-client", "-t", name).Run(); err != nil {
			log.Debug().Err(err).Str("session", name).Msg("tmux switch-client failed")
		}
		return nil
	}
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

// selectedSession returns the session selected in the sessions view, or nil.
func (m Model) selectedSession() *session.Session {
	if m.sessionsView == nil {
		return nil
	}
	return m.sessionsView.SelectedSession()
}

// --- Todo action helpers ---

// actionResultMsg is returned when an external todo action completes.
type actionResultMsg struct {
	TodoID string
	Err    error
}

func launchAction(todoID string, cmd *exec.Cmd) tea.Cmd {
	return func() tea.Msg {
		err := cmd.Run()
		return actionResultMsg{TodoID: todoID, Err: err}
	}
}

func osOpenCmd(uri string) *exec.Cmd {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", uri)
	default:
		return exec.Command("xdg-open", uri)
	}
}

func renderCustomAction(tmplStr string, ref todo.Ref) (*exec.Cmd, error) {
	renderer := tmpl.New(tmpl.Config{})
	rendered, err := renderer.Render(tmplStr, config.ActionTemplateData{
		Scheme: ref.Scheme(),
		Value:  ref.Value(),
		URI:    ref.String(),
	})
	if err != nil {
		return nil, fmt.Errorf("render action template: %w", err)
	}
	return exec.Command("sh", "-c", rendered), nil
}
