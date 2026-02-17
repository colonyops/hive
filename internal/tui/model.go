package tui

import (
	"context"
	"os/exec"
	"strings"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/rs/zerolog/log"

	act "github.com/colonyops/hive/internal/core/action"
	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/internal/core/eventbus"
	"github.com/colonyops/hive/internal/core/git"
	corekv "github.com/colonyops/hive/internal/core/kv"
	"github.com/colonyops/hive/internal/core/notify"
	"github.com/colonyops/hive/internal/core/session"
	"github.com/colonyops/hive/internal/core/styles"
	"github.com/colonyops/hive/internal/core/terminal"

	"github.com/colonyops/hive/internal/data/db"
	"github.com/colonyops/hive/internal/data/stores"
	"github.com/colonyops/hive/internal/hive"
	"github.com/colonyops/hive/internal/hive/plugins"
	"github.com/colonyops/hive/internal/tui/command"
	"github.com/colonyops/hive/internal/tui/components"
	"github.com/colonyops/hive/internal/tui/components/form"
	tuinotify "github.com/colonyops/hive/internal/tui/notify"
	"github.com/colonyops/hive/internal/tui/views/messages"
	"github.com/colonyops/hive/internal/tui/views/review"
	"github.com/colonyops/hive/internal/tui/views/sessions"

	"github.com/colonyops/hive/pkg/tmpl"
)

// UIState represents the current state of the TUI.
type UIState int

const (
	stateNormal UIState = iota
	stateConfirming
	stateLoading
	stateRunningRecycle
	stateCreatingSession
	stateCommandPalette
	stateShowingHelp
	stateShowingNotifications
	stateRenaming
	stateFormInput
)

// Key constants for event handling.
const (
	keyEnter = "enter"
	keyCtrlC = "ctrl+c"
)

// Options configures the TUI behavior.
type Options struct {
	LocalRemote     string               // Remote URL of current directory (empty if not in git repo)
	MsgStore        *hive.MessageService // Message service for pub/sub events (optional)
	Bus             *eventbus.EventBus   // Event bus for cross-component communication
	TerminalManager *terminal.Manager    // Terminal integration manager (optional)
	PluginManager   *plugins.Manager     // Plugin manager (optional)
	DB              *db.DB               // Database connection for stores
	KVStore         corekv.KV            // Persistent KV store (optional)
	Renderer        *tmpl.Renderer       // Template renderer for shell commands
	Warnings        []string             // Startup warnings to display as toasts
}

// PendingCreate holds data for a session to create after TUI exits.
type PendingCreate struct {
	Remote string
	Name   string
}

// Model is the main Bubble Tea model for the TUI.
type Model struct {
	cfg            *config.Config
	service        *hive.SessionService
	cmdService     *command.Service
	handler        *KeybindingResolver
	state          UIState
	modal          Modal
	pending        Action
	width          int
	height         int
	spinner        spinner.Model
	loadingMessage string
	quitting       bool

	// Sessions view (sub-model) — owns all session-related state
	sessionsView *sessions.View

	// Recycle streaming state (remains on Model because recycle modal is Model-level)
	outputModal   OutputModal
	recycleOutput <-chan string
	recycleDone   <-chan error
	recycleCancel context.CancelFunc

	// Layout
	activeView ViewType

	// Messages view (sub-model)
	msgView *messages.View

	// Clipboard
	copyCommand string

	// New session form
	newSessionForm *NewSessionForm

	// Command palette
	commandPalette *CommandPalette
	mergedCommands map[string]config.UserCommand

	// Help dialog
	helpDialog *components.HelpDialog

	// Rename session
	renameInput     textinput.Model
	renameSessionID string

	// Pending action for after TUI exits
	pendingCreate *PendingCreate

	// Pending recycled sessions for batch delete
	pendingRecycledSessions []session.Session

	// Review view
	reviewView *review.View

	// KV store browser
	kvStore corekv.KV
	kvView  *KVView

	// Document picker (shown on Sessions view to start reviews)
	docPickerModal *review.DocumentPickerModal

	// Notifications
	notifyBus         *tuinotify.Bus
	toastController   *ToastController
	toastView         *ToastView
	notificationModal *NotificationModal

	bus *eventbus.EventBus

	// Form dialog (for user command forms)
	formDialog      *form.Dialog
	pendingFormCmd  config.UserCommand
	pendingFormName string
	pendingFormSess *session.Session
	pendingFormArgs []string

	// Template rendering
	renderer *tmpl.Renderer

	// Startup warnings to show as toasts after init
	startupWarnings []string
}

// PendingCreate returns any pending session creation data.
func (m Model) PendingCreate() *PendingCreate {
	return m.pendingCreate
}

// actionCompleteMsg is sent when an action completes.
type actionCompleteMsg struct {
	err error
}

// recycleStartedMsg is sent when recycle begins with streaming output.
type recycleStartedMsg struct {
	output <-chan string
	done   <-chan error
	cancel context.CancelFunc
}

// recycleOutputMsg is sent when new output is available.
type recycleOutputMsg struct {
	line string
}

// recycleCompleteMsg is sent when recycle finishes.
type recycleCompleteMsg struct {
	err error
}

// notificationMsg carries a notification from an async tea.Cmd into the Update loop.
type notificationMsg struct {
	notification notify.Notification
}

// New creates a new TUI model.
func New(service *hive.SessionService, cfg *config.Config, opts Options) Model {
	// Compute merged commands: system → plugins → user
	var mergedCommands map[string]config.UserCommand
	if opts.PluginManager != nil {
		mergedCommands = opts.PluginManager.MergedCommands(config.DefaultUserCommands(), cfg.UserCommands)
	} else {
		mergedCommands = cfg.MergedUserCommands()
	}

	handler := NewKeybindingResolver(cfg.Keybindings, mergedCommands, opts.Renderer)
	cmdService := command.NewService(service, service, service)

	// Create sessions view (sub-model) — owns all session-related state
	sessionsView := sessions.New(sessions.ViewOpts{
		Cfg:             cfg,
		Service:         service,
		Handler:         handler,
		TerminalManager: opts.TerminalManager,
		PluginManager:   opts.PluginManager,
		LocalRemote:     opts.LocalRemote,
		RepoDirs:        cfg.RepoDirs,
		Renderer:        opts.Renderer,
		Bus:             opts.Bus,
	})

	// Wire handler lookups through sessions view stores
	handler.SetTmuxWindowLookup(func(sessionID string) string {
		if status, ok := sessionsView.TerminalStatuses().Get(sessionID); ok {
			return status.WindowName
		}
		return ""
	})
	handler.SetToolLookup(func(sessionID string) string {
		if status, ok := sessionsView.TerminalStatuses().Get(sessionID); ok {
			return status.Tool
		}
		return ""
	})

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = styles.TextPrimaryStyle

	// Create message view (sub-model)
	msgView := messages.New(opts.MsgStore, "*", cfg.CopyCommand)

	// Create KV browser view
	kvView := NewKVView()

	// Initialize review view with document discovery
	var contextDir string
	var docs []review.Document
	if opts.LocalRemote != "" {
		owner, repo := git.ExtractOwnerRepo(opts.LocalRemote)
		if owner != "" && repo != "" {
			contextDir = cfg.RepoContextDir(owner, repo)
			docs, _ = review.DiscoverDocuments(contextDir)
		}
	}
	if contextDir == "" {
		contextDir = cfg.SharedContextDir()
		docs, _ = review.DiscoverDocuments(contextDir)
	}

	var reviewStore *stores.ReviewStore
	if opts.DB != nil {
		reviewStore = stores.NewReviewStore(opts.DB)
	}

	reviewView := review.New(docs, contextDir, reviewStore, cfg.Review.CommentLineWidthOrDefault())

	// Initialize notification system
	var notifyStore notify.Store
	if opts.DB != nil {
		notifyStore = stores.NewNotifyStore(opts.DB)
	}
	notifyBus := tuinotify.NewBus(notifyStore)
	toastCtrl := NewToastController()
	toastView := NewToastView(toastCtrl)

	notifyBus.Subscribe(func(n notify.Notification) {
		toastCtrl.Push(n)
	})

	return Model{
		cfg:             cfg,
		service:         service,
		cmdService:      cmdService,
		handler:         handler,
		state:           stateNormal,
		spinner:         s,
		sessionsView:    sessionsView,
		msgView:         &msgView,
		activeView:      ViewSessions,
		copyCommand:     cfg.CopyCommand,
		mergedCommands:  mergedCommands,
		reviewView:      &reviewView,
		kvStore:         opts.KVStore,
		kvView:          kvView,
		notifyBus:       notifyBus,
		toastController: toastCtrl,
		toastView:       toastView,
		bus:             opts.Bus,
		renderer:        opts.Renderer,
		startupWarnings: opts.Warnings,
	}
}

// quit sets the quitting flag and emits tui.stopped.
func (m Model) quit() (Model, tea.Cmd) {
	m.quitting = true
	if m.bus != nil {
		m.bus.PublishTuiStopped(eventbus.TUIStoppedPayload{})
	}
	return m, tea.Quit
}

// Init initializes the model.
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.spinner.Tick}
	if m.bus != nil {
		m.bus.PublishTuiStarted(eventbus.TUIStartedPayload{})
	}
	// Start sessions view (handles session loading, polling, terminal, plugins, animation)
	if m.sessionsView != nil {
		if cmd := m.sessionsView.Init(); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	// Show toast if no repo dirs configured
	if len(m.cfg.RepoDirs) == 0 {
		m.toastController.Push(notify.Notification{
			Level:   notify.LevelInfo,
			Message: "No directories have been added for project start",
		})
	}
	// Start messages view
	if m.msgView != nil {
		if cmd := m.msgView.Init(); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	// Start KV store polling if store view is enabled
	if m.kvStore != nil && m.cfg.TUI.Views.Store {
		cmds = append(cmds, scheduleKVPollTick())
	}
	// Start review view file watcher
	if m.reviewView != nil {
		if cmd := m.reviewView.Init(); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	return tea.Batch(cmds...)
}

// executeAction returns a command that executes the given action.
func (m Model) executeAction(a Action) tea.Cmd {
	return func() tea.Msg {
		exec, err := m.cmdService.CreateExecutor(a)
		if err != nil {
			return actionCompleteMsg{err: err}
		}

		err = command.ExecuteSync(context.Background(), exec)
		return actionCompleteMsg{err: err}
	}
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	// Window
	case tea.WindowSizeMsg:
		return m.handleWindowSize(msg)

	// KV data loaded
	case kvKeysLoadedMsg:
		return m.handleKVKeysLoaded(msg)
	case kvEntryLoadedMsg:
		return m.handleKVEntryLoaded(msg)

	// Polling ticks (KV only — session polling is handled by sessionsView)
	case kvPollTickMsg:
		return m.handleKVPollTick(msg)
	case toastTickMsg:
		return m.handleToastTick(msg)

	// Outbound messages from sessions view
	case sessions.ActionRequestMsg:
		return m.handleSessionAction(msg)
	case sessions.FormCommandRequestMsg:
		return m.handleSessionFormCommand(msg)
	case sessions.CommandPaletteRequestMsg:
		return m.handleSessionCommandPalette(msg)
	case sessions.NewSessionRequestMsg:
		return m.handleSessionNewSession()
	case sessions.RenameRequestMsg:
		return m.handleSessionRename(msg)
	case sessions.DocReviewRequestMsg:
		return m.handleSessionDocReview()
	case sessions.RecycledDeleteRequestMsg:
		return m.handleSessionRecycledDelete(msg)
	case sessions.OpenRepoRequestMsg:
		return m.handleSessionOpenRepo(msg)

	// Action results
	case renameCompleteMsg:
		return m.handleRenameComplete(msg)
	case actionCompleteMsg:
		return m.handleActionComplete(msg)
	case recycleStartedMsg:
		return m.handleRecycleStarted(msg)
	case recycleOutputMsg:
		return m.handleRecycleOutput(msg)
	case recycleCompleteMsg:
		return m.handleRecycleComplete(msg)

	// Review delegation
	case review.DocumentChangeMsg:
		return m.handleReviewDocChange(msg)
	case review.ReviewFinalizedMsg:
		return m.handleReviewFinalized(msg)
	case review.OpenDocumentMsg:
		return m.handleReviewOpenDoc(msg)

	// Notifications
	case notificationMsg:
		return m.handleNotification(msg)

	// Input
	case tea.KeyMsg:
		return m.handleKeyMsg(msg)
	case spinner.TickMsg:
		return m.handleSpinnerTick(msg)
	}

	return m.handleFallthrough(msg)
}

// handleKey processes key presses.
func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	keyStr := msg.String()

	// Handle modal states first
	if m.state == stateCreatingSession {
		return m.handleNewSessionFormKey(msg, keyStr)
	}
	if m.state == stateCommandPalette {
		return m.handleCommandPaletteKey(msg, keyStr)
	}
	if m.state == stateShowingHelp {
		return m.handleHelpDialogKey(keyStr)
	}
	if m.state == stateShowingNotifications {
		return m.handleNotificationModalKey(keyStr)
	}
	if m.state == stateRenaming {
		return m.handleRenameKey(msg, keyStr)
	}
	if m.state == stateRunningRecycle {
		return m.handleRecycleModalKey(keyStr)
	}
	if m.state == stateConfirming {
		return m.handleConfirmModalKey(keyStr)
	}
	if m.state == stateFormInput {
		return m.handleFormDialogKey(msg, keyStr)
	}

	// When filtering in either list, pass most keys except quit
	if m.sessionsView.IsSettingFilter() || m.kvView.IsFiltering() || m.sessionsView.FocusMode() {
		return m.handleFilteringKey(msg, keyStr)
	}

	// Handle normal state
	return m.handleNormalKey(msg, keyStr)
}

// handleNewSessionFormKey handles keys when new session form is shown.
func (m Model) handleNewSessionFormKey(msg tea.KeyMsg, keyStr string) (tea.Model, tea.Cmd) {
	if keyStr == keyCtrlC {
		return m.quit()
	}

	// Pass all keys to the form (it handles esc internally)
	return m.updateNewSessionForm(msg)
}

// updateNewSessionForm routes any message to the form and handles state changes.
func (m Model) updateNewSessionForm(msg tea.Msg) (tea.Model, tea.Cmd) {
	form, cmd := m.newSessionForm.Update(msg)
	m.newSessionForm = &form

	if m.newSessionForm.Submitted() {
		result := m.newSessionForm.Result()
		m.state = stateNormal
		m.newSessionForm = nil
		m.pendingCreate = &PendingCreate{
			Remote: result.Repo.Remote,
			Name:   result.SessionName,
		}
		return m, tea.Quit
	}

	if m.newSessionForm.Cancelled() {
		m.state = stateNormal
		m.newSessionForm = nil
		return m, nil
	}

	return m, cmd
}

// showFormOrExecute checks if a command has form fields. If so, creates a form
// dialog. Otherwise falls through to normal execution via ResolveUserCommand.
func (m Model) showFormOrExecute(name string, cmd config.UserCommand, sess session.Session, args []string) (Model, tea.Cmd) {
	if len(cmd.Form) == 0 {
		action := m.handler.ResolveUserCommand(name, cmd, sess, args)
		return m.dispatchAction(action)
	}

	dialog, err := newFormDialog(name, cmd.Form, m.sessionsView.AllSessions(), m.sessionsView.DiscoveredRepos(), m.sessionsView.TerminalStatuses())
	if err != nil {
		return m, m.notifyError("form error: %v", err)
	}

	m.formDialog = dialog
	m.pendingFormCmd = cmd
	m.pendingFormName = name
	m.pendingFormSess = &sess
	m.pendingFormArgs = args
	m.state = stateFormInput
	return m, nil
}

// handleFormDialogKey handles keys when the form dialog is shown.
func (m Model) handleFormDialogKey(msg tea.KeyMsg, keyStr string) (tea.Model, tea.Cmd) {
	if keyStr == keyCtrlC {
		return m.quit()
	}

	var cmd tea.Cmd
	m.formDialog, cmd = m.formDialog.Update(msg)

	if m.formDialog.Submitted() {
		formValues := m.formDialog.FormValues()
		action := m.handler.RenderWithFormData(
			m.pendingFormName,
			m.pendingFormCmd,
			*m.pendingFormSess,
			m.pendingFormArgs,
			formValues,
		)
		m.clearFormState()
		return m.dispatchAction(action)
	}

	if m.formDialog.Cancelled() {
		m.clearFormState()
		m.state = stateNormal
		return m, nil
	}

	return m, cmd
}

// clearFormState resets all form dialog state.
func (m *Model) clearFormState() {
	m.formDialog = nil
	m.pendingFormCmd = config.UserCommand{}
	m.pendingFormName = ""
	m.pendingFormSess = nil
	m.pendingFormArgs = nil
}

// dispatchAction handles an action that may need confirmation or immediate execution.
func (m Model) dispatchAction(action Action) (Model, tea.Cmd) {
	if action.Err != nil {
		m.state = stateNormal
		return m, m.notifyError("%v", action.Err)
	}

	if action.NeedsConfirm() {
		m.state = stateConfirming
		m.pending = action
		m.modal = NewModal("Confirm", action.Confirm)
		return m, nil
	}

	if action.Type == act.TypeRecycle {
		m.state = stateNormal
		return m, m.startRecycle(action.SessionID)
	}

	if action.Exit {
		exec, err := m.cmdService.CreateExecutor(action)
		if err != nil {
			log.Error().Str("command", action.Key).Err(err).Msg("failed to create executor before exit")
		} else if err := command.ExecuteSync(context.Background(), exec); err != nil {
			log.Error().Str("command", action.Key).Err(err).Msg("command failed before exit")
		}
		return m.quit()
	}

	m.state = stateNormal
	m.pending = action
	if !action.Silent {
		m.state = stateLoading
		m.loadingMessage = "Processing..."
	}
	return m, m.executeAction(action)
}

// handleHelpDialogKey handles keys when help dialog is shown.
func (m Model) handleHelpDialogKey(keyStr string) (tea.Model, tea.Cmd) {
	switch keyStr {
	case keyCtrlC:
		return m.quit()
	case "esc", "?", "q":
		m.state = stateNormal
		m.helpDialog = nil
		return m, nil
	}
	return m, nil
}

func (m Model) handleNotificationModalKey(keyStr string) (tea.Model, tea.Cmd) {
	switch keyStr {
	case keyCtrlC:
		return m.quit()
	case "esc", "q":
		m.state = stateNormal
		m.notificationModal = nil
		return m, nil
	case "j", "down":
		m.notificationModal.ScrollDown()
	case "k", "up":
		m.notificationModal.ScrollUp()
	case "D":
		if err := m.notificationModal.Clear(); err != nil {
			return m, m.notifyError("failed to clear notifications: %v", err)
		}
		m.notifyBus.Infof("notifications cleared")
		return m, m.ensureToastTick()
	}
	return m, nil
}

// showHelpDialog creates and displays the help dialog.
func (m Model) showHelpDialog() (tea.Model, tea.Cmd) {
	var sections []components.HelpDialogSection

	userEntries := m.handler.HelpEntries()
	if len(userEntries) > 0 {
		entries := make([]components.HelpEntry, 0, len(userEntries))
		for _, e := range userEntries {
			if len(e) > 2 && e[0] == '[' {
				endBracket := strings.Index(e, "]")
				if endBracket > 0 {
					key := e[1:endBracket]
					desc := strings.TrimSpace(e[endBracket+1:])
					entries = append(entries, components.HelpEntry{Key: key, Desc: desc})
				}
			}
		}
		if len(entries) > 0 {
			sections = append(sections, components.HelpDialogSection{
				Title:   "User Commands",
				Entries: entries,
			})
		}
	}

	navEntries := []components.HelpEntry{
		{Key: "↑/k", Desc: "move up"},
		{Key: "↓/j", Desc: "move down"},
		{Key: "J/K", Desc: "next/prev active session"},
		{Key: "enter", Desc: "select session"},
		{Key: "/", Desc: "filter"},
		{Key: "tab", Desc: "switch view"},
		{Key: ":", Desc: "command palette"},
		{Key: "g", Desc: "refresh git status"},
	}

	if m.sessionsView != nil && m.sessionsView.PreviewEnabled() {
		navEntries = append(navEntries, components.HelpEntry{Key: "v", Desc: "toggle preview"})
	}

	navEntries = append(navEntries,
		components.HelpEntry{Key: "R", Desc: "rename session"},
	)
	navEntries = append(navEntries, components.HelpEntry{Key: "q", Desc: "quit"})

	sections = append(sections, components.HelpDialogSection{
		Title:   "Navigation",
		Entries: navEntries,
	})

	m.helpDialog = components.NewHelpDialog("Keyboard Shortcuts", sections, m.width, m.height)
	m.state = stateShowingHelp
	return m, nil
}

// handleCommandPaletteKey handles keys when command palette is shown.
func (m Model) handleCommandPaletteKey(msg tea.KeyMsg, keyStr string) (tea.Model, tea.Cmd) {
	if keyStr == keyCtrlC {
		return m.quit()
	}

	// Update the palette
	var cmd tea.Cmd
	m.commandPalette, cmd = m.commandPalette.Update(msg)

	// Check if user selected a command
	if entry, args, ok := m.commandPalette.SelectedCommand(); ok {
		selected := m.selectedSession()

		// Check if this is a doc review action (doesn't require a session)
		if entry.Command.Action == act.TypeDocReview {
			m.state = stateNormal
			cmd := HiveDocReviewCmd{Arg: ""}
			return m, cmd.Execute(&m)
		}

		// Messages doesn't require a session
		if entry.Command.Action == act.TypeMessages {
			m.state = stateShowingNotifications
			m.notificationModal = NewNotificationModal(m.notifyBus, m.width, m.height)
			return m, nil
		}

		// NewSession doesn't require a selected session
		if entry.Command.Action == act.TypeNewSession {
			m.state = stateNormal
			if len(m.sessionsView.DiscoveredRepos()) == 0 {
				return m, nil
			}
			return m.openNewSessionForm()
		}

		// RenameSession requires a selected session
		if entry.Command.Action == act.TypeRenameSession {
			m.state = stateNormal
			if selected == nil {
				return m, nil
			}
			return m.openRenameInput(selected)
		}

		// SetTheme doesn't require a session
		if entry.Command.Action == act.TypeSetTheme {
			m.state = stateNormal
			if len(args) > 0 {
				m.applyTheme(args[0])
			}
			return m, m.ensureToastTick()
		}

		// Check if this is a filter action (doesn't require a session)
		if sessions.IsFilterAction(entry.Command.Action) {
			m.state = stateNormal
			m.sessionsView.ApplyStatusFilter(entry.Command.Action)
			return m, nil
		}

		// Form commands don't require a selected session (they collect their own input)
		if len(entry.Command.Form) > 0 {
			m.state = stateNormal
			var sess session.Session
			if selected != nil {
				sess = *selected
			}
			return m.showFormOrExecute(entry.Name, entry.Command, sess, args)
		}

		// Other commands require a selected session
		if selected == nil {
			m.state = stateNormal
			return m, nil
		}

		// If a window sub-item is selected, override TmuxWindow for template rendering
		if ti := m.selectedTreeItem(); ti != nil && ti.IsWindowItem {
			m.handler.SetSelectedWindow(ti.WindowName)
		} else {
			m.handler.SetSelectedWindow("")
		}

		// Resolve the user command to an Action
		action := m.handler.ResolveUserCommand(entry.Name, entry.Command, *selected, args)
		action = sessions.MaybeOverrideWindowDelete(action, m.selectedTreeItem(), m.renderer)

		// Check for resolution errors (e.g., template errors)
		if action.Err != nil {
			m.state = stateNormal
			return m, m.notifyError("command error: %v", action.Err)
		}

		// Reset to normal state
		m.state = stateNormal

		// Handle confirmation if needed
		if action.NeedsConfirm() {
			m.state = stateConfirming
			m.pending = action
			m.modal = NewModal("Confirm", action.Confirm)
			return m, nil
		}

		// Execute immediately if exit requested (synchronous to avoid race conditions)
		if action.Exit {
			exec, err := m.cmdService.CreateExecutor(action)
			if err != nil {
				log.Error().Str("command", action.Key).Err(err).Msg("failed to create executor before exit")
			} else if err := command.ExecuteSync(context.Background(), exec); err != nil {
				log.Error().Str("command", action.Key).Err(err).Msg("command failed before exit")
			}
			return m.quit()
		}

		// Store pending action for exit check after completion
		m.pending = action
		if !action.Silent {
			m.state = stateLoading
			m.loadingMessage = "Processing..."
		}
		return m, m.executeAction(action)
	}

	// Check if user cancelled
	if m.commandPalette.Cancelled() {
		m.state = stateNormal
		return m, nil
	}

	return m, cmd
}

// copyToClipboard copies the given text to the system clipboard.
func (m Model) copyToClipboard(text string) error {
	if m.copyCommand == "" {
		return nil
	}

	// Split the command into program and args
	parts := strings.Fields(m.copyCommand)
	if len(parts) == 0 {
		return nil
	}

	cmd := exec.Command(parts[0], parts[1:]...)
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}

// handleFilteringKey handles keys when filter input is active.
func (m Model) handleFilteringKey(msg tea.KeyMsg, keyStr string) (tea.Model, tea.Cmd) {
	if keyStr == keyCtrlC {
		return m.quit()
	}

	// Delegate session focus mode / list filter to sessionsView
	if m.sessionsView.FocusMode() || m.sessionsView.IsSettingFilter() {
		m.sessionsView.Update(msg)
		return m, nil
	}

	// Handle KV view filtering
	if m.kvView.IsFiltering() {
		prevKey := m.kvView.SelectedKey()
		switch keyStr {
		case "esc":
			m.kvView.CancelFilter()
		case keyEnter:
			m.kvView.ConfirmFilter()
		case "backspace":
			m.kvView.DeleteFilterRune()
		default:
			if text := msg.Key().Text; text != "" {
				for _, r := range text {
					m.kvView.AddFilterRune(r)
				}
			}
		}
		// Load preview if selected key changed
		if newKey := m.kvView.SelectedKey(); newKey != prevKey && newKey != "" {
			return m, m.loadKVEntry(newKey)
		}
		return m, nil
	}

	return m, nil
}

// handleNormalKey handles keys in normal state.
func (m Model) handleNormalKey(msg tea.KeyMsg, keyStr string) (tea.Model, tea.Cmd) {
	// When editor has focus, block all keybindings except critical ones
	if m.hasEditorFocus() {
		switch keyStr {
		case keyCtrlC:
			// Allow quitting even when typing
			return m.quit()
		case "esc":
			// Allow canceling input - delegate to component
			return m.delegateToComponent(msg)
		case keyEnter:
			// Allow confirming input - delegate to component
			return m.delegateToComponent(msg)
		default:
			// Everything else goes directly to the component for typing
			return m.delegateToComponent(msg)
		}
	}

	// Messages preview modal intercepts keys before global handlers
	if m.isMessagesFocused() && m.msgView != nil && m.msgView.IsPreviewActive() {
		if keyStr == keyCtrlC {
			return m.quit()
		}
		var cmd tea.Cmd
		*m.msgView, cmd = m.msgView.Update(msg)
		return m, cmd
	}

	// Not in editor - handle core hardcoded keys
	// Global keys that work regardless of focus
	switch keyStr {
	case "q", keyCtrlC:
		return m.quit()
	case "esc":
		if m.toastController.HasToasts() {
			m.toastController.Dismiss()
			return m, nil
		}
	case "tab":
		return m.handleTabKey()
	case "?":
		// Don't show help dialog when in review view - let review view handle keys
		if !m.isReviewFocused() {
			return m.showHelpDialog()
		}
	}

	// Session-specific keys only when sessions focused
	if m.isSessionsFocused() {
		if keyStr == "g" {
			return m, m.sessionsView.RefreshGitStatuses()
		}
		if keyStr == "v" && m.sessionsView.HasTerminalIntegration() {
			m.sessionsView.TogglePreview()
			return m, nil
		}
		m.sessionsView.Update(msg)
		return m, nil
	}

	// Messages view focused - delegate all keys to the sub-model
	if m.isMessagesFocused() && m.msgView != nil {
		var cmd tea.Cmd
		*m.msgView, cmd = m.msgView.Update(msg)
		return m, cmd
	}

	// Store view focused - handle KV navigation
	if m.isStoreFocused() {
		prevKey := m.kvView.SelectedKey()
		switch keyStr {
		case "up", "k":
			m.kvView.MoveUp()
		case "down", "j":
			m.kvView.MoveDown()
		case "shift+up", "K":
			m.kvView.ScrollPreviewUp()
			return m, nil
		case "shift+down", "J":
			m.kvView.ScrollPreviewDown()
			return m, nil
		case "/":
			m.kvView.StartFilter()
		case "r":
			return m, m.loadKVKeys()
		}
		// Load preview if selected key changed
		if newKey := m.kvView.SelectedKey(); newKey != prevKey && newKey != "" {
			return m, m.loadKVEntry(newKey)
		}
		return m, nil
	}

	// Review view focused - forward keys to review view
	if m.isReviewFocused() && m.reviewView != nil {
		var cmd tea.Cmd
		*m.reviewView, cmd = m.reviewView.Update(msg)
		return m, cmd
	}

	return m, nil
}

// handleTabKey handles tab key for switching views.
// Cycle: Sessions -> Messages -> Store -> [Review if visible] -> Sessions
func (m Model) handleTabKey() (tea.Model, tea.Cmd) {
	showReviewTab := m.reviewView != nil && m.reviewView.CanShowInTabBar()
	showStoreTab := m.kvStore != nil && m.cfg.TUI.Views.Store

	switch m.activeView {
	case ViewSessions:
		m.activeView = ViewMessages
	case ViewMessages:
		switch {
		case showStoreTab:
			m.activeView = ViewStore
		case showReviewTab:
			m.activeView = ViewReview
		default:
			m.activeView = ViewSessions
		}
	case ViewStore:
		switch {
		case showReviewTab:
			m.activeView = ViewReview
		default:
			m.activeView = ViewSessions
		}
	case ViewReview:
		m.activeView = ViewSessions
	}

	m.handler.SetActiveView(m.activeView)
	if m.msgView != nil {
		m.msgView.SetActive(m.activeView == ViewMessages)
	}

	// Load KV keys when switching to Store tab
	if m.activeView == ViewStore {
		return m, m.loadKVKeys()
	}

	return m, nil
}

// renameCompleteMsg is sent when a rename operation completes.
type renameCompleteMsg struct {
	err error
}

// openRenameInput initializes the rename text input with the current session name.
func (m Model) openRenameInput(sess *session.Session) (tea.Model, tea.Cmd) {
	input := textinput.New()
	input.SetValue(sess.Name)
	input.Focus()
	input.CharLimit = 64
	input.Prompt = ""
	input.SetWidth(40)
	input.KeyMap.Paste.SetEnabled(true)
	inputStyles := textinput.DefaultStyles(true)
	inputStyles.Cursor.Color = styles.ColorPrimary
	input.SetStyles(inputStyles)

	m.renameInput = input
	m.renameSessionID = sess.ID
	m.state = stateRenaming
	return m, nil
}

// handleRenameKey handles keys when the rename input is active.
func (m Model) handleRenameKey(msg tea.KeyMsg, keyStr string) (tea.Model, tea.Cmd) {
	switch keyStr {
	case keyCtrlC:
		return m.quit()
	case "esc":
		m.state = stateNormal
		m.renameSessionID = ""
		return m, nil
	case keyEnter:
		newName := strings.TrimSpace(m.renameInput.Value())
		if newName == "" {
			m.state = stateNormal
			m.renameSessionID = ""
			return m, nil
		}
		sessionID := m.renameSessionID
		m.state = stateNormal
		m.renameSessionID = ""
		return m, m.executeRename(sessionID, newName)
	}

	// Forward to textinput
	var cmd tea.Cmd
	m.renameInput, cmd = m.renameInput.Update(msg)
	return m, cmd
}

// executeRename returns a command that renames a session and its tmux session.
func (m Model) executeRename(sessionID, newName string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()

		// Look up old session to find current tmux session name
		oldSess, err := m.service.GetSession(ctx, sessionID)
		if err != nil {
			return renameCompleteMsg{err: err}
		}

		oldTmuxName := oldSess.GetMeta(session.MetaTmuxSession)
		if oldTmuxName == "" {
			oldTmuxName = oldSess.Slug
		}

		// Rename in hive store
		if err := m.service.RenameSession(ctx, sessionID, newName); err != nil {
			return renameCompleteMsg{err: err}
		}

		// Rename tmux session (best-effort — session may not have a tmux session)
		newSlug := session.Slugify(newName)
		if oldTmuxName != "" && newSlug != "" && oldTmuxName != newSlug {
			//nolint:gosec // arguments are slugified, not user-controlled shell input
			tmuxCmd := exec.Command("tmux", "rename-session", "-t", oldTmuxName, newSlug)
			if tmuxErr := tmuxCmd.Run(); tmuxErr != nil {
				log.Debug().Err(tmuxErr).
					Str("old", oldTmuxName).
					Str("new", newSlug).
					Msg("tmux rename-session failed (session may not exist)")
			}
		}

		return renameCompleteMsg{err: nil}
	}
}

// openNewSessionForm initializes the new session form and transitions to the creating state.
func (m Model) openNewSessionForm() (tea.Model, tea.Cmd) {
	preselectedRemote := m.sessionsView.LocalRemote()

	if treeItem := m.selectedTreeItem(); treeItem != nil {
		if treeItem.IsHeader && treeItem.RepoRemote != "" {
			preselectedRemote = treeItem.RepoRemote
		} else if selected := m.selectedSession(); selected != nil {
			preselectedRemote = selected.Remote
		}
	}

	allSessions := m.sessionsView.AllSessions()
	existingNames := make(map[string]bool, len(allSessions))
	for _, s := range allSessions {
		existingNames[s.Name] = true
	}
	m.newSessionForm = NewNewSessionForm(m.sessionsView.DiscoveredRepos(), preselectedRemote, existingNames)
	m.state = stateCreatingSession
	return m, m.newSessionForm.Init()
}

// selectedTreeItem returns the currently selected tree item, or nil if none.
func (m Model) selectedTreeItem() *sessions.TreeItem {
	if m.sessionsView == nil {
		return nil
	}
	return m.sessionsView.SelectedTreeItem()
}

// hasEditorFocus returns true if any text input currently has focus.
// When an editor has focus, most keybindings should be blocked to allow normal typing.
func (m *Model) hasEditorFocus() bool {
	// Check sessions view (list filter or focus mode)
	if m.sessionsView != nil && m.sessionsView.HasEditorFocus() {
		return true
	}

	// Check messages view filter or preview
	if m.msgView != nil && m.msgView.HasEditorFocus() {
		return true
	}

	// Check review view editors (search or comment modal)
	if m.reviewView != nil && m.reviewView.HasActiveEditor() {
		return true
	}

	// Check command palette
	if m.state == stateCommandPalette {
		return true
	}

	// Check new session form
	if m.state == stateCreatingSession {
		return true
	}

	// Check rename input
	if m.state == stateRenaming {
		return true
	}

	// Check form dialog
	if m.state == stateFormInput {
		return true
	}

	return false
}

// delegateToComponent forwards a key message to the appropriate component.
// This is used when editor has focus to allow normal typing.
func (m Model) delegateToComponent(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	// Route based on current state
	switch m.state {
	case stateCommandPalette:
		if m.commandPalette != nil {
			m.commandPalette, cmd = m.commandPalette.Update(msg)
		}
		return m, cmd
	case stateCreatingSession:
		if m.newSessionForm != nil {
			*m.newSessionForm, cmd = m.newSessionForm.Update(msg)
		}
		return m, cmd
	case stateRenaming:
		m.renameInput, cmd = m.renameInput.Update(msg)
		return m, cmd
	case stateFormInput:
		if m.formDialog != nil {
			m.formDialog, cmd = m.formDialog.Update(msg)
		}
		return m, cmd
	default:
		// For other states (normal, confirming, loading, etc.),
		// delegate to active view
	}

	// Route to active view
	switch m.activeView {
	case ViewSessions:
		m.sessionsView.Update(msg)
		return m, nil
	case ViewMessages:
		if m.msgView != nil {
			*m.msgView, cmd = m.msgView.Update(msg)
		}
	case ViewStore:
		// KV view handles its own updates via explicit method calls
		return m, nil
	case ViewReview:
		if m.reviewView != nil {
			*m.reviewView, cmd = m.reviewView.Update(msg)
		}
	}

	return m, cmd
}

// isSessionsFocused returns true if the sessions view is active.
func (m Model) isSessionsFocused() bool {
	return m.activeView == ViewSessions
}

// isMessagesFocused returns true if the messages view is active.
func (m Model) isMessagesFocused() bool {
	return m.activeView == ViewMessages
}

// isReviewFocused returns true if the review view is active.
func (m Model) isReviewFocused() bool {
	return m.activeView == ViewReview
}

func (m Model) isStoreFocused() bool {
	return m.activeView == ViewStore
}

// isModalActive returns true if any modal is currently open.
func (m Model) isModalActive() bool {
	return m.state != stateNormal
}

// startRecycle returns a command that starts the recycle operation with streaming output.
func (m Model) startRecycle(sessionID string) tea.Cmd {
	return func() tea.Msg {
		exec, err := m.cmdService.CreateExecutor(Action{
			Type:      act.TypeRecycle,
			SessionID: sessionID,
		})
		if err != nil {
			return recycleCompleteMsg{err: err}
		}

		output, done, cancel := exec.Execute(context.Background())
		return recycleStartedMsg{
			output: output,
			done:   done,
			cancel: cancel,
		}
	}
}

// deleteRecycledSessionsBatch returns a command that deletes multiple recycled sessions.
func (m Model) deleteRecycledSessionsBatch(sessions []session.Session) tea.Cmd {
	return func() tea.Msg {
		var lastErr error
		for _, sess := range sessions {
			exec, err := m.cmdService.CreateExecutor(Action{
				Type:      act.TypeDelete,
				SessionID: sess.ID,
			})
			if err != nil {
				lastErr = err
				continue
			}

			if err := command.ExecuteSync(context.Background(), exec); err != nil {
				lastErr = err
			}
		}
		return actionCompleteMsg{err: lastErr}
	}
}

// ensureToastTick returns a tick command when there are active toasts.
// Multiple concurrent tick chains are harmless — Tick() uses absolute time
// and is idempotent, so extra ticks just no-op. Chains naturally stop when
// all toasts expire.
func (m *Model) ensureToastTick() tea.Cmd {
	if m.toastController.HasToasts() {
		return scheduleToastTick()
	}
	return nil
}

// notifyError publishes an error-level notification and returns a command
// to start the toast tick timer if needed.
func (m *Model) notifyError(format string, args ...any) tea.Cmd {
	m.notifyBus.Errorf(format, args...)
	return m.ensureToastTick()
}

// applyTheme switches the active theme at runtime.
func (m *Model) applyTheme(name string) {
	palette, ok := styles.GetPalette(name)
	if !ok {
		m.notifyBus.Errorf("unknown theme %q, available: %v", name, styles.ThemeNames())
		return
	}
	styles.SetTheme(palette)
	m.sessionsView.ApplyTheme()
}

// listenForRecycleOutput returns a command that waits for the next output or completion.
func listenForRecycleOutput(output <-chan string, done <-chan error) tea.Cmd {
	return func() tea.Msg {
		select {
		case line, ok := <-output:
			if !ok {
				// Output channel closed, wait for done
				err := <-done
				return recycleCompleteMsg{err: err}
			}
			return recycleOutputMsg{line: line}
		case err := <-done:
			return recycleCompleteMsg{err: err}
		}
	}
}
