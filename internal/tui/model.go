package tui

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/rs/zerolog/log"

	act "github.com/colonyops/hive/internal/core/action"
	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/internal/core/doctor"
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
	"github.com/colonyops/hive/internal/hive/updatecheck"
	"github.com/colonyops/hive/internal/tui/command"
	"github.com/colonyops/hive/internal/tui/components"
	"github.com/colonyops/hive/internal/tui/views/messages"
	"github.com/colonyops/hive/internal/tui/views/review"
	"github.com/colonyops/hive/internal/tui/views/sessions"
	"github.com/colonyops/hive/internal/tui/views/tasks"

	"github.com/colonyops/hive/pkg/tmpl"
)

// UIState represents the current state of the TUI.
type UIState int

const (
	stateNormal UIState = iota
	stateConfirming
	stateLoading
	stateStreaming
	stateCreatingSession
	stateCommandPalette
	stateShowingHelp
	stateShowingNotifications
	stateShowingInfo
	stateRenaming
	stateSettingGroup
	stateFormInput
	stateShowingTodos
)

// Key constants for event handling.
const (
	keyEnter = "enter"
	keyCtrlC = "ctrl+c"
)

// Deps holds all external dependencies for the TUI Model.
type Deps struct {
	// Required — nil causes a panic at construction time.
	Config          *config.Config
	Service         *hive.SessionService
	Renderer        *tmpl.Renderer
	TerminalManager *terminal.Manager
	PluginManager   *plugins.Manager
	TodoService     *hive.TodoService
	DB              *db.DB

	// Optional — nil disables the corresponding feature.
	MsgStore      *hive.MessageService
	Bus           *eventbus.EventBus
	KVStore       corekv.KV
	BuildInfo     BuildInfo
	DoctorService *hive.DoctorService
	Honeycomb     *hive.HoneycombService
}

// Opts holds runtime options that are not service dependencies.
type Opts struct {
	LocalRemote string
	Source      string // Source directory for file copying (cwd)
	Warnings    []string
	ConfigPath  string
}

// Model is the main Bubble Tea model for the TUI.
type Model struct {
	cfg            *config.Config
	service        *hive.SessionService
	cmdService     *command.Service
	handler        *KeybindingResolver
	state          UIState
	width          int
	height         int
	spinner        spinner.Model
	loadingMessage string
	quitting       bool

	// Source directory for file copying during session creation
	source string

	// Modal coordinator — owns all modal components, pending state, recycle streaming
	modals *ModalCoordinator

	// Sessions view (sub-model) — owns all session-related state
	sessionsView *sessions.View

	activeView ViewType

	// Messages view (sub-model)
	msgView *messages.View

	copyCommand string

	// Merged commands (system + plugins + user)
	mergedCommands map[string]config.UserCommand

	reviewView *review.View

	kvStore corekv.KV
	kvView  *KVView

	tasksView *tasks.View

	notifyStore     notify.Store
	notifyBuffer    *NotificationBuffer
	toastController *ToastController
	toastView       *ToastView

	bus *eventbus.EventBus

	todoService *hive.TodoService
	todoBadge   todoBadgeState
	todoCh      <-chan eventbus.TodoCreatedPayload

	renderer      *tmpl.Renderer
	buildInfo     BuildInfo
	updateChecker *updatecheck.Checker
	updateInfo    *updatecheck.Result
	doctorService *hive.DoctorService
	configPath    string

	// Startup warnings to show as toasts after init
	startupWarnings []string
}

// actionCompleteMsg is sent when an action completes.
type actionCompleteMsg struct {
	err error
}

// streamResult holds optional session metadata from a streaming create operation.
// Fields are populated by the executor goroutine before the done channel fires,
// so they are safe to read after receiving from done.
type streamResult struct {
	sessionID   *string
	sessionName *string
}

// streamStartedMsg is sent when a streaming operation (create, recycle) begins.
type streamStartedMsg struct {
	title  string
	output <-chan string
	done   <-chan error
	cancel context.CancelFunc
	result streamResult // optional: populated for create operations
}

// streamOutputMsg is sent when new streaming output is available.
type streamOutputMsg struct {
	line string
}

// streamCompleteMsg is sent when a streaming operation finishes.
type streamCompleteMsg struct {
	err error
}

// drainNotificationsMsg signals that buffered notifications should be drained.
type drainNotificationsMsg struct{}

type doctorResultsMsg struct {
	results []doctor.Result
}

type todoCountUpdatedMsg struct {
	pendingCount int
	openCount    int
}

type todoCountLoadFailedMsg struct{}

type todoAutoCompleteResultMsg struct {
	pendingCount int
	openCount    int
	failed       int
}

type todoCreatedMsg struct {
	payload eventbus.TodoCreatedPayload
}

// New creates a new TUI model. Panics if required Deps fields are nil.
func New(deps Deps, opts Opts) Model {
	if deps.Config == nil || deps.Service == nil || deps.Renderer == nil || deps.TerminalManager == nil || deps.PluginManager == nil || deps.TodoService == nil || deps.DB == nil {
		panic("tui.New: Config, Service, Renderer, TerminalManager, PluginManager, TodoService, and DB are required")
	}
	cfg := deps.Config
	service := deps.Service

	// Compute merged commands: system → plugins → user
	mergedCommands := deps.PluginManager.MergedCommands(config.DefaultUserCommands(), cfg.UserCommands)

	handler := NewKeybindingResolver(cfg.Keybindings, mergedCommands, deps.Renderer)
	cmdService := command.NewService(service, service, service, service, service)

	sessionsView := sessions.New(sessions.ViewOpts{
		Cfg:             cfg,
		Service:         service,
		Handler:         handler,
		TerminalManager: deps.TerminalManager,
		PluginManager:   deps.PluginManager,
		LocalRemote:     opts.LocalRemote,
		Workspaces:      cfg.Workspaces,
		Renderer:        deps.Renderer,
		Bus:             deps.Bus,
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

	msgView := messages.New(deps.MsgStore, "*", cfg.CopyCommand)

	kvView := NewKVView()

	var repoKey string
	var contextDir string
	var docs []review.Document
	if opts.LocalRemote != "" {
		owner, repo := git.ExtractOwnerRepo(opts.LocalRemote)
		if owner != "" && repo != "" {
			repoKey = owner + "/" + repo
			contextDir = cfg.RepoContextDir(owner, repo)
			docs, _ = review.DiscoverDocuments(contextDir)
		}
	}

	tasksView := tasks.New(deps.Honeycomb, repoKey)
	if contextDir == "" {
		contextDir = cfg.SharedContextDir()
		docs, _ = review.DiscoverDocuments(contextDir)
	}

	var reviewStore *stores.ReviewStore
	if deps.DB != nil {
		reviewStore = stores.NewReviewStore(deps.DB)
	}

	reviewView := review.New(docs, contextDir, reviewStore)

	notifyStore := stores.NewNotifyStore(deps.DB)
	toastCtrl := NewToastController()
	toastView := NewToastView(toastCtrl)
	notifyBuffer := NewNotificationBuffer()
	if deps.Bus != nil {
		deps.Bus.SubscribeNotificationPublished(func(p eventbus.NotificationPublishedPayload) {
			notifyBuffer.Push(notify.Notification{
				Level:   p.Level,
				Message: p.Message,
			})
		})
	}

	// Subscribe to todo events if enabled
	var todoCh <-chan eventbus.TodoCreatedPayload
	if deps.Bus != nil && cfg.Todos.Notifications.Toast {
		ch := make(chan eventbus.TodoCreatedPayload, 16)
		deps.Bus.SubscribeTodoCreated(func(payload eventbus.TodoCreatedPayload) {
			select {
			case ch <- payload:
			default:
				log.Debug().Str("todo_id", payload.Todo.ID).Msg("todo event dropped: channel buffer full")
			}
		})
		todoCh = ch
	}

	// Sessions tab is active by default
	sessionsView.SetActive(true)

	updateChecker := updatecheck.New(deps.KVStore, nil)

	return Model{
		cfg:             cfg,
		service:         service,
		cmdService:      cmdService,
		handler:         handler,
		state:           stateNormal,
		spinner:         s,
		source:          opts.Source,
		modals:          NewModalCoordinator(),
		sessionsView:    sessionsView,
		msgView:         msgView,
		activeView:      ViewSessions,
		copyCommand:     cfg.CopyCommand,
		mergedCommands:  mergedCommands,
		reviewView:      &reviewView,
		kvStore:         deps.KVStore,
		kvView:          kvView,
		tasksView:       tasksView,
		notifyStore:     notifyStore,
		notifyBuffer:    notifyBuffer,
		toastController: toastCtrl,
		toastView:       toastView,
		bus:             deps.Bus,
		todoService:     deps.TodoService,
		todoCh:          todoCh,
		renderer:        deps.Renderer,
		buildInfo:       deps.BuildInfo,
		updateChecker:   updateChecker,
		doctorService:   deps.DoctorService,
		configPath:      opts.ConfigPath,
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
	if m.notifyBuffer != nil {
		cmds = append(cmds, m.notifyBuffer.WaitForSignal())
	}
	// Start sessions view (handles session loading, polling, terminal, plugins, animation)
	if m.sessionsView != nil {
		if cmd := m.sessionsView.Init(); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	// Show toast if no repo dirs configured
	if len(m.cfg.Workspaces) == 0 {
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
	// Start tasks view
	if m.tasksView != nil {
		if cmd := m.tasksView.Init(); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	// Start review view file watcher
	if m.reviewView != nil {
		if cmd := m.reviewView.Init(); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	if m.cfg.TUI.UpdateChecker && m.updateChecker != nil && m.buildInfo.Version != "" {
		cmds = append(cmds, checkForUpdate(m.updateChecker, m.buildInfo.Version))
	}
	// Load initial todo counts and start polling + event listening
	cmds = append(cmds, m.loadTodoCounts(), scheduleTodoPollTick())
	if m.todoCh != nil {
		cmds = append(cmds, m.listenForTodoCreated())
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

// listenForTodoCreated returns a command that listens for todo creation events.
func (m Model) listenForTodoCreated() tea.Cmd {
	return func() tea.Msg {
		payload, ok := <-m.todoCh
		if !ok {
			return nil
		}
		return todoCreatedMsg{payload: payload}
	}
}

// loadTodoCounts returns a command that loads both pending and open todo counts.
// On error, it preserves previous counts and marks counts as degraded.
func (m Model) loadTodoCounts() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		pending, err := m.todoService.CountPending(ctx)
		if err != nil {
			log.Debug().Err(err).Msg("failed to load todo pending count")
			return todoCountLoadFailedMsg{}
		}
		open, err := m.todoService.CountOpen(ctx)
		if err != nil {
			log.Debug().Err(err).Msg("failed to load todo open count")
			return todoCountLoadFailedMsg{}
		}
		return todoCountUpdatedMsg{pendingCount: pending, openCount: open}
	}
}

// syncModalState notifies the sessions view whether a modal is currently open.
// This gates periodic session refresh so it doesn't fire while modals are shown.
func (m *Model) syncModalState() {
	if m.sessionsView != nil {
		m.sessionsView.SetModalActive(m.isModalActive())
	}
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		model tea.Model
		cmd   tea.Cmd
	)

	switch msg := msg.(type) {
	// Window
	case tea.WindowSizeMsg:
		model, cmd = m.handleWindowSize(msg)

	// KV data loaded
	case kvKeysLoadedMsg:
		model, cmd = m.handleKVKeysLoaded(msg)
	case kvEntryLoadedMsg:
		model, cmd = m.handleKVEntryLoaded(msg)

	// Polling ticks
	case kvPollTickMsg:
		model, cmd = m.handleKVPollTick(msg)
	case todoPollTickMsg:
		model, cmd = m.handleTodoPollTick()
	case toastTickMsg:
		model, cmd = m.handleToastTick(msg)

	// Outbound messages from sessions view
	case sessions.ActionRequestMsg:
		model, cmd = m.handleSessionAction(msg)
	case sessions.FormCommandRequestMsg:
		model, cmd = m.handleSessionFormCommand(msg)
	case sessions.CommandPaletteRequestMsg:
		model, cmd = m.handleSessionCommandPalette(msg)
	case sessions.NewSessionRequestMsg:
		model, cmd = m.handleSessionNewSession()
	case sessions.RenameRequestMsg:
		model, cmd = m.handleSessionRename(msg)
	case sessions.DocReviewRequestMsg:
		model, cmd = m.handleSessionDocReview()
	case sessions.RecycledDeleteRequestMsg:
		model, cmd = m.handleSessionRecycledDelete(msg)
	case sessions.OpenRepoRequestMsg:
		model, cmd = m.handleSessionOpenRepo(msg)
	case sessions.ErrorMsg:
		m.notifyErrorf("%v", msg.Err)
		model, cmd = m, nil

	// Action results
	case renameCompleteMsg:
		model, cmd = m.handleRenameComplete(msg)
	case setGroupCompleteMsg:
		model, cmd = m.handleSetGroupComplete(msg)
	case actionCompleteMsg:
		model, cmd = m.handleActionComplete(msg)
	case doctorResultsMsg:
		model, cmd = m.handleDoctorResults(msg)
	case streamStartedMsg:
		model, cmd = m.handleStreamStarted(msg)
	case streamOutputMsg:
		model, cmd = m.handleStreamOutput(msg)
	case streamCompleteMsg:
		model, cmd = m.handleStreamComplete(msg)

	// Review delegation
	case review.DocumentChangeMsg:
		model, cmd = m.handleReviewDocChange(msg)
	case review.ReviewFinalizedMsg:
		model, cmd = m.handleReviewFinalized(msg)
	case review.OpenDocumentMsg:
		model, cmd = m.handleReviewOpenDoc(msg)

	// Notifications
	case drainNotificationsMsg:
		model, cmd = m.handleDrainNotifications(msg)
	case updateAvailableMsg:
		model, cmd = m.handleUpdateAvailable(msg)

	// Todos
	case todoCountUpdatedMsg:
		m.todoBadge.updateCounts(msg.pendingCount, msg.openCount)
		model, cmd = m, nil
	case todoCountLoadFailedMsg:
		m.todoBadge.markDegraded()
		model, cmd = m, nil
	case todoAutoCompleteResultMsg:
		m.todoBadge.updateCounts(msg.pendingCount, msg.openCount)
		if msg.failed > 0 {
			m.publishNotificationf(notify.LevelWarning, "Failed to auto-complete %d todo(s)", msg.failed)
			model, cmd = m, nil
		} else {
			model, cmd = m, nil
		}
	case todoCreatedMsg:
		scheme := msg.payload.Todo.URI.Scheme()
		if scheme == "" {
			scheme = "todo"
		}
		m.publishNotificationf(notify.LevelInfo, "New %s: %s", scheme, msg.payload.Todo.Title)
		model, cmd = m, tea.Batch(m.loadTodoCounts(), m.listenForTodoCreated())

	case actionResultMsg:
		if msg.Err != nil {
			m.publishNotificationf(notify.LevelError, "Action failed: %v", msg.Err)
		} else if _, err := m.todoService.Complete(context.Background(), msg.TodoID); err != nil {
			log.Warn().Err(err).Str("id", msg.TodoID).Msg("failed to complete todo after action")
			m.publishNotificationf(notify.LevelWarning, "Action opened but todo not completed: %v", err)
		} else {
			m.publishNotificationf(notify.LevelInfo, "Action completed")
		}
		model, cmd = m, m.loadTodoCounts()

	// Input
	case tea.KeyPressMsg:
		model, cmd = m.handleKeyMsg(msg)
	case spinner.TickMsg:
		model, cmd = m.handleSpinnerTick(msg)

	default:
		model, cmd = m.handleFallthrough(msg)
	}

	// Sync modal state to sessions view after every update so periodic
	// refresh is paused while modals are open.
	if mdl, ok := model.(Model); ok {
		mdl.syncModalState()
		return mdl, cmd
	}
	return model, cmd
}

// handleKey processes key presses.
func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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
	if m.state == stateShowingInfo {
		return m.handleInfoDialogKey(keyStr)
	}
	if m.state == stateRenaming {
		return m.handleRenameKey(msg, keyStr)
	}
	if m.state == stateSettingGroup {
		return m.handleGroupKey(msg, keyStr)
	}
	if m.state == stateStreaming {
		return m.handleStreamingModalKey(keyStr)
	}
	if m.state == stateConfirming {
		return m.handleConfirmModalKey(keyStr)
	}
	if m.state == stateShowingTodos {
		return m.handleTodoPanelKey(keyStr)
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
func (m Model) handleNewSessionFormKey(msg tea.KeyPressMsg, keyStr string) (tea.Model, tea.Cmd) {
	if keyStr == keyCtrlC {
		return m.quit()
	}

	// Pass all keys to the form (it handles esc internally)
	return m.updateNewSessionForm(msg)
}

// updateNewSessionForm routes any message to the form and handles state changes.
func (m Model) updateNewSessionForm(msg tea.Msg) (tea.Model, tea.Cmd) {
	form, cmd := m.modals.NewSession.Update(msg)
	m.modals.NewSession = &form

	if m.modals.NewSession.Submitted() {
		result := m.modals.NewSession.Result()
		m.modals.NewSession = nil
		return m, m.startCreate(result.SessionName, result.Repo.Remote)
	}

	if m.modals.NewSession.Cancelled() {
		m.state = stateNormal
		m.modals.NewSession = nil
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
		m.notifyErrorf("form error: %v", err)
		return m, nil
	}

	m.modals.FormDialog = dialog
	m.modals.PendingFormCmd = cmd
	m.modals.PendingFormName = name
	m.modals.PendingFormSess = &sess
	m.modals.PendingFormArgs = args
	m.state = stateFormInput
	return m, nil
}

// handleFormDialogKey handles keys when the form dialog is shown.
func (m Model) handleFormDialogKey(msg tea.KeyPressMsg, keyStr string) (tea.Model, tea.Cmd) {
	if keyStr == keyCtrlC {
		return m.quit()
	}

	var cmd tea.Cmd
	m.modals.FormDialog, cmd = m.modals.FormDialog.Update(msg)

	if m.modals.FormDialog.Submitted() {
		formValues := m.modals.FormDialog.FormValues()
		action := m.handler.RenderWithFormData(
			m.modals.PendingFormName,
			m.modals.PendingFormCmd,
			*m.modals.PendingFormSess,
			m.modals.PendingFormArgs,
			formValues,
		)
		m.modals.ClearFormState()
		return m.dispatchAction(action)
	}

	if m.modals.FormDialog.Cancelled() {
		m.modals.ClearFormState()
		m.state = stateNormal
		return m, nil
	}

	return m, cmd
}

// dispatchAction handles an action that may need confirmation or immediate execution.
func (m Model) dispatchAction(action Action) (Model, tea.Cmd) {
	if action.Err != nil {
		m.state = stateNormal
		m.notifyErrorf("%v", action.Err)
		return m, nil
	}

	if action.NeedsConfirm() {
		m.state = stateConfirming
		m.modals.Pending = action
		m.modals.Confirm = NewModal("Confirm", action.Confirm)
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
	m.modals.Pending = action
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
		m.modals.DismissHelp()
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
		m.modals.DismissNotifications()
		return m, nil
	case "j", "down":
		m.modals.Notification.ScrollDown()
	case "k", "up":
		m.modals.Notification.ScrollUp()
	case "D":
		if err := m.modals.Notification.Clear(); err != nil {
			m.notifyErrorf("failed to clear notifications: %v", err)
			return m, nil
		}
		m.publishNotificationf(notify.LevelInfo, "notifications cleared")
		return m, nil
	}
	return m, nil
}

func (m Model) handleInfoDialogKey(keyStr string) (tea.Model, tea.Cmd) {
	switch keyStr {
	case keyCtrlC:
		return m.quit()
	case "esc", "q":
		m.state = stateNormal
		m.modals.DismissInfo()
		return m, nil
	case "j", "down":
		m.modals.InfoDialog.ScrollDown()
	case "k", "up":
		m.modals.InfoDialog.ScrollUp()
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
		{Key: "tab/shift+tab", Desc: "next/prev view"},
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

	m.modals.ShowHelp("Keyboard Shortcuts", sections)
	m.state = stateShowingHelp
	return m, nil
}

func (m Model) showHiveInfo() (tea.Model, tea.Cmd) {
	commit := m.buildInfo.Commit
	if len(commit) > 7 {
		commit = commit[:7]
	}

	sections := []components.InfoSection{
		{
			Title: "Build",
			Items: []components.InfoItem{
				{Label: "Version", Value: m.buildInfo.Version},
				{Label: "Commit", Value: commit},
				{Label: "Built", Value: m.buildInfo.Date},
				{Label: "Go", Value: runtime.Version()},
				{Label: "OS/Arch", Value: runtime.GOOS + "/" + runtime.GOARCH},
			},
		},
		{
			Title: "Configuration",
			Items: []components.InfoItem{
				{Label: "Config file", Value: m.configPath},
				{Label: "Config version", Value: m.cfg.Version},
				{Label: "Data directory", Value: m.cfg.DataDir},
				{Label: "Repos directory", Value: m.cfg.ReposDir()},
				{Label: "Theme", Value: m.cfg.TUI.Theme},
				{Label: "Sessions", Value: strconv.Itoa(len(m.sessionsView.AllSessions()))},
			},
		},
	}

	m.modals.ShowInfo("Hive Info", sections, "", "[j/k] scroll  [esc] close")
	m.state = stateShowingInfo
	return m, nil
}

func (m Model) showHiveDoctor() (tea.Model, tea.Cmd) {
	if m.doctorService == nil {
		m.notifyErrorf("doctor service not available")
		return m, nil
	}

	m.state = stateLoading
	m.loadingMessage = "Running health checks..."

	return m, func() tea.Msg {
		results := m.doctorService.RunChecks(context.Background(), m.configPath, false)
		return doctorResultsMsg{results: results}
	}
}

func (m Model) handleDoctorResults(msg doctorResultsMsg) (tea.Model, tea.Cmd) {
	sections, footer := buildDoctorDialogContent(msg.results)
	m.modals.ShowInfo("Hive Doctor", sections, footer, "[j/k] scroll  [esc] close")
	m.state = stateShowingInfo
	return m, nil
}

func buildDoctorDialogContent(results []doctor.Result) ([]components.InfoSection, string) {
	sections := make([]components.InfoSection, 0, len(results))
	for _, result := range results {
		items := make([]components.InfoItem, 0, len(result.Items))
		for _, item := range result.Items {
			status := components.InfoStatusNone
			switch item.Status {
			case doctor.StatusPass:
				status = components.InfoStatusPass
			case doctor.StatusWarn:
				status = components.InfoStatusWarn
			case doctor.StatusFail:
				status = components.InfoStatusFail
			}

			items = append(items, components.InfoItem{
				Label:  item.Label,
				Value:  item.Detail,
				Status: status,
			})
		}
		sections = append(sections, components.InfoSection{
			Title: result.Name,
			Items: items,
		})
	}

	passed, warned, failed := doctor.Summary(results)
	footer := fmt.Sprintf("%s  %s  %s",
		styles.TextSuccessStyle.Render(fmt.Sprintf("%d passed", passed)),
		styles.TextWarningStyle.Render(fmt.Sprintf("%d warnings", warned)),
		styles.TextErrorStyle.Render(fmt.Sprintf("%d failed", failed)),
	)

	return sections, footer
}

// handleCommandPaletteKey handles keys when command palette is shown.
func (m Model) handleCommandPaletteKey(msg tea.KeyPressMsg, keyStr string) (tea.Model, tea.Cmd) {
	if keyStr == keyCtrlC {
		return m.quit()
	}

	// Update the palette
	var cmd tea.Cmd
	m.modals.CommandPalette, cmd = m.modals.CommandPalette.Update(msg)

	// Check if user selected a command
	if entry, args, ok := m.modals.CommandPalette.SelectedCommand(); ok {
		selected := m.selectedSession()

		// Check if this is a doc review action (doesn't require a session)
		if entry.Command.Action == act.TypeDocReview {
			m.state = stateNormal
			cmd := HiveDocReviewCmd{Arg: ""}
			return m, cmd.Execute(&m)
		}

		// Notifications doesn't require a session
		if entry.Command.Action == act.TypeNotifications {
			m.state = stateShowingNotifications
			m.modals.ShowNotifications(m.notifyStore)
			return m, nil
		}

		// TodoPanel doesn't require a session
		if entry.Command.Action == act.TypeTodoPanel {
			m.state = stateShowingTodos
			m.modals.ShowTodoPanel(m.todoService)
			if failures := m.modals.TodoPanel.AcknowledgeErrorCount(); failures > 0 {
				m.notifyErrorf("failed to acknowledge %d todo(s)", failures)
				return m, nil
			}
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

		// GroupToggle doesn't require a session
		if entry.Command.Action == act.TypeGroupToggle {
			m.state = stateNormal
			cmd := m.sessionsView.ToggleGroupBy()
			return m, cmd
		}

		// GroupSet requires a selected session
		if entry.Command.Action == act.TypeGroupSet {
			m.state = stateNormal
			if selected == nil {
				return m, nil
			}
			return m.openGroupInput(selected)
		}

		// SetTheme doesn't require a session
		if entry.Command.Action == act.TypeSetTheme {
			m.state = stateNormal
			if len(args) > 0 {
				m.applyTheme(args[0])
			}
			return m, nil
		}

		if entry.Command.Action == act.TypeHiveInfo {
			m.state = stateNormal
			return m.showHiveInfo()
		}

		if entry.Command.Action == act.TypeHiveDoctor {
			m.state = stateNormal
			return m.showHiveDoctor()
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

		m.state = stateNormal
		return m.dispatchAction(action)
	}

	// Check if user cancelled
	if m.modals.CommandPalette.Cancelled() {
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
func (m Model) handleFilteringKey(msg tea.KeyPressMsg, keyStr string) (tea.Model, tea.Cmd) {
	if keyStr == keyCtrlC {
		return m.quit()
	}

	// Delegate session focus mode / list filter to sessionsView
	if m.sessionsView.FocusMode() || m.sessionsView.IsSettingFilter() {
		cmd := m.sessionsView.Update(msg)
		return m, cmd
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
func (m Model) handleNormalKey(msg tea.KeyPressMsg, keyStr string) (tea.Model, tea.Cmd) {
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

	// Messages preview pane intercepts keys before global handlers
	if m.isMessagesFocused() && m.msgView != nil && m.msgView.HasPreviewFocus() {
		if keyStr == keyCtrlC {
			return m.quit()
		}
		cmd := m.msgView.Update(msg)
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
		return m.handleTabKey(1)
	case "shift+tab":
		return m.handleTabKey(-1)
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
		cmd := m.sessionsView.Update(msg)
		return m, cmd
	}

	// Messages view focused - delegate all keys to the sub-model
	if m.isMessagesFocused() && m.msgView != nil {
		cmd := m.msgView.Update(msg)
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

	// Tasks detail pane intercepts keys before global handlers
	if m.isTasksFocused() && m.tasksView != nil && m.tasksView.HasDetailFocus() {
		if keyStr == keyCtrlC {
			return m.quit()
		}
		cmd := m.tasksView.Update(msg)
		return m, cmd
	}

	// Tasks view focused - delegate all keys to the sub-model
	if m.isTasksFocused() && m.tasksView != nil {
		cmd := m.tasksView.Update(msg)
		return m, cmd
	}

	// Review view focused - forward keys to review view
	if m.isReviewFocused() && m.reviewView != nil {
		var cmd tea.Cmd
		*m.reviewView, cmd = m.reviewView.Update(msg)
		return m, cmd
	}

	return m, nil
}

// handleTabKey handles tab/shift+tab for switching views.
// direction: +1 for next tab, -1 for previous tab.
// Cycle: Sessions -> [Tasks if visible] -> Messages -> [Store if visible] -> [Review if visible] -> Sessions
func (m Model) handleTabKey(direction int) (tea.Model, tea.Cmd) {
	tabs := []ViewType{ViewSessions}
	if m.tasksView != nil {
		tabs = append(tabs, ViewTasks)
	}
	tabs = append(tabs, ViewMessages)
	if m.kvStore != nil && m.cfg.TUI.Views.Store {
		tabs = append(tabs, ViewStore)
	}
	if m.reviewView != nil && m.reviewView.CanShowInTabBar() {
		tabs = append(tabs, ViewReview)
	}

	current := 0
	for i, v := range tabs {
		if v == m.activeView {
			current = i
			break
		}
	}

	next := (current + direction + len(tabs)) % len(tabs)
	m.activeView = tabs[next]

	m.handler.SetActiveView(m.activeView)
	m.sessionsView.SetActive(m.activeView == ViewSessions)
	if m.msgView != nil {
		m.msgView.SetActive(m.activeView == ViewMessages)
	}
	if m.tasksView != nil {
		m.tasksView.SetActive(m.activeView == ViewTasks)
	}

	// Load data when switching to Store or Tasks tab
	switch m.activeView {
	case ViewStore:
		return m, m.loadKVKeys()
	case ViewTasks:
		return m, func() tea.Msg { return tasks.RefreshTasksMsg{} }
	case ViewSessions, ViewMessages, ViewReview:
		// No special load action needed
	}

	return m, nil
}

// renameCompleteMsg is sent when a rename operation completes.
type renameCompleteMsg struct {
	err error
}

// setGroupCompleteMsg is sent when a set-group operation completes.
type setGroupCompleteMsg struct {
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

	m.modals.RenameInput = input
	m.modals.RenameSessionID = sess.ID
	m.state = stateRenaming
	return m, nil
}

// handleRenameKey handles keys when the rename input is active.
func (m Model) handleRenameKey(msg tea.KeyPressMsg, keyStr string) (tea.Model, tea.Cmd) {
	switch keyStr {
	case keyCtrlC:
		return m.quit()
	case "esc":
		m.state = stateNormal
		m.modals.RenameSessionID = ""
		return m, nil
	case keyEnter:
		newName := strings.TrimSpace(m.modals.RenameInput.Value())
		if newName == "" {
			m.state = stateNormal
			m.modals.RenameSessionID = ""
			return m, nil
		}
		sessionID := m.modals.RenameSessionID
		m.state = stateNormal
		m.modals.RenameSessionID = ""
		return m, m.executeRename(sessionID, newName)
	}

	// Forward to textinput
	var cmd tea.Cmd
	m.modals.RenameInput, cmd = m.modals.RenameInput.Update(msg)
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

// openGroupInput initializes the group text input with the current session group.
func (m Model) openGroupInput(sess *session.Session) (tea.Model, tea.Cmd) {
	input := textinput.New()
	input.SetValue(sess.Group())
	input.Focus()
	input.CharLimit = 64
	input.Prompt = ""
	input.SetWidth(40)
	input.KeyMap.Paste.SetEnabled(true)
	inputStyles := textinput.DefaultStyles(true)
	inputStyles.Cursor.Color = styles.ColorPrimary
	input.SetStyles(inputStyles)

	m.modals.GroupInput = input
	m.modals.GroupSessionID = sess.ID
	m.state = stateSettingGroup
	return m, nil
}

// handleGroupKey handles keys when the group input is active.
func (m Model) handleGroupKey(msg tea.KeyPressMsg, keyStr string) (tea.Model, tea.Cmd) {
	switch keyStr {
	case keyCtrlC:
		return m.quit()
	case "esc":
		m.state = stateNormal
		m.modals.GroupSessionID = ""
		return m, nil
	case keyEnter:
		group := strings.TrimSpace(m.modals.GroupInput.Value())
		sessionID := m.modals.GroupSessionID
		m.state = stateNormal
		m.modals.GroupSessionID = ""
		return m, m.executeSetGroup(sessionID, group)
	}

	// Forward to textinput
	var cmd tea.Cmd
	m.modals.GroupInput, cmd = m.modals.GroupInput.Update(msg)
	return m, cmd
}

// executeSetGroup returns a command that sets a session's group.
func (m Model) executeSetGroup(sessionID, group string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		if err := m.service.SetSessionGroup(ctx, sessionID, group); err != nil {
			return setGroupCompleteMsg{err: err}
		}
		return setGroupCompleteMsg{err: nil}
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
	m.modals.NewSession = NewNewSessionForm(m.sessionsView.DiscoveredRepos(), preselectedRemote, existingNames)
	m.state = stateCreatingSession
	return m, m.modals.NewSession.Init()
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

	// Check modal coordinator (command palette, new session, rename, form dialog)
	return m.modals.HasEditorFocus(m.state)
}

// delegateToComponent forwards a key message to the appropriate component.
// This is used when editor has focus to allow normal typing.
func (m Model) delegateToComponent(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	// Route based on current state
	switch m.state {
	case stateCommandPalette:
		if m.modals.CommandPalette != nil {
			m.modals.CommandPalette, cmd = m.modals.CommandPalette.Update(msg)
		}
		return m, cmd
	case stateCreatingSession:
		if m.modals.NewSession != nil {
			*m.modals.NewSession, cmd = m.modals.NewSession.Update(msg)
		}
		return m, cmd
	case stateRenaming:
		m.modals.RenameInput, cmd = m.modals.RenameInput.Update(msg)
		return m, cmd
	case stateSettingGroup:
		m.modals.GroupInput, cmd = m.modals.GroupInput.Update(msg)
		return m, cmd
	case stateFormInput:
		if m.modals.FormDialog != nil {
			m.modals.FormDialog, cmd = m.modals.FormDialog.Update(msg)
		}
		return m, cmd
	default:
		// For other states (normal, confirming, loading, etc.),
		// delegate to active view
	}

	// Route to active view
	switch m.activeView {
	case ViewSessions:
		cmd = m.sessionsView.Update(msg)
		return m, cmd
	case ViewTasks:
		if m.tasksView != nil {
			cmd = m.tasksView.Update(msg)
		}
	case ViewMessages:
		if m.msgView != nil {
			cmd = m.msgView.Update(msg)
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

func (m Model) isTasksFocused() bool {
	return m.activeView == ViewTasks
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
			return streamCompleteMsg{err: err}
		}

		output, done, cancel := exec.Execute(context.Background())
		return streamStartedMsg{
			title:  "Recycling session...",
			output: output,
			done:   done,
			cancel: cancel,
		}
	}
}

// startCreate returns a command that starts session creation with streaming output.
func (m Model) startCreate(name, remote string) tea.Cmd {
	return func() tea.Msg {
		exec := m.cmdService.NewCreateExecutor(hive.CreateOptions{
			Name:       name,
			Remote:     remote,
			Source:     m.source,
			Background: true,
		})

		output, done, cancel := exec.Execute(context.Background())
		return streamStartedMsg{
			title:  "Creating session...",
			output: output,
			done:   done,
			cancel: cancel,
			result: streamResult{
				sessionID:   &exec.ResultSessionID,
				sessionName: &exec.ResultSessionName,
			},
		}
	}
}

// listenForStreamingOutput returns a command that waits for the next line or completion
// from a streaming operation.
func listenForStreamingOutput(output <-chan string, done <-chan error) tea.Cmd {
	return func() tea.Msg {
		select {
		case line, ok := <-output:
			if !ok {
				// Output channel closed, wait for done
				err := <-done
				return streamCompleteMsg{err: err}
			}
			return streamOutputMsg{line: line}
		case err := <-done:
			return streamCompleteMsg{err: err}
		}
	}
}

// deleteRecycledSessionsBatch returns a command that deletes multiple recycled sessions.
func (m Model) deleteRecycledSessionsBatch(sessions []session.Session) tea.Cmd {
	return func() tea.Msg {
		var errs []error
		for _, sess := range sessions {
			exec, err := m.cmdService.CreateExecutor(Action{
				Type:      act.TypeDelete,
				SessionID: sess.ID,
			})
			if err != nil {
				log.Error().Err(err).Str("session", sess.ID).Msg("failed to create delete executor")
				errs = append(errs, err)
				continue
			}

			if err := command.ExecuteSync(context.Background(), exec); err != nil {
				log.Error().Err(err).Str("session", sess.ID).Msg("failed to delete recycled session")
				errs = append(errs, err)
			}
		}
		return actionCompleteMsg{err: errors.Join(errs...)}
	}
}

// publishNotificationf publishes a notification via the EventBus.
func (m *Model) publishNotificationf(level notify.Level, format string, args ...any) {
	message := fmt.Sprintf(format, args...)
	if m.bus != nil {
		m.bus.PublishNotificationPublished(eventbus.NotificationPublishedPayload{
			Level:   level,
			Message: message,
		})
		return
	}
	if m.notifyBuffer != nil {
		m.notifyBuffer.Push(notify.Notification{Level: level, Message: message})
	}
}

// notifyErrorf publishes an error-level notification.
func (m *Model) notifyErrorf(format string, args ...any) {
	m.publishNotificationf(notify.LevelError, format, args...)
}

// notifyError publishes an error-level notification and returns nil.
// This is a convenience wrapper for use in (Model, tea.Cmd) return patterns.
func (m *Model) notifyError(format string, args ...any) tea.Cmd {
	m.notifyErrorf(format, args...)
	return nil
}

// applyTheme switches the active theme at runtime.
func (m *Model) applyTheme(name string) {
	palette, ok := styles.GetPalette(name)
	if !ok {
		m.publishNotificationf(notify.LevelError, "unknown theme %q, available: %v", name, styles.ThemeNames())
		return
	}
	styles.SetTheme(palette)
	m.sessionsView.ApplyTheme()
}
