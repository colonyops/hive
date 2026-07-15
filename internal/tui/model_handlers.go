package tui

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/rs/zerolog/log"

	act "github.com/colonyops/hive/internal/core/action"
	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/internal/core/git"
	"github.com/colonyops/hive/internal/core/hc"
	"github.com/colonyops/hive/internal/core/notify"
	"github.com/colonyops/hive/internal/core/session"
	"github.com/colonyops/hive/internal/core/todo"
	"github.com/colonyops/hive/internal/core/workspace"
	"github.com/colonyops/hive/internal/hive"
	"github.com/colonyops/hive/internal/sources"
	"github.com/colonyops/hive/internal/tui/command"
	"github.com/colonyops/hive/internal/tui/sourcepicker"
	"github.com/colonyops/hive/internal/tui/views/review"
	"github.com/colonyops/hive/internal/tui/views/sessions"
	"github.com/colonyops/hive/internal/tui/views/tasks"
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

	if m.msgView != nil {
		m.msgView.SetSize(msg.Width, contentHeight)
	}

	if m.reviewView != nil {
		m.reviewView.SetSize(msg.Width, contentHeight)
	}

	m.kvView.SetSize(msg.Width, contentHeight)

	if m.tasksView != nil {
		m.tasksView.SetSize(msg.Width, contentHeight)
	}

	// Publish startup warnings on the first WindowSizeMsg
	if len(m.startupWarnings) > 0 {
		for _, w := range m.startupWarnings {
			m.publishNotificationf(notify.LevelWarning, "%s", w)
		}
		m.startupWarnings = nil
		return m, nil
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
		m.notifyErrorf("keybinding error: %v", action.Err)
		return m, nil
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
	if action.Type == act.TypeViewTasks {
		return m.viewTasksForSelectedSession()
	}
	if action.Type == act.TypeOpenSourcePicker {
		sourceID, err := m.resolveSourceID(action.Args)
		if err != nil {
			m.notifyErrorf("%v", err)
			return m, nil
		}
		return m.openSourcePicker(sourceID, m.sourcePickerScopeForSelection(m.selectedSession(), action.Args))
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
	m.modals.CommandPalette = NewCommandPalette(m.commandSet.All(), msg.Session, m.width, m.height, m.activeView)
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

func (m Model) viewTasksForSelectedSession() (tea.Model, tea.Cmd) {
	sess := m.sessionsView.SelectedSession()
	if sess == nil || sess.Remote == "" {
		return m, nil
	}
	if m.tasksView == nil {
		return m, nil
	}
	owner, repo := git.ExtractOwnerRepo(sess.Remote)
	if owner == "" || repo == "" {
		return m, nil
	}
	repoKey := owner + "/" + repo
	cmd := m.tasksView.SetRepoKey(repoKey)

	// Switch to tasks tab.
	m.activeView = ViewTasks
	m.handler.SetActiveView(ViewTasks)
	m.sessionsView.SetActive(false)
	m.tasksView.SetActive(true)

	return m, cmd
}

func (m Model) handleSessionOpenRepo(msg sessions.OpenRepoRequestMsg) (tea.Model, tea.Cmd) {
	return m.openRepoHeaderByRemote(msg.Name, msg.Remote)
}

// --- Outbound messages from tasks view ---

func (m Model) handleTaskAction(msg tasks.ActionRequestMsg) (tea.Model, tea.Cmd) {
	a := msg.Action
	if a.Err != nil {
		m.notifyErrorf("keybinding error: %v", a.Err)
		return m, nil
	}

	switch a.Type {
	case act.TypeTasksRefresh:
		return m, func() tea.Msg { return tasks.RefreshTasksMsg{} }
	case act.TypeTasksFilter:
		if m.tasksView != nil {
			return m, m.tasksView.CycleFilter()
		}
		return m, nil
	case act.TypeTasksCopyID:
		if m.tasksView != nil {
			if item := m.tasksView.SelectedItem(); item != nil {
				if err := m.copyToClipboard(item.ID); err != nil {
					m.notifyErrorf("copy failed: %v", err)
				} else {
					m.publishNotificationf(notify.LevelInfo, "Copied %s", item.ID)
				}
			}
		}
		return m, nil
	case act.TypeTasksTogglePreview:
		if m.tasksView != nil {
			return m, m.tasksView.TogglePreview()
		}
		return m, nil
	case act.TypeTasksSelectRepo:
		if m.tasksView != nil && m.tasksView.Svc() != nil {
			return m, m.loadRepoKeys()
		}
		return m, nil

	// Direct status changes (no confirmation)
	case act.TypeTasksSetOpen:
		return m, m.taskUpdateStatus(hc.StatusOpen)
	case act.TypeTasksSetInProgress:
		return m, m.taskUpdateStatus(hc.StatusInProgress)
	case act.TypeTasksSetDone:
		return m, m.taskUpdateStatus(hc.StatusDone)

	// Confirmation-required actions
	case act.TypeTasksSetCancelled:
		if m.tasksView == nil {
			return m, nil
		}
		item := m.tasksView.SelectedItem()
		if item == nil {
			return m, nil
		}
		a.SessionID = item.ID
		a.SessionName = item.Title
		a.Confirm = fmt.Sprintf("Cancel task %q?", item.Title)
		return m.dispatchAction(a)
	case act.TypeTasksDelete:
		if m.tasksView == nil {
			return m, nil
		}
		item := m.tasksView.SelectedItem()
		if item == nil {
			return m, nil
		}
		a.SessionID = item.ID
		a.SessionName = item.Title
		a.Confirm = fmt.Sprintf("Delete %q? This cannot be undone.", item.Title)
		return m.dispatchAction(a)
	case act.TypeTasksPrune:
		a.Confirm = "Remove all done/cancelled items older than 24h?"
		return m.dispatchAction(a)

	default:
		return m.handleGlobalAction(a)
	}
}

// taskUpdateStatus returns a command that updates the selected task's status.
func (m Model) taskUpdateStatus(status hc.Status) tea.Cmd {
	if m.tasksView == nil {
		return nil
	}
	item := m.tasksView.SelectedItem()
	if item == nil {
		return nil
	}
	svc := m.tasksView.Svc()
	if svc == nil {
		return nil
	}
	id := item.ID
	return func() tea.Msg {
		_, err := svc.UpdateItem(context.Background(), id, hc.ItemUpdate{Status: &status})
		return tasks.TaskActionCompleteMsg{Err: err}
	}
}

// handleGlobalAction handles actions that are not view-specific (info, doctor, theme, etc.).
// Used as a fallback when a view receives an action it doesn't own.
func (m Model) handleGlobalAction(a Action) (tea.Model, tea.Cmd) {
	switch a.Type {
	case act.TypeHiveInfo:
		return m.showHiveInfo()
	case act.TypeHiveDoctor:
		return m.showHiveDoctor()
	case act.TypeNotifications:
		m.state = stateShowingNotifications
		m.modals.ShowNotifications(m.notifyStore)
		return m, nil
	case act.TypeSetTheme:
		return m, nil
	case act.TypeQuit:
		return m.quit()
	case act.TypeShowHelp:
		if !m.isReviewFocused() {
			return m.showHelpDialog()
		}
		// Review renders its own help overlay, so the global help modal stays out of the way here.
		return m, nil
	default:
		log.Warn().Str("type", string(a.Type)).Msg("unhandled action type")
		return m, nil
	}
}

// repoKeysLoadedMsg carries the result of loading repo keys.
type repoKeysLoadedMsg struct {
	repos []string
	err   error
}

func (m Model) loadRepoKeys() tea.Cmd {
	svc := m.tasksView.Svc()
	return func() tea.Msg {
		repos, err := svc.ListRepoKeys(context.Background())
		return repoKeysLoadedMsg{repos: repos, err: err}
	}
}

func (m Model) handleRepoKeysLoaded(msg repoKeysLoadedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		return m, m.notifyError("load repos: %v", msg.err)
	}
	if len(msg.repos) == 0 {
		return m, m.notifyError("no repositories found")
	}
	currentRepo := ""
	if m.tasksView != nil {
		currentRepo = m.tasksView.RepoKey()
	}
	m.modals.RepoPicker = NewRepoPicker(msg.repos, currentRepo, m.width, m.height)
	m.state = stateSelectingRepo
	return m, nil
}

// docsRepoEntry holds a repo key and its context directory for the docs repo picker.
type docsRepoEntry struct {
	key        string // "owner/repo"
	contextDir string
}

// docsRepoKeysLoadedMsg carries the result of loading docs repo keys from sessions.
type docsRepoKeysLoadedMsg struct {
	repos []docsRepoEntry
	err   error
}

func (m Model) loadDocsRepoKeys() tea.Cmd {
	svc := m.service
	cfg := m.cfg
	return func() tea.Msg {
		sessions, err := svc.ListSessions(context.Background())
		if err != nil {
			return docsRepoKeysLoadedMsg{err: err}
		}
		seen := map[string]bool{}
		var repos []docsRepoEntry
		for _, s := range sessions {
			owner, repo := git.ExtractOwnerRepo(s.Remote)
			if owner == "" || repo == "" {
				continue
			}
			key := owner + "/" + repo
			if seen[key] {
				continue
			}
			seen[key] = true
			repos = append(repos, docsRepoEntry{
				key:        key,
				contextDir: cfg.RepoContextDir(owner, repo),
			})
		}
		return docsRepoKeysLoadedMsg{repos: repos}
	}
}

func (m Model) handleDocsRepoKeysLoaded(msg docsRepoKeysLoadedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		return m, m.notifyError("load repos: %v", msg.err)
	}
	if len(msg.repos) == 0 {
		return m, m.notifyError("no repositories found")
	}
	keys := make([]string, len(msg.repos))
	for i, r := range msg.repos {
		keys[i] = r.key
	}
	currentRepo := ""
	if m.reviewView != nil {
		currentRepo = m.reviewView.RepoKey()
	}
	m.modals.DocsRepoEntries = msg.repos
	m.modals.RepoPicker = NewRepoPicker(keys, currentRepo, m.width, m.height)
	m.state = stateSelectingRepo
	return m, nil
}

func (m Model) handleTaskCommandPalette(_ tasks.CommandPaletteRequestMsg) (tea.Model, tea.Cmd) {
	m.modals.CommandPalette = NewCommandPalette(m.commandSet.All(), nil, m.width, m.height, m.activeView)
	m.state = stateCommandPalette
	return m, nil
}

func (m Model) handleReviewCommandPalette() (tea.Model, tea.Cmd) {
	m.modals.CommandPalette = NewCommandPalette(m.commandSet.All(), nil, m.width, m.height, m.activeView)
	m.state = stateCommandPalette
	return m, nil
}

// --- Action results ---

func (m Model) handleRenameComplete(msg renameCompleteMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		log.Error().Err(msg.err).Msg("rename failed")
		m.state = stateNormal
		m.notifyErrorf("rename failed: %v", msg.err)
		return m, nil
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

func (m Model) handleTaskActionComplete(msg tasks.TaskActionCompleteMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		m.notifyErrorf("task action failed: %v", msg.Err)
		return m, nil
	}
	return m, func() tea.Msg { return tasks.RefreshTasksMsg{} }
}

func (m Model) handleReviewAction(msg review.ActionRequestMsg) (tea.Model, tea.Cmd) {
	a := msg.Action
	if a.Err != nil {
		m.notifyErrorf("keybinding error: %v", a.Err)
		return m, nil
	}

	var docPath, docRelPath string
	if m.reviewView != nil {
		if doc := m.reviewView.SelectedDoc(); doc != nil {
			docPath = doc.Path
			docRelPath = doc.RelPath
		}
	}

	switch a.Type {
	case act.TypeDocsCopyPath:
		if docPath == "" {
			return m, nil
		}
		if err := m.copyToClipboard(docPath); err != nil {
			m.notifyErrorf("copy failed: %v", err)
		} else {
			m.publishNotificationf(notify.LevelInfo, "Copied path")
		}
	case act.TypeDocsCopyRelPath:
		if docRelPath == "" {
			return m, nil
		}
		if err := m.copyToClipboard(docRelPath); err != nil {
			m.notifyErrorf("copy failed: %v", err)
		} else {
			m.publishNotificationf(notify.LevelInfo, "Copied relative path")
		}
	case act.TypeDocsCopyContents:
		if m.reviewView == nil {
			return m, nil
		}
		if doc := m.reviewView.SelectedDoc(); doc != nil {
			content, err := os.ReadFile(doc.Path)
			if err != nil {
				m.notifyErrorf("read failed: %v", err)
			} else if err := m.copyToClipboard(string(content)); err != nil {
				m.notifyErrorf("copy failed: %v", err)
			} else {
				m.publishNotificationf(notify.LevelInfo, "Copied contents")
			}
		}
	case act.TypeDocsOpen:
		if docPath == "" {
			return m, nil
		}
		editor := os.Getenv("EDITOR")
		if editor == "" {
			editor = "vi"
		}
		c := exec.Command(editor, docPath)
		c.Stdin = os.Stdin
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		if err := c.Run(); err != nil {
			m.notifyErrorf("open failed: %v", err)
		}
	case act.TypeDocsTogglePreview:
		if m.reviewView != nil {
			return m, m.reviewView.TogglePreview()
		}
	case act.TypeDocsToggleTree:
		if m.reviewView != nil {
			return m, m.reviewView.ToggleTree()
		}
	case act.TypeDocsSelectRepo:
		return m, m.loadDocsRepoKeys()
	default:
		return m.handleGlobalAction(a)
	}
	return m, nil
}

func (m Model) handleActionComplete(msg actionCompleteMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		log.Error().Err(msg.err).Msg("action failed")
		m.state = stateNormal
		m.modals.Pending = Action{}
		m.notifyErrorf("action failed: %v", msg.err)
		return m, nil
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

func (m Model) handleBgStreamStarted(msg bgStreamStartedMsg) (tea.Model, tea.Cmd) {
	m.state = stateNormal
	if m.modals.BgStreamDone != nil {
		if msg.cancel != nil {
			msg.cancel()
		}
		m.publishNotificationf(notify.LevelError, "Already running in background: %s", m.modals.BgStreamTitle)
		return m, listenForBgStreamComplete(msg.title, msg.output, msg.done, msg.result)
	}

	m.modals.BgStreamOutput = msg.output
	m.modals.BgStreamDone = msg.done
	m.modals.BgStreamCancel = msg.cancel
	m.modals.BgStreamResult = msg.result
	m.modals.BgStreamTitle = msg.title
	m.publishNotificationf(notify.LevelInfo, "Started in background: %s", msg.title)
	return m, listenForBgStreamComplete(msg.title, msg.output, msg.done, msg.result)
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
	case "b":
		if !m.modals.Output.IsRunning() {
			return m, nil
		}
		if m.modals.BgStreamDone != nil {
			m.publishNotificationf(notify.LevelError, "Already running in background: %s", m.modals.BgStreamTitle)
			return m, nil
		}
		// Move the running operation to background.
		m.modals.BgStreamOutput = m.modals.StreamOutput
		m.modals.BgStreamDone = m.modals.StreamDone
		m.modals.BgStreamCancel = m.modals.StreamCancel
		m.modals.BgStreamResult = m.modals.StreamResult
		m.modals.BgStreamTitle = m.modals.Output.title

		m.modals.StreamOutput = nil
		m.modals.StreamDone = nil
		m.modals.StreamCancel = nil
		m.modals.StreamResult = streamResult{}

		m.state = stateNormal
		m.modals.Pending = Action{}
		m.publishNotificationf(notify.LevelInfo, "Moved to background: %s", m.modals.BgStreamTitle)
		return m, listenForBgStreamComplete(m.modals.BgStreamTitle, m.modals.BgStreamOutput, m.modals.BgStreamDone, m.modals.BgStreamResult)
	case keyEnter:
		if !m.modals.Output.IsRunning() {
			m.state = stateNormal
			m.modals.Pending = Action{}
			return m, m.refreshSessions()
		}
	}
	return m, nil
}

// handleBgStreamComplete handles completion of a backgrounded streaming operation.
func (m Model) handleBgStreamComplete(msg bgStreamCompleteMsg) (tea.Model, tea.Cmd) {
	active := msg.done == m.modals.BgStreamDone
	if active {
		m.modals.BgStreamOutput = nil
		m.modals.BgStreamDone = nil
		m.modals.BgStreamCancel = nil
		m.modals.BgStreamResult = streamResult{}
		m.modals.BgStreamTitle = ""
	}

	if active {
		if msg.err == nil {
			m.publishNotificationf(notify.LevelInfo, "Complete: %s", msg.title)
		} else {
			m.publishNotificationf(notify.LevelError, "Failed: %s — %v", msg.title, msg.err)
		}
	}

	// A batch can fail partially: an error with a populated session ID means
	// some sessions were still created, so they must appear (and be
	// selected) without waiting for a manual or periodic refresh.
	if msg.result.sessionID != nil && *msg.result.sessionID != "" {
		m.sessionsView.SelectOnNextRefresh(*msg.result.sessionID)
	} else if msg.err != nil {
		return m, nil
	}
	return m, m.refreshSessions()
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
		m.notifyErrorf("failed to copy feedback: %v", err)
		return m, nil
	}

	m.publishNotificationf(notify.LevelInfo, "Review copied to clipboard")

	// Auto-complete todos whose ref matches the finalized document
	if msg.DocumentPath != "" || msg.DocumentRel != "" {
		return m, m.completeTodosMatchingRef(msg.DocumentPath, msg.DocumentRel)
	}

	return m, nil
}

func (m Model) handleReviewOpenDoc(msg review.OpenDocumentMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		m.notifyErrorf("open document: %v", msg.Err)
		return m, nil
	}
	if m.reviewView != nil {
		// Try to find the document in the indexed list first
		for _, item := range m.reviewView.List().Items() {
			if treeItem, ok := item.(review.TreeItem); ok && !treeItem.IsHeader {
				if treeItem.Document.Path == msg.Path {
					m.reviewView.LoadDocument(&treeItem.Document)
					return m, nil
				}
			}
		}

		// Document not in the list (cross-repo todo). Load directly from disk.
		m.reviewView.LoadDocumentFromPath(msg.Path)
	}
	return m, nil
}

// --- Notifications ---

func (m Model) handleDrainNotifications(_ drainNotificationsMsg) (tea.Model, tea.Cmd) {
	if m.notifyBuffer == nil {
		return m, nil
	}

	notifications := m.notifyBuffer.Drain()
	for _, n := range notifications {
		if n.CreatedAt.IsZero() {
			n.CreatedAt = time.Now()
		}

		if _, err := m.notifyStore.Save(context.Background(), n); err != nil {
			log.Error().Err(err).Str("message", n.Message).Msg("failed to persist notification")
		}

		m.toastController.Push(n)
	}

	cmds := []tea.Cmd{m.notifyBuffer.WaitForSignal()}
	if m.toastController.HasToasts() {
		cmds = append(cmds, scheduleToastTick())
	}

	return m, tea.Batch(cmds...)
}

func (m Model) handleUpdateAvailable(msg updateAvailableMsg) (tea.Model, tea.Cmd) {
	if msg.result == nil {
		return m, nil
	}

	m.updateInfo = msg.result
	m.publishNotificationf(notify.LevelInfo, "Update available: %s -> %s", msg.result.Current, msg.result.Latest)
	return m, nil
}

// --- Source Picker ---

type sourcePickerScope struct {
	Search      string // "owner/name" — the CLI --repo scope and initial search text
	Remote      string // git remote URL — drives backend detection and CreateOptions.Remote
	Source      string // local repo checkout dir; the CLI cwd and CreateOptions.Source (file-copy source)
	ExactSearch bool   // use Search for detail instead of trusting the item's Fields.repo
}

// pendingSourceForm holds one source selection while the user confirms its
// repository, editable name, and agent in NewSessionForm.
type pendingSourceForm struct {
	result          sourcepicker.Result
	remote          string
	scope           string
	useSelectedRepo bool
}

// resolveSourceID returns the registered source to open: an explicit args[0]
// when given, otherwise the sole registered source.
func (m Model) resolveSourceID(args []string) (string, error) {
	if len(args) > 0 && args[0] != "" {
		id := args[0]
		if m.sourceRegistry != nil && !slices.Contains(m.sourceRegistry.IDs(), id) {
			return "", fmt.Errorf("source %q is not configured", id)
		}
		return id, nil
	}
	if m.sourceRegistry == nil {
		return "", fmt.Errorf("no sources are configured")
	}
	ids := m.sourceRegistry.IDs()
	switch len(ids) {
	case 0:
		return "", fmt.Errorf("no sources are configured")
	case 1:
		return ids[0], nil
	default:
		return "", fmt.Errorf("multiple sources configured: use :Sources <id>")
	}
}

// openSourcePicker opens the tabbed source picker with the sources registered
// for the repo's detected forge backend, starting on the tab identified by
// sourceID. The picker handles lazy initialization, loading, and error states
// internally.
func (m Model) openSourcePicker(sourceID string, scope sourcePickerScope) (tea.Model, tea.Cmd) {
	if m.sourceRegistry == nil {
		m.notifyErrorf("no sources are configured")
		return m, nil
	}

	backend := m.detectSourceBackend(scope.Remote)

	if _, _, ok := m.sourceRegistry.Get(sourceID, backend); !ok {
		m.notifyErrorf("source %q is not available for %s repositories", sourceID, backend)
		return m, nil
	}

	entries := m.sourceRegistry.All(backend)
	tabs := make([]sourcepicker.TabSource, len(entries))
	for i, entry := range entries {
		tabs[i] = sourcepicker.TabSource{
			ID:             entry.ID,
			Source:         entry.Source,
			Templates:      entry.Templates,
			SearchDisabled: m.isViewSource(entry.ID),
			Manifest: sources.Manifest{
				ID:          entry.ID,
				DisplayName: entry.DisplayName,
			},
		}
	}

	m.pendingSourceScope = scope
	picker := sourcepicker.New(tabs, sourceID, scope.Search, scope.Source, m.width, m.height)
	m.state = stateSourcePicker
	m.modals.SourcePicker = &picker
	return m, picker.Init()
}

// isViewSource reports whether id identifies a configured saved-search view.
func (m Model) isViewSource(id string) bool {
	if m.cfg == nil {
		return false
	}
	for _, view := range m.cfg.Sources.Views {
		if view.Name == id {
			return true
		}
	}
	return false
}

// isExternalSource reports whether id identifies a configured subprocess source.
func (m Model) isExternalSource(id string) bool {
	if m.cfg == nil {
		return false
	}
	for _, external := range m.cfg.Sources.External {
		if external.Name == id {
			return true
		}
	}
	return false
}

// detectSourceBackend resolves the forge backend from the remote's host and
// any configured sources.hosts overrides.
func (m Model) detectSourceBackend(remote string) sources.Backend {
	host := git.ExtractHost(remote)
	var overrides map[string]sources.Backend
	if len(m.cfg.Sources.Hosts) > 0 {
		overrides = make(map[string]sources.Backend, len(m.cfg.Sources.Hosts))
		for h, b := range m.cfg.Sources.Hosts {
			backend, err := sources.ParseBackend(b)
			if err != nil {
				continue // values are validated at config load; skip defensively
			}
			overrides[strings.ToLower(h)] = backend
		}
	}
	return sources.DetectBackend(host, overrides)
}

// forwardSourcePickerMsg forwards a source search/detail/spinner message
// to the active SourcePicker.
func (m Model) forwardSourcePickerMsg(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.modals.SourcePicker == nil {
		return m, nil
	}
	picker, cmd := m.modals.SourcePicker.Update(msg)
	m.modals.SourcePicker = &picker
	return m, cmd
}

// handleSourcePickerKey routes key events to the active SourcePicker.
// On cancellation the picker closes with no side effects; on selection the
// picker closes and a session is created from the selected item's rendered
// templates.
func (m Model) handleSourcePickerKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.modals.SourcePicker == nil {
		m.state = stateNormal
		return m, nil
	}

	picker, cmd := m.modals.SourcePicker.Update(msg)
	m.modals.SourcePicker = &picker

	if picker.Cancelled() {
		m.modals.SourcePicker = nil
		m.state = stateNormal
		return m, nil
	}

	if results, ok := picker.Selected(); ok {
		return m.handleSourceSelection(results)
	}

	return m, cmd
}

// handleSourceSelection keeps built-in and scoped-view selections on the
// instant batch path. Global views and external sources require repository
// confirmation and accept one marked result at a time, preserving all marks
// when the user must narrow a multi-selection.
func (m Model) handleSourceSelection(results []sourcepicker.Result) (tea.Model, tea.Cmd) {
	if m.modals.BgStreamDone != nil {
		m.modals.SourcePicker = nil
		m.state = stateNormal
		m.notifyErrorf("already running in background: %s", m.modals.BgStreamTitle)
		return m, nil
	}

	hasGlobal := false
	hasExternal := false
	for _, result := range results {
		hasGlobal = hasGlobal || m.isGlobalViewResult(result)
		hasExternal = hasExternal || m.isExternalSource(result.SourceID)
	}
	if (hasGlobal || hasExternal) && len(results) != 1 {
		if m.modals.SourcePicker != nil {
			m.modals.SourcePicker.Resume()
			m.state = stateSourcePicker
		}
		if hasExternal {
			m.notifyErrorf("external sources create one session at a time; unmark all but one result")
		} else {
			m.notifyErrorf("global source views create one session at a time; unmark all but one result")
		}
		return m, nil
	}
	if hasExternal {
		return m.openExternalSourceSessionForm(results[0])
	}
	if hasGlobal {
		return m.openSourceSessionForm(results[0])
	}

	m.modals.SourcePicker = nil
	return m, m.startSourceCreate(results, m.pendingSourceScope)
}

// isGlobalViewResult reports whether a result belongs to a configured view
// whose empty scope permits cross-repository results.
func (m Model) isGlobalViewResult(result sourcepicker.Result) bool {
	if m.cfg == nil {
		return false
	}
	for _, view := range m.cfg.Sources.Views {
		if view.Name == result.SourceID {
			return strings.TrimSpace(view.Scope) == ""
		}
	}
	return false
}

// openSourceSessionForm opens NewSessionForm for one global-view result. All
// work here is in-memory; detail fetching remains in the background create
// command after form submission.
func (m Model) openSourceSessionForm(result sourcepicker.Result) (tea.Model, tea.Cmd) {
	remote, ok := m.resolveItemRemote(result.Item)
	if !ok {
		if m.modals.SourcePicker != nil {
			m.modals.SourcePicker.Resume()
			m.state = stateSourcePicker
		}
		m.notifyErrorf("source result does not identify a valid repository remote")
		return m, nil
	}

	rendered, err := sources.RenderSessionTemplates(result.Templates, result.Item, sources.Detail{})
	if err != nil {
		if m.modals.SourcePicker != nil {
			m.modals.SourcePicker.Resume()
			m.state = stateSourcePicker
		}
		m.notifyErrorf("source templates: %v", err)
		return m, nil
	}

	itemScope := sourceItemScope(result.Item, m.pendingSourceScope.Search)
	repos := m.sourceFormRepos()
	if !containsRepoRemote(repos, remote) {
		repos = append([]workspace.DiscoveredRepo{{Name: itemScope, Remote: remote}}, repos...)
	}

	existingNames, agentKeys, defaultAgent := m.sourceFormDefaults()
	form := NewNewSessionForm(repos, remote, rendered.Name, existingNames, agentKeys)
	form.selectAgent(defaultAgent)
	if m.modals.SourcePicker != nil {
		m.modals.SourcePicker.Resume()
	}
	m.pendingSourceForm = &pendingSourceForm{result: result, remote: remote, scope: itemScope}
	m.modals.NewSession = form
	m.state = stateCreatingSession
	return m, form.Init()
}

// openExternalSourceSessionForm opens NewSessionForm over only discovered
// repositories. Item-provided repo/URI values never create or preselect a
// synthetic repository for external sources.
func (m Model) openExternalSourceSessionForm(result sourcepicker.Result) (tea.Model, tea.Cmd) {
	rendered, err := sources.RenderSessionTemplates(result.Templates, result.Item, sources.Detail{})
	if err != nil {
		return m.resumeSourcePickerWithError("source templates: %v", err)
	}

	repos := m.sourceFormRepos()
	if len(repos) == 0 {
		return m.resumeSourcePickerWithError("no repositories are available to select for external source %q", result.SourceID)
	}

	preselectedRemote := m.pendingSourceScope.Remote
	if !containsRepoRemote(repos, preselectedRemote) {
		preselectedRemote = ""
	}
	existingNames, agentKeys, defaultAgent := m.sourceFormDefaults()
	form := NewNewSessionForm(repos, preselectedRemote, rendered.Name, existingNames, agentKeys)
	form.selectAgent(defaultAgent)
	if m.modals.SourcePicker != nil {
		m.modals.SourcePicker.Resume()
	}
	m.pendingSourceForm = &pendingSourceForm{result: result, useSelectedRepo: true}
	m.modals.NewSession = form
	m.state = stateCreatingSession
	return m, form.Init()
}

func (m Model) resumeSourcePickerWithError(format string, args ...any) (tea.Model, tea.Cmd) {
	if m.modals.SourcePicker != nil {
		m.modals.SourcePicker.Resume()
		m.state = stateSourcePicker
	}
	m.notifyErrorf(format, args...)
	return m, nil
}

// resolveItemRemote resolves an item's host-qualified owner/repo to a clone
// remote. An exact known host+owner/repo match preserves its SSH/HTTPS form;
// otherwise a deterministic HTTPS remote is constructed.
func (m Model) resolveItemRemote(item sources.Item) (string, bool) {
	repoScope := sourceItemScope(item, "")
	if !validSourceRepoScope(repoScope) {
		return "", false
	}

	rawURI := strings.TrimSpace(item.URI)
	parsed, err := url.ParseRequestURI(rawURI)
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" || parsed.User != nil {
		return "", false
	}
	host := git.ExtractHost(rawURI)
	if !validSourceHost(host) || !strings.EqualFold(host, parsed.Hostname()) {
		return "", false
	}

	for _, repo := range m.knownSourceRepos() {
		knownHost := git.ExtractHost(repo.Remote)
		owner, name := git.ExtractOwnerRepo(repo.Remote)
		if strings.EqualFold(knownHost, host) && strings.EqualFold(owner+"/"+name, repoScope) {
			return repo.Remote, true
		}
	}

	return (&url.URL{Scheme: "https", Host: host, Path: "/" + repoScope}).String(), true
}

// sourceItemScope prefers a result's explicit repo field and falls back to the
// picker scope used for built-in and repo-scoped sources.
func sourceItemScope(item sources.Item, fallback string) string {
	if repo, ok := item.Fields["repo"].(string); ok && strings.TrimSpace(repo) != "" {
		return strings.TrimSpace(repo)
	}
	return fallback
}

func (m Model) knownSourceRepos() []workspace.DiscoveredRepo {
	if m.sessionsView == nil {
		return nil
	}
	repos := append([]workspace.DiscoveredRepo(nil), m.sessionsView.DiscoveredRepos()...)
	for _, sess := range m.sessionsView.AllSessions() {
		repos = append(repos, workspace.DiscoveredRepo{Path: sess.Path, Name: sess.Name, Remote: sess.Remote})
	}
	return repos
}

func (m Model) sourceFormRepos() []workspace.DiscoveredRepo {
	if m.sessionsView == nil {
		return nil
	}
	return append([]workspace.DiscoveredRepo(nil), m.sessionsView.DiscoveredRepos()...)
}

func (m Model) sourceFormDefaults() (map[string]bool, []string, string) {
	existingNames := map[string]bool{}
	if m.sessionsView != nil {
		for _, sess := range m.sessionsView.AllSessions() {
			existingNames[sess.Name] = true
		}
	}

	if m.cfg == nil {
		return existingNames, nil, ""
	}
	defaultAgent := m.defaultAgentKey()
	if !m.cfg.Agents.AgentSelector || len(m.cfg.Agents.Profiles) <= 1 {
		return existingNames, nil, defaultAgent
	}
	keys := make([]string, 0, len(m.cfg.Agents.Profiles))
	for key := range m.cfg.Agents.Profiles {
		if key != defaultAgent {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return existingNames, append([]string{defaultAgent}, keys...), defaultAgent
}

func containsRepoRemote(repos []workspace.DiscoveredRepo, remote string) bool {
	for _, repo := range repos {
		if repo.Remote == remote {
			return true
		}
	}
	return false
}

func validSourceRepoScope(scope string) bool {
	parts := strings.Split(scope, "/")
	return len(parts) == 2 && validSourceRepoPart(parts[0]) && validSourceRepoPart(parts[1])
}

func validSourceRepoPart(part string) bool {
	if part == "" || part == "." || part == ".." {
		return false
	}
	for _, r := range part {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '.' && r != '_' && r != '-' {
			return false
		}
	}
	return true
}

func validSourceHost(host string) bool {
	if host == "" || strings.TrimSpace(host) != host || strings.HasPrefix(host, ".") || strings.HasSuffix(host, ".") {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return false
		}
		for _, r := range label {
			if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-' {
				return false
			}
		}
	}
	return true
}

// fetchSourceDetail returns the item's detail for template rendering. Detail
// is optional: sources without the capability fall back to the item's inline
// "body" field, and fetch failures degrade the same way (with a log) rather
// than blocking session creation.
func fetchSourceDetail(ctx context.Context, result sourcepicker.Result, scope, dir string) sources.Detail {
	if !result.Manifest.Capabilities.FetchDetail || result.Source == nil {
		return detailFromBodyField(result.Item)
	}
	fetched, err := result.Source.FetchDetail(ctx, sources.FetchDetailParams{
		ID:    result.Item.ID,
		Scope: scope,
		URI:   result.Item.URI,
		Dir:   dir,
	})
	if err != nil {
		log.Warn().Err(err).Str("source", result.SourceID).Str("item", result.Item.ID).
			Msg("source picker: fetch detail failed; creating session without detail")
		return detailFromBodyField(result.Item)
	}
	return fetched
}

// detailFromBodyField synthesizes a markdown detail from an item's "body"
// field when present, so .Detail templates work for sources that carry the
// body inline instead of via FetchDetail.
func detailFromBodyField(item sources.Item) sources.Detail {
	body, ok := item.Fields["body"].(string)
	if !ok || body == "" {
		return sources.Detail{}
	}
	return sources.Detail{Markdown: &sources.MarkdownDetail{Content: body}}
}

// startSourceCreate starts one background stream that creates a session
// for every result in order. Background stream output is discarded (see
// listenForBgStreamComplete), so the user-visible record of per-item
// failures is the summarized completion error plus the log; remaining
// items still spawn.
func (m Model) startSourceCreate(results []sourcepicker.Result, scope sourcePickerScope) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithCancel(context.Background())
		output := make(chan string, 100)
		done := make(chan error, 1)

		// Populated with the first created session before done fires, for
		// post-completion selection (same contract as CreateExecutor).
		firstID := new(string)
		firstName := new(string)

		go func() {
			defer close(output)
			defer close(done)
			done <- m.createSourceSessions(ctx, results, scope, output, firstID, firstName)
		}()

		title := "Creating session..."
		if len(results) > 1 {
			title = fmt.Sprintf("Creating %d sessions...", len(results))
		}
		return bgStreamStartedMsg{
			title:  title,
			output: output,
			done:   done,
			cancel: cancel,
			result: streamResult{
				sessionID:   firstID,
				sessionName: firstName,
			},
		}
	}
}

// startSourceFormCreate starts the existing background source-create path
// with the repository, edited name, and agent confirmed by NewSessionForm.
func (m Model) startSourceFormCreate(pending pendingSourceForm, form NewSessionFormResult) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithCancel(context.Background())
		output := make(chan string, 100)
		done := make(chan error, 1)
		resultID := new(string)
		resultName := new(string)

		remote := form.Repo.Remote
		detailScope := pending.scope
		exactSearch := false
		if pending.useSelectedRepo {
			owner, repo := git.ExtractOwnerRepo(remote)
			if owner != "" && repo != "" {
				detailScope = owner + "/" + repo
			}
			exactSearch = true
		} else if remote == "" {
			remote = pending.remote
		}
		scope := sourcePickerScope{
			Search: detailScope, Remote: remote, Source: form.Repo.Path, ExactSearch: exactSearch,
		}
		go func() {
			defer close(output)
			defer close(done)
			id, name, err := m.createSourceSessionWithForm(
				ctx, pending.result, scope, output, form.SessionName, form.AgentKey,
			)
			if err == nil {
				*resultID = id
				*resultName = name
			} else {
				log.Warn().Err(err).Str("source", pending.result.SourceID).Str("item", pending.result.Item.ID).
					Msg("source picker: session create failed")
			}
			done <- err
		}()

		return bgStreamStartedMsg{
			title:  "Creating session...",
			output: output,
			done:   done,
			cancel: cancel,
			result: streamResult{sessionID: resultID, sessionName: resultName},
		}
	}
}

// createSourceSessions renders and creates one session per result, writing
// progress lines to out (discarded while the stream runs in background —
// the summary error and the log are the user-visible record). A failed
// item is logged, then the loop moves on; only cancellation aborts the
// batch. The returned error is the lone item's error for a single-item
// batch, or a partial-failure summary otherwise. The first created
// session is written to firstID/firstName for post-completion selection.
func (m Model) createSourceSessions(ctx context.Context, results []sourcepicker.Result, scope sourcePickerScope, out chan<- string, firstID, firstName *string) error {
	var failedIDs []string
	var firstErr error
	for i, result := range results {
		if err := ctx.Err(); err != nil {
			return err
		}
		if len(results) > 1 {
			streamLine(ctx, out, fmt.Sprintf("[%d/%d] %s", i+1, len(results), result.Item.Title))
		}
		id, name, err := m.createSourceSession(ctx, result, scope, out)
		if err == nil {
			if *firstID == "" {
				*firstID = id
				*firstName = name
			}
			continue
		}
		if ctx.Err() != nil {
			return err
		}
		failedIDs = append(failedIDs, result.Item.ID)
		if firstErr == nil {
			firstErr = err
		}
		streamLine(ctx, out, fmt.Sprintf("[%d/%d] failed: %v", i+1, len(results), err))
		log.Warn().Err(err).Str("source", result.SourceID).Str("item", result.Item.ID).
			Msg("source picker: session create failed")
	}

	switch {
	case len(failedIDs) == 0:
		return nil
	case len(results) == 1:
		return firstErr
	default:
		// Name the failed items (IDs are short — issue/PR numbers) so the
		// completion notification tells the user what to retry; full causes
		// stay in the log.
		return fmt.Errorf("%d of %d sessions failed (#%s)", len(failedIDs), len(results), strings.Join(failedIDs, ", #"))
	}
}

// createSourceSession fetches the item's detail (best-effort), renders the
// source's session templates, and creates the session, forwarding its
// creation output onto the shared stream. It returns the created session's
// ID and name.
func (m Model) createSourceSession(ctx context.Context, result sourcepicker.Result, scope sourcePickerScope, out chan<- string) (id, name string, err error) {
	return m.createSourceSessionWithForm(ctx, result, scope, out, "", "")
}

func (m Model) createSourceSessionWithForm(
	ctx context.Context,
	result sourcepicker.Result,
	scope sourcePickerScope,
	out chan<- string,
	nameOverride, agentKey string,
) (id, name string, err error) {
	detailScope := scope.Search
	if !scope.ExactSearch {
		detailScope = sourceItemScope(result.Item, detailScope)
	}
	detail := fetchSourceDetail(ctx, result, detailScope, scope.Source)

	rendered, err := sources.RenderSessionTemplates(result.Templates, result.Item, detail)
	if err != nil {
		return "", "", err
	}
	if nameOverride != "" {
		rendered.Name = nameOverride
	}

	exec := m.cmdService.NewCreateExecutor(hive.CreateOptions{
		Name:          rendered.Name,
		Prompt:        rendered.Prompt,
		Remote:        scope.Remote,
		Source:        scope.Source,
		UseBatchSpawn: true,
		Background:    true,
		Tags:          rendered.Tags,
		AgentKey:      agentKey,
	})

	output, done, cancel := exec.Execute(ctx)
	defer cancel()
	for line := range output {
		streamLine(ctx, out, line)
	}
	if err := <-done; err != nil {
		return "", "", err
	}
	return exec.ResultSessionID, exec.ResultSessionName, nil
}

// streamLine sends a line to a stream output channel without blocking past
// cancellation.
func streamLine(ctx context.Context, out chan<- string, line string) {
	select {
	case out <- line:
	case <-ctx.Done():
	}
}

// --- Repo Picker ---

func (m Model) handleRepoPickerKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.modals.RepoPicker == nil {
		m.state = stateNormal
		return m, nil
	}

	m.modals.RepoPicker, _ = m.modals.RepoPicker.Update(msg)

	if m.modals.RepoPicker.Cancelled() {
		m.modals.RepoPicker = nil
		m.modals.DocsRepoEntries = nil
		m.state = stateNormal
		return m, nil
	}

	if selected := m.modals.RepoPicker.Selected(); selected != "" {
		m.modals.RepoPicker = nil
		m.state = stateNormal

		if len(m.modals.DocsRepoEntries) > 0 {
			entries := m.modals.DocsRepoEntries
			m.modals.DocsRepoEntries = nil
			if m.reviewView != nil {
				for _, e := range entries {
					if e.key == selected {
						m.reviewView.SetRepoKey(e.key)
						return m, m.reviewView.SetContextDir(e.contextDir)
					}
				}
			}
			return m, nil
		}

		m.modals.DocsRepoEntries = nil
		if m.tasksView != nil {
			return m, m.tasksView.SetRepoKey(selected)
		}
		return m, nil
	}

	return m, nil
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
			m.publishNotificationf(notify.LevelInfo, "no URI on this todo")
			return m, nil
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
				path := m.resolveReviewDocPath(item)
				return m, m.reviewView.OpenDocumentByPath(path)
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
	case "r":
		if err := m.modals.TodoPanel.ReopenCurrent(); err != nil {
			return m, m.notifyError("reopen todo: %v", err)
		}
	}
	return m, nil
}

// resolveReviewDocPath resolves a review todo's URI value to an absolute file path
// by looking up the todo's session to determine the correct context directory.
// This handles cross-repo todos where the document lives in a different repo's context.
func (m Model) resolveReviewDocPath(item *todo.Todo) string {
	if item.SessionID == "" {
		return item.URI.Value()
	}

	// Find the session to get its remote URL
	for _, s := range m.sessionsView.AllSessions() {
		if s.ID == item.SessionID {
			return resolveReviewURI(item.URI.Value(), s.Remote, m.cfg)
		}
	}

	return item.URI.Value()
}

// resolveReviewURI resolves a review URI value to an absolute file path using
// a session's remote URL to determine the correct context directory.
func resolveReviewURI(uriValue, remote string, cfg *config.Config) string {
	if remote == "" {
		return uriValue
	}
	owner, repo := git.ExtractOwnerRepo(remote)
	if owner == "" || repo == "" {
		return uriValue
	}
	contextDir := cfg.RepoContextDir(owner, repo)
	rel := strings.TrimPrefix(uriValue, ".hive/")
	return filepath.Join(contextDir, rel)
}

// completeTodosMatchingRef completes all open review todos whose URI value matches any of the given paths,
// then returns updated todo counts directly (avoiding a double loadTodoCounts invocation).
func (m Model) completeTodosMatchingRef(paths ...string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		items, err := m.todoService.List(ctx, todo.ListFilter{})
		if err != nil {
			log.Warn().Err(err).Msg("failed to list todos for auto-complete")
			return todoAutoCompleteResultMsg{failed: -1}
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
			return todoAutoCompleteResultMsg{failed: failed}
		}
		open, err := m.todoService.CountOpen(ctx)
		if err != nil {
			log.Warn().Err(err).Msg("failed to load todo open count after auto-complete")
			return todoAutoCompleteResultMsg{failed: failed}
		}
		return todoAutoCompleteResultMsg{pendingCount: pending, openCount: open, failed: failed}
	}
}

// --- Input ---

func (m Model) handleKeyMsg(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	return m.handleKey(msg)
}

func (m Model) handleSpinnerTick(msg spinner.TickMsg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	// Advance the main spinner and re-schedule only while the loading modal is
	// visible. Returning nil here stops the idle tick loop; it is restarted by
	// the code that transitions into stateLoading.
	var cmd tea.Cmd
	m.spinner, cmd = m.spinner.Update(msg)
	if cmd != nil && m.state == stateLoading {
		cmds = append(cmds, cmd)
	}

	// Drive the source picker's internal spinner while the picker is active.
	if m.state == stateSourcePicker && m.modals.SourcePicker != nil {
		picker, pickerCmd := m.modals.SourcePicker.Update(msg)
		m.modals.SourcePicker = &picker
		if pickerCmd != nil {
			cmds = append(cmds, pickerCmd)
		}
	}

	// Drive the output modal's own spinner while streaming.
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

	// Forward to tasks view
	if m.activeView == ViewTasks && m.tasksView != nil {
		if cmd := m.tasksView.Update(msg); cmd != nil {
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

const doubleClickWindow = 300 * time.Millisecond

// handleMouseClick routes left-button clicks to the active view or tab bar.
// Two clicks within doubleClickWindow at the same cell synthesize an Enter key press.
func (m Model) handleMouseClick(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	if msg.Button != tea.MouseLeft || m.isModalActive() {
		return m, nil
	}

	// Tab bar is at terminal row 1 (topDivider=0, header=1, headerDivider=2).
	if msg.Y == 1 {
		return m.handleTabClick(msg.X)
	}

	const tabChrome = 3
	contentY := msg.Y - tabChrome
	if contentY < 0 {
		return m, nil
	}

	// Detect double-click: same cell within the window.
	now := time.Now()
	isDouble := msg.X == m.lastClickX && msg.Y == m.lastClickY &&
		now.Sub(m.lastClickTime) <= doubleClickWindow
	m.lastClickX = msg.X
	m.lastClickY = msg.Y
	m.lastClickTime = now

	if isDouble {
		return m.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEnter})
	}

	var cmd tea.Cmd
	switch m.activeView {
	case ViewSessions:
		if m.sessionsView != nil {
			cmd = m.sessionsView.SelectAtRow(msg.X, contentY)
		}
	case ViewTasks:
		if m.tasksView != nil {
			cmd = m.tasksView.SelectAtRow(msg.X, contentY)
		}
	case ViewMessages:
		if m.msgView != nil {
			cmd = m.msgView.SelectAtRow(msg.X, contentY)
		}
	case ViewReview:
		if m.reviewView != nil {
			cmd = m.reviewView.SelectAtRow(msg.X, contentY)
		}
	case ViewStore:
		if m.kvView != nil {
			cmd = m.kvView.SelectAtRow(msg.X, contentY)
		}
	}
	return m, cmd
}

// switchToView activates the given view, updating all per-view active flags and
// firing any data-load commands required on first entry. This is the single
// source of truth for view switching used by both keyboard (tab key) and mouse
// (tab bar click).
func (m Model) switchToView(view ViewType) (tea.Model, tea.Cmd) {
	m.lastClickTime = time.Time{} // reset double-click state across view changes
	if m.activeView == ViewStore && view != ViewStore && m.kvView.IsFiltering() {
		m.kvView.CancelFilter() // don't leave stale KV filter state behind
	}
	m.activeView = view
	m.handler.SetActiveView(view)
	m.sessionsView.SetActive(view == ViewSessions)
	if m.msgView != nil {
		m.msgView.SetActive(view == ViewMessages)
	}
	if m.tasksView != nil {
		m.tasksView.SetActive(view == ViewTasks)
	}

	switch view {
	case ViewStore:
		return m, m.loadKVKeys()
	case ViewTasks:
		if cmd := m.syncTasksRepoFromSessions(); cmd != nil {
			return m, cmd
		}
		return m, func() tea.Msg { return tasks.RefreshTasksMsg{} }
	case ViewReview:
		if cmd := m.syncDocsRepoFromSessions(); cmd != nil {
			return m, cmd
		}
	case ViewSessions, ViewMessages:
		// No data load needed on switch.
	}
	return m, nil
}

// handleTabClick switches the active view based on which tab label was clicked at column x.
// Tab labels start at column 1 (one-space left margin) and are separated by " | " (3 cols).
func (m Model) handleTabClick(x int) (tea.Model, tea.Cmd) {
	showStoreTab := m.kvStore != nil && m.cfg.TUI.Store

	type tabEntry struct {
		view  ViewType
		label string
	}
	tabs := []tabEntry{{ViewSessions, "Sessions"}}
	if m.tasksView != nil {
		tabs = append(tabs, tabEntry{ViewTasks, "Tasks"})
	}
	if m.reviewView != nil {
		tabs = append(tabs, tabEntry{ViewReview, "Docs"})
	}
	tabs = append(tabs, tabEntry{ViewMessages, "Messages"})
	if showStoreTab || m.activeView == ViewStore {
		tabs = append(tabs, tabEntry{ViewStore, "Store"})
	}

	const (
		leftMargin = 1
		sepWidth   = 3 // " | "
	)
	pos := leftMargin
	for i, t := range tabs {
		w := len(t.label) // styles use no padding/borders, so visual width == len
		if x >= pos && x < pos+w {
			return m.switchToView(t.view)
		}
		pos += w
		if i < len(tabs)-1 {
			pos += sepWidth
		}
	}
	return m, nil
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
		m.notifyErrorf("no local repository found for %s", name)
		return m, nil
	}

	shellCmd, err := m.renderer.Render(
		`{{ hiveTmux }} {{ .Name | shq }} {{ .Path | shq }}`,
		struct{ Name, Path string }{Name: name, Path: repoPath},
	)
	if err != nil {
		m.notifyErrorf("template error: %v", err)
		return m, nil
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

// handleShowRiskLoading reveals the loading spinner after the 250ms delay, but
// only if the risk check is still in progress (state is still normal and the
// pending action matches). This prevents a flash on fast repos.
func (m Model) handleShowRiskLoading(msg showRiskLoadingMsg) (tea.Model, tea.Cmd) {
	if m.state != stateNormal || m.modals.Pending.SessionID != msg.sessionID {
		return m, nil
	}
	m.state = stateLoading
	m.loadingMessage = "Checking for unsaved work..."
	return m, m.spinner.Tick
}

// handleSessionRiskChecked continues the delete/recycle dispatch after the async
// git risk check completes. If the session has at-risk data, a dangerous
// confirmation modal is shown; otherwise normal dispatch resumes.
func (m Model) handleSessionRiskChecked(msg sessionRiskCheckedMsg) (tea.Model, tea.Cmd) {
	action := msg.action

	if msg.risk.HasRisk() {
		var lines []string
		lines = append(lines, "This session has work that will be permanently lost:")
		lines = append(lines, "")
		if msg.risk.UncommittedChanges {
			lines = append(lines, "  • Uncommitted changes")
		}
		if msg.risk.UnpushedCommits {
			lines = append(lines, "  • Unpushed commits")
		}

		title := "Delete Session?"
		requireText := "delete"
		if action.Type == act.TypeRecycle {
			title = "Recycle Session?"
			requireText = "recycle"
		}

		m.state = stateConfirming
		m.modals.Pending = action
		m.modals.Confirm = NewDangerousModal(title, strings.Join(lines, "\n"), requireText)
		return m, nil
	}

	// No risk — continue with the normal dispatch path.
	if action.NeedsConfirm() {
		m.state = stateConfirming
		m.modals.Pending = action
		m.modals.Confirm = NewModal("Confirm", action.Confirm)
		return m, nil
	}

	if action.Type == act.TypeRecycle {
		m.state = stateNormal
		m.modals.Pending = Action{}
		return m, m.startRecycle(action.SessionID)
	}

	m.state = stateNormal
	m.modals.Pending = action
	if !action.Silent {
		m.state = stateLoading
		m.loadingMessage = "Processing..."
		return m, tea.Batch(m.spinner.Tick, m.executeAction(action))
	}
	return m, m.executeAction(action)
}

// handleConfirmModalKey handles keys when confirmation modal is shown.
func (m Model) handleConfirmModalKey(keyStr string) (tea.Model, tea.Cmd) {
	if m.modals.Confirm.IsTextInput() {
		return m.handleDangerousConfirmKey(keyStr)
	}

	switch keyStr {
	case keyEnter:
		confirmed := m.modals.Confirm.ConfirmSelected()
		m.modals.DismissConfirm()
		if confirmed {
			action := m.modals.Pending
			if action.Type == act.TypeRecycle {
				m.state = stateNormal
				return m, m.startRecycle(action.SessionID)
			}
			if action.Type == act.TypeDeleteRecycledBatch {
				m.state = stateNormal
				recycled := m.modals.PendingRecycledSessions
				m.modals.Pending = Action{}
				m.modals.PendingRecycledSessions = nil
				return m, m.deleteRecycledSessionsBatch(recycled)
			}
			if action.Type == act.TypeTasksSetCancelled {
				m.state = stateNormal
				m.modals.Pending = Action{}
				status := hc.StatusCancelled
				id := action.SessionID
				svc := m.tasksView.Svc()
				return m, func() tea.Msg {
					_, err := svc.UpdateItem(context.Background(), id, hc.ItemUpdate{Status: &status})
					return tasks.TaskActionCompleteMsg{Err: err}
				}
			}
			if action.Type == act.TypeTasksDelete {
				m.state = stateNormal
				m.modals.Pending = Action{}
				id := action.SessionID
				svc := m.tasksView.Svc()
				return m, func() tea.Msg {
					err := svc.DeleteItem(context.Background(), id)
					return tasks.TaskActionCompleteMsg{Err: err}
				}
			}
			if action.Type == act.TypeTasksPrune {
				m.state = stateNormal
				m.modals.Pending = Action{}
				svc := m.tasksView.Svc()
				repoKey := ""
				if m.tasksView != nil {
					repoKey = m.tasksView.RepoKey()
				}
				return m, func() tea.Msg {
					_, err := svc.Prune(context.Background(), hc.PruneOpts{
						OlderThan: 24 * time.Hour,
						Statuses:  []hc.Status{hc.StatusDone, hc.StatusCancelled},
						RepoKey:   repoKey,
					})
					return tasks.TaskActionCompleteMsg{Err: err}
				}
			}
			m.state = stateNormal
			if !action.Silent {
				m.state = stateLoading
				m.loadingMessage = "Processing..."
				return m, tea.Batch(m.spinner.Tick, m.executeAction(action))
			}
			return m, m.executeAction(action)
		}
		m.state = stateNormal
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

// handleDangerousConfirmKey handles keys for text-input confirmation modals
// (dangerous delete/recycle). The user must type the required word before enter
// is accepted; all other printable characters are fed into the text buffer.
func (m Model) handleDangerousConfirmKey(keyStr string) (tea.Model, tea.Cmd) {
	switch keyStr {
	case keyEnter:
		if !m.modals.Confirm.ConfirmSelected() {
			// Text doesn't match the required word yet — do nothing.
			return m, nil
		}
		m.modals.DismissConfirm()
		action := m.modals.Pending
		m.modals.Pending = Action{}
		m.modals.PendingRecycledSessions = nil
		if action.Type == act.TypeRecycle {
			m.state = stateNormal
			return m, m.startRecycle(action.SessionID)
		}
		m.state = stateNormal
		if !action.Silent {
			m.state = stateLoading
			m.loadingMessage = "Processing..."
			return m, tea.Batch(m.spinner.Tick, m.executeAction(action))
		}
		return m, m.executeAction(action)
	case "esc":
		m.state = stateNormal
		m.modals.DismissConfirm()
		m.modals.Pending = Action{}
		m.modals.PendingRecycledSessions = nil
		return m, nil
	case "backspace":
		m.modals.Confirm.DeleteChar()
		return m, nil
	default:
		if len(keyStr) == 1 {
			m.modals.Confirm.AddChar(keyStr)
		}
		return m, nil
	}
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
