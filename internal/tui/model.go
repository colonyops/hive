package tui

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/rs/zerolog/log"

	act "github.com/colonyops/hive/internal/core/action"
	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/internal/core/eventbus"
	"github.com/colonyops/hive/internal/core/git"
	corekv "github.com/colonyops/hive/internal/core/kv"
	"github.com/colonyops/hive/internal/core/messaging"
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
	"github.com/colonyops/hive/internal/tui/views/review"

	"github.com/colonyops/hive/pkg/kv"
	"github.com/colonyops/hive/pkg/tmpl"
)

// Buffer pools for reducing allocations in rendering.
var builderPool = sync.Pool{
	New: func() any {
		return &strings.Builder{}
	},
}

// UIState represents the current state of the TUI.
type UIState int

const (
	stateNormal UIState = iota
	stateConfirming
	stateLoading
	stateRunningRecycle
	statePreviewingMessage
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
	list           list.Model
	handler        *KeybindingResolver
	state          UIState
	modal          Modal
	pending        Action
	width          int
	height         int
	spinner        spinner.Model
	loadingMessage string
	quitting       bool
	gitStatuses    *kv.Store[string, GitStatus]
	gitWorkers     int
	columnWidths   *ColumnWidths

	// Terminal integration
	terminalManager    *terminal.Manager
	terminalStatuses   *kv.Store[string, TerminalStatus]
	previewEnabled     bool              // toggle tmux pane preview sidebar
	previewTemplates   *PreviewTemplates // parsed preview panel templates
	currentTmuxSession string            // current tmux session name (to prevent recursive preview)

	// Plugin integration
	pluginManager      *plugins.Manager
	pluginStatuses     map[string]*kv.Store[string, plugins.Status] // plugin name -> session ID -> status
	pluginResultsChan  <-chan plugins.Result                        // from background worker
	pluginPollInterval time.Duration                                // interval for background polling

	// Status animation
	animationFrame int
	treeDelegate   TreeDelegate // Keep reference to update animation frame

	// Filtering
	localRemote  string            // Remote URL of current directory (for highlighting)
	allSessions  []session.Session // All sessions (unfiltered)
	statusFilter terminal.Status   // Filter by terminal status (empty = show all)

	// Focus mode filtering (sessions tree only)
	focusMode        bool            // true when focus mode filter is active
	focusFilter      string          // current focus filter text
	focusFilterInput textinput.Model // dedicated input for focus mode

	// Recycle streaming state
	outputModal   OutputModal
	recycleOutput <-chan string
	recycleDone   <-chan error
	recycleCancel context.CancelFunc

	// Layout
	activeView ViewType // which view is shown
	refreshing bool     // true during background session refresh

	// Messages
	msgStore     *hive.MessageService
	msgView      *MessagesView
	allMessages  []messaging.Message
	lastPollTime time.Time
	topicFilter  string

	// Message preview
	previewModal MessagePreviewModal

	// Clipboard
	copyCommand string

	// New session form
	repoDirs        []string
	discoveredRepos []DiscoveredRepo
	newSessionForm  *NewSessionForm

	// Command palette
	commandPalette *CommandPalette
	mergedCommands map[string]config.UserCommand // system + plugin + user commands

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

// sessionsLoadedMsg is sent when sessions are loaded.
type sessionsLoadedMsg struct {
	sessions []session.Session
	err      error
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

// reposDiscoveredMsg is sent when repository scanning completes.
type reposDiscoveredMsg struct {
	repos []DiscoveredRepo
}

// pluginStatusUpdateMsg is sent when a single plugin status result arrives.
type pluginStatusUpdateMsg struct {
	PluginName string
	SessionID  string
	Status     plugins.Status
	Err        error
}

// pluginWorkerStartedMsg is sent when the background plugin worker starts.
type pluginWorkerStartedMsg struct {
	resultsChan <-chan plugins.Result
}

// notificationMsg carries a notification from an async tea.Cmd into the Update loop.
type notificationMsg struct {
	notification notify.Notification
}

// New creates a new TUI model.
func New(service *hive.SessionService, cfg *config.Config, opts Options) Model {
	gitStatuses := kv.New[string, GitStatus]()
	terminalStatuses := kv.New[string, TerminalStatus]()
	columnWidths := &ColumnWidths{}

	// Initialize plugin status stores for each enabled plugin
	var pluginStatuses map[string]*kv.Store[string, plugins.Status]
	if opts.PluginManager != nil {
		pluginStatuses = make(map[string]*kv.Store[string, plugins.Status])
		for _, p := range opts.PluginManager.EnabledPlugins() {
			if p.StatusProvider() != nil {
				pluginStatuses[p.Name()] = kv.New[string, plugins.Status]()
			}
		}
	}

	delegate := NewTreeDelegate()
	delegate.GitStatuses = gitStatuses
	delegate.TerminalStatuses = terminalStatuses
	delegate.ColumnWidths = columnWidths
	delegate.PluginStatuses = pluginStatuses
	delegate.IconsEnabled = cfg.TUI.IconsEnabled()

	l := list.New([]list.Item{}, delegate, 0, 0)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.SetShowTitle(false)  // Title shown in tab bar instead
	l.SetShowFilter(false) // Don't reserve space for filter bar until filtering
	l.Styles.TitleBar = lipgloss.NewStyle()
	l.Styles.Title = lipgloss.NewStyle()
	// Configure filter input styles for bubbles v2
	l.FilterInput.Prompt = "Filter: "
	filterStyles := textinput.DefaultStyles(true) // dark mode
	filterStyles.Focused.Prompt = styles.ListFilterPromptStyle
	filterStyles.Cursor.Color = styles.ColorPrimary
	l.FilterInput.SetStyles(filterStyles)

	// Create dedicated focus filter input (matches list filter styling)
	focusInput := textinput.New()
	focusInput.Prompt = "/"
	focusInputStyles := textinput.DefaultStyles(true)
	focusInputStyles.Focused.Prompt = styles.ListFilterPromptStyle
	focusInputStyles.Cursor.Color = styles.ColorPrimary
	focusInput.SetStyles(focusInputStyles)

	// Style help to match messages view (consistent gray, bullet separators, left padding)
	helpStyle := styles.TextMutedStyle
	l.Help.Styles.ShortKey = helpStyle
	l.Help.Styles.ShortDesc = helpStyle
	l.Help.Styles.ShortSeparator = helpStyle
	l.Help.Styles.FullKey = helpStyle
	l.Help.Styles.FullDesc = helpStyle
	l.Help.Styles.FullSeparator = helpStyle
	l.Help.ShortSeparator = " • "
	l.Styles.HelpStyle = styles.ListHelpContainerStyle

	// Compute merged commands: system → plugins → user
	var mergedCommands map[string]config.UserCommand
	if opts.PluginManager != nil {
		mergedCommands = opts.PluginManager.MergedCommands(config.DefaultUserCommands(), cfg.UserCommands)
	} else {
		mergedCommands = cfg.MergedUserCommands()
	}

	handler := NewKeybindingResolver(cfg.Keybindings, mergedCommands, opts.Renderer)
	handler.SetTmuxWindowLookup(func(sessionID string) string {
		if status, ok := terminalStatuses.Get(sessionID); ok {
			return status.WindowName
		}
		return ""
	})
	handler.SetToolLookup(func(sessionID string) string {
		if status, ok := terminalStatuses.Get(sessionID); ok {
			return status.Tool
		}
		return ""
	})
	cmdService := command.NewService(service, service, service)

	// Add minimal keybindings to list help - just navigation and help trigger
	l.AdditionalShortHelpKeys = func() []key.Binding {
		return []key.Binding{
			key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑/↓", "navigate")),
			key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		}
	}

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = styles.TextPrimaryStyle

	// Create message view
	msgView := NewMessagesView()

	// Create KV browser view
	kvView := NewKVView()

	// Detect current tmux session to prevent recursive preview
	currentTmux := detectCurrentTmuxSession()

	// Parse preview templates
	previewTemplates := ParsePreviewTemplates(
		cfg.TUI.Preview.TitleTemplate,
		cfg.TUI.Preview.StatusTemplate,
	)

	// Initialize review view with document discovery
	// Use repo-specific context directory if we have a local remote, otherwise shared
	var contextDir string
	var docs []review.Document
	if opts.LocalRemote != "" {
		owner, repo := git.ExtractOwnerRepo(opts.LocalRemote)
		if owner != "" && repo != "" {
			contextDir = cfg.RepoContextDir(owner, repo)
			docs, _ = review.DiscoverDocuments(contextDir)
		}
	}
	// Fallback to shared if no repo context
	if contextDir == "" {
		contextDir = cfg.SharedContextDir()
		docs, _ = review.DiscoverDocuments(contextDir)
	}

	// Create review store if database is available
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

	// Wire bus -> toast controller
	notifyBus.Subscribe(func(n notify.Notification) {
		toastCtrl.Push(n)
	})

	return Model{
		cfg:                cfg,
		service:            service,
		cmdService:         cmdService,
		list:               l,
		handler:            handler,
		state:              stateNormal,
		spinner:            s,
		gitStatuses:        gitStatuses,
		gitWorkers:         cfg.Git.StatusWorkers,
		columnWidths:       columnWidths,
		terminalManager:    opts.TerminalManager,
		terminalStatuses:   terminalStatuses,
		previewEnabled:     cfg.TUI.PreviewEnabled,
		previewTemplates:   previewTemplates,
		currentTmuxSession: currentTmux,
		pluginManager:      opts.PluginManager,
		pluginStatuses:     pluginStatuses,
		pluginPollInterval: cfg.Plugins.GitHub.ResultsCache, // use GitHub cache duration as poll interval
		treeDelegate:       delegate,
		localRemote:        opts.LocalRemote,
		msgStore:           opts.MsgStore,
		msgView:            msgView,
		topicFilter:        "*",
		activeView:         ViewSessions,
		copyCommand:        cfg.CopyCommand,
		repoDirs:           cfg.RepoDirs,
		mergedCommands:     mergedCommands,
		reviewView:         &reviewView,
		kvStore:            opts.KVStore,
		kvView:             kvView,
		notifyBus:          notifyBus,
		toastController:    toastCtrl,
		toastView:          toastView,
		bus:                opts.Bus,
		focusFilterInput:   focusInput,
		renderer:           opts.Renderer,
		startupWarnings:    opts.Warnings,
	}
}

// detectCurrentTmuxSession returns the current tmux session name, or empty if not in tmux.
func detectCurrentTmuxSession() string {
	cmd := exec.Command("tmux", "display-message", "-p", "#S")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

// isCurrentTmuxSession returns true if the given session matches the current tmux session.
// This prevents recursive preview when hive is previewing its own pane.
func (m Model) isCurrentTmuxSession(sess *session.Session) bool {
	if m.currentTmuxSession == "" {
		return false
	}

	// Check exact match with slug
	if m.currentTmuxSession == sess.Slug {
		return true
	}

	// Check prefix match (tmux session might be slug_suffix or slug-suffix)
	if strings.HasPrefix(m.currentTmuxSession, sess.Slug+"_") ||
		strings.HasPrefix(m.currentTmuxSession, sess.Slug+"-") {
		return true
	}

	// Check metadata for explicit tmux session name
	if tmuxSession := sess.Metadata[session.MetaTmuxSession]; tmuxSession != "" {
		if m.currentTmuxSession == tmuxSession {
			return true
		}
	}

	return false
}

// quit sets the quitting flag and emits tui.stopped.
func (m Model) quit() (Model, tea.Cmd) {
	m.quitting = true
	if m.bus != nil {
		m.bus.PublishTuiStopped(eventbus.TUIStoppedPayload{})
	}
	return m, tea.Quit
}

// findSessionByID returns a pointer to the session with the given ID, or nil if not found.
func (m Model) findSessionByID(id string) *session.Session {
	for i := range m.allSessions {
		if m.allSessions[i].ID == id {
			return &m.allSessions[i]
		}
	}
	return nil
}

// Init initializes the model.
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.loadSessions(), m.spinner.Tick}
	if m.bus != nil {
		m.bus.PublishTuiStarted(eventbus.TUIStartedPayload{})
	}
	// Start message polling if we have a store
	if m.msgStore != nil {
		cmds = append(cmds, loadMessages(m.msgStore, m.topicFilter, time.Time{}))
		cmds = append(cmds, schedulePollTick())
	}
	// Start session refresh timer
	if cmd := m.scheduleSessionRefresh(); cmd != nil {
		cmds = append(cmds, cmd)
	}
	// Start KV store polling if store view is enabled
	if m.kvStore != nil && m.cfg.TUI.Views.Store {
		cmds = append(cmds, scheduleKVPollTick())
	}
	// Scan for repositories if configured
	if len(m.repoDirs) > 0 {
		cmds = append(cmds, m.scanRepoDirs())
	} else {
		m.toastController.Push(notify.Notification{
			Level:   notify.LevelInfo,
			Message: "No directories have been added for project start",
		})
	}
	// Start terminal status polling and animation if integration is enabled
	if m.terminalManager != nil && m.terminalManager.HasEnabledIntegrations() {
		cmds = append(cmds, startTerminalPollTicker(m.cfg.Tmux.PollInterval))
		cmds = append(cmds, scheduleAnimationTick())
	}
	// Start plugin background worker if plugins are enabled
	if m.pluginManager != nil && len(m.pluginStatuses) > 0 {
		cmds = append(cmds, m.startPluginWorker())
	}
	// Start review view file watcher
	if m.reviewView != nil {
		if cmd := m.reviewView.Init(); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	return tea.Batch(cmds...)
}

// startPluginWorker returns a command that starts the background plugin worker.
func (m Model) startPluginWorker() tea.Cmd {
	return func() tea.Msg {
		resultsChan := m.pluginManager.StartBackgroundWorker(context.Background(), m.pluginPollInterval)
		return pluginWorkerStartedMsg{resultsChan: resultsChan}
	}
}

// listenForPluginResult returns a command that waits for the next plugin result.
func listenForPluginResult(ch <-chan plugins.Result) tea.Cmd {
	if ch == nil {
		return nil
	}
	return func() tea.Msg {
		result, ok := <-ch
		if !ok {
			// Channel closed, stop listening
			return nil
		}
		return pluginStatusUpdateMsg{
			PluginName: result.PluginName,
			SessionID:  result.SessionID,
			Status:     result.Status,
			Err:        result.Err,
		}
	}
}

// scanRepoDirs returns a command that scans configured directories for git repositories.
func (m Model) scanRepoDirs() tea.Cmd {
	return func() tea.Msg {
		repos, _ := ScanRepoDirs(context.Background(), m.repoDirs, m.service.Git())
		return reposDiscoveredMsg{repos: repos}
	}
}

// loadSessions returns a command that loads sessions from the service.
func (m Model) loadSessions() tea.Cmd {
	return func() tea.Msg {
		sessions, err := m.service.ListSessions(context.Background())
		return sessionsLoadedMsg{sessions: sessions, err: err}
	}
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

	// Data loaded
	case messagesLoadedMsg:
		return m.handleMessagesLoaded(msg)
	case kvKeysLoadedMsg:
		return m.handleKVKeysLoaded(msg)
	case kvEntryLoadedMsg:
		return m.handleKVEntryLoaded(msg)
	case sessionsLoadedMsg:
		return m.handleSessionsLoaded(msg)
	case gitStatusBatchCompleteMsg:
		return m.handleGitStatusComplete(msg)
	case terminalStatusBatchCompleteMsg:
		return m.handleTerminalStatusComplete(msg)
	case pluginWorkerStartedMsg:
		return m.handlePluginWorkerStarted(msg)
	case pluginStatusUpdateMsg:
		return m.handlePluginStatusUpdate(msg)
	case reposDiscoveredMsg:
		return m.handleReposDiscovered(msg)

	// Polling ticks
	case pollTickMsg:
		return m.handlePollTick(msg)
	case sessionRefreshTickMsg:
		return m.handleSessionRefreshTick(msg)
	case kvPollTickMsg:
		return m.handleKVPollTick(msg)
	case terminalPollTickMsg:
		return m.handleTerminalPollTick(msg)
	case animationTickMsg:
		return m.handleAnimationTick(msg)
	case toastTickMsg:
		return m.handleToastTick(msg)

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
	if m.state == statePreviewingMessage {
		return m.handlePreviewModalKey(msg, keyStr)
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
	if m.list.SettingFilter() || m.msgView.IsFiltering() || m.kvView.IsFiltering() || m.focusMode {
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

	dialog, err := newFormDialog(name, cmd.Form, m.allSessions, m.discoveredRepos, m.terminalStatuses)
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

// handleRecycleModalKey handles keys when recycle modal is shown.
func (m Model) handleRecycleModalKey(keyStr string) (tea.Model, tea.Cmd) {
	switch keyStr {
	case keyCtrlC:
		if m.recycleCancel != nil {
			m.recycleCancel()
		}
		return m.quit()
	case "esc":
		if m.outputModal.IsRunning() && m.recycleCancel != nil {
			m.recycleCancel()
		}
		m.state = stateNormal
		m.pending = Action{}
		return m, m.loadSessions()
	case keyEnter:
		if !m.outputModal.IsRunning() {
			m.state = stateNormal
			m.pending = Action{}
			return m, m.loadSessions()
		}
	}
	return m, nil
}

// handleConfirmModalKey handles keys when confirmation modal is shown.
func (m Model) handleConfirmModalKey(keyStr string) (tea.Model, tea.Cmd) {
	switch keyStr {
	case keyEnter:
		m.state = stateNormal
		if m.modal.ConfirmSelected() {
			action := m.pending
			if action.Type == act.TypeRecycle {
				return m, m.startRecycle(action.SessionID)
			}
			// Handle batch delete of recycled sessions
			if action.Type == act.TypeDeleteRecycledBatch {
				sessions := m.pendingRecycledSessions
				m.pending = Action{}
				m.pendingRecycledSessions = nil
				return m, m.deleteRecycledSessionsBatch(sessions)
			}
			return m, m.executeAction(action)
		}
		m.pending = Action{}
		m.pendingRecycledSessions = nil
		return m, nil
	case "esc":
		m.state = stateNormal
		m.pending = Action{}
		m.pendingRecycledSessions = nil
		return m, nil
	case "left", "right", "h", "l", "tab":
		m.modal.ToggleSelection()
		return m, nil
	}
	return m, nil
}

// handleRecycledPlaceholderKey handles keys when a recycled placeholder is selected.
// Only allows delete action to permanently remove all recycled sessions.
func (m Model) handleRecycledPlaceholderKey(keyStr string, treeItem *TreeItem) (tea.Model, tea.Cmd) {
	// Check if this key is bound to delete action
	kb, exists := m.handler.keybindings[keyStr]
	if !exists {
		return m, nil
	}

	cmd, cmdExists := m.handler.commands[kb.Cmd]
	if !cmdExists || cmd.Action != act.TypeDelete {
		return m, nil // Only delete is allowed on recycled placeholders
	}

	// Show confirmation modal for deleting all recycled sessions
	confirmMsg := fmt.Sprintf("Permanently delete %d recycled session(s)?", treeItem.RecycledCount)
	m.state = stateConfirming
	m.pending = Action{
		Type:    act.TypeDeleteRecycledBatch,
		Key:     keyStr,
		Help:    "delete recycled sessions",
		Confirm: confirmMsg,
	}
	// Store recycled sessions in pending action for later deletion
	m.pendingRecycledSessions = treeItem.RecycledSessions
	m.modal = NewModal("Confirm", confirmMsg)
	return m, nil
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

	// Add user-configured keybindings section
	userEntries := m.handler.HelpEntries()
	if len(userEntries) > 0 {
		entries := make([]components.HelpEntry, 0, len(userEntries))
		for _, e := range userEntries {
			// Parse "[key] description" format
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

	// Add navigation section
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

	// Add preview toggle if terminal integration enabled
	if m.terminalManager != nil && m.terminalManager.HasEnabledIntegrations() {
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

// handlePreviewModalKey handles keys when message preview modal is shown.
func (m Model) handlePreviewModalKey(msg tea.KeyMsg, keyStr string) (tea.Model, tea.Cmd) {
	// Clear copy status on any key press
	m.previewModal.ClearCopyStatus()

	switch keyStr {
	case keyCtrlC:
		return m.quit()
	case "esc", keyEnter, "q":
		m.state = stateNormal
		return m, nil
	case "up", "k":
		m.previewModal.ScrollUp()
		return m, nil
	case "down", "j":
		m.previewModal.ScrollDown()
		return m, nil
	case "c", "y":
		// Copy payload to clipboard
		if err := m.copyToClipboard(m.previewModal.Payload()); err != nil {
			m.previewModal.SetCopyStatus("Copy failed: " + err.Error())
		} else {
			m.previewModal.SetCopyStatus("Copied!")
		}
		return m, nil
	default:
		// Pass other messages to viewport for mouse wheel etc
		m.previewModal.UpdateViewport(msg)
		return m, nil
	}
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
			if len(m.discoveredRepos) == 0 {
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
		if isFilterAction(entry.Command.Action) {
			m.state = stateNormal
			m.handleFilterAction(entry.Command.Action)
			return m.applyFilter()
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
		action = m.maybeOverrideWindowDelete(action, m.selectedTreeItem())

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

	// Handle focus mode filtering (sessions view)
	if m.focusMode {
		switch keyStr {
		case "esc":
			m.stopFocusMode()
			return m, nil
		case keyEnter:
			m.stopFocusMode()
			return m, nil
		default:
			var cmd tea.Cmd
			m.focusFilterInput, cmd = m.focusFilterInput.Update(msg)
			// Update filter and navigate on every keystroke
			m.updateFocusFilter(m.focusFilterInput.Value())
			return m, cmd
		}
	}

	// Handle message view filtering
	if m.msgView.IsFiltering() {
		switch keyStr {
		case "esc":
			m.msgView.CancelFilter()
		case keyEnter:
			m.msgView.ConfirmFilter()
		case "backspace":
			m.msgView.DeleteFilterRune()
		default:
			// Add character to filter if it's a printable rune
			// In bubbletea V2, msg.Runes is replaced with msg.Key().Text
			if text := msg.Key().Text; text != "" {
				for _, r := range text {
					m.msgView.AddFilterRune(r)
				}
			}
		}
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

	// Handle session list filtering
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
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
			return m, m.refreshGitStatuses()
		}
		if keyStr == "v" && m.terminalManager != nil && m.terminalManager.HasEnabledIntegrations() {
			m.previewEnabled = !m.previewEnabled
			return m, nil
		}
		return m.handleSessionsKey(msg, keyStr)
	}

	// Handle keys based on active view
	if m.isMessagesFocused() {
		// Messages view focused - handle navigation
		switch keyStr {
		case keyEnter:
			// Open message preview modal
			selectedMsg := m.selectedMessage()
			if selectedMsg != nil {
				m.state = statePreviewingMessage
				m.previewModal = NewMessagePreviewModal(*selectedMsg, m.width, m.height)
			}
		case "up", "k":
			m.msgView.MoveUp()
		case "down", "j":
			m.msgView.MoveDown()
		case "/":
			m.msgView.StartFilter()
		}
		return m, nil
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
	preselectedRemote := m.localRemote

	if treeItem := m.selectedTreeItem(); treeItem != nil {
		if treeItem.IsHeader && treeItem.RepoRemote != "" {
			preselectedRemote = treeItem.RepoRemote
		} else if selected := m.selectedSession(); selected != nil {
			preselectedRemote = selected.Remote
		}
	}

	existingNames := make(map[string]bool, len(m.allSessions))
	for _, s := range m.allSessions {
		existingNames[s.Name] = true
	}
	m.newSessionForm = NewNewSessionForm(m.discoveredRepos, preselectedRemote, existingNames)
	m.state = stateCreatingSession
	return m, m.newSessionForm.Init()
}

// handleSessionsKey handles keys when sessions pane is focused.
func (m Model) handleSessionsKey(msg tea.KeyMsg, keyStr string) (tea.Model, tea.Cmd) {
	// Handle navigation keys - skip over headers
	switch keyStr {
	case "up", "k":
		m.navigateSkippingHeaders(-1)
		return m, nil
	case "down", "j":
		m.navigateSkippingHeaders(1)
		return m, nil
	}

	// Handle navigation to next/prev active session (doesn't require selection)
	if m.handler.IsAction(keyStr, act.TypeNextActive) {
		m.navigateToNextActive(1)
		return m, nil
	}
	if m.handler.IsAction(keyStr, act.TypePrevActive) {
		m.navigateToNextActive(-1)
		return m, nil
	}

	// Handle new session action (only if repos are discovered)
	if m.handler.IsAction(keyStr, act.TypeNewSession) && len(m.discoveredRepos) > 0 {
		return m.openNewSessionForm()
	}

	// Handle '/' for focus mode filter
	if keyStr == "/" {
		m.focusMode = true
		m.focusFilter = ""
		m.focusFilterInput.Reset()
		m.focusFilterInput.SetValue("")

		// Hide list help and reduce list size to make room for search input
		contentHeight := m.height - 3
		if contentHeight < 2 {
			contentHeight = 2
		}

		var listWidth int
		if m.previewEnabled && m.width >= 80 && m.activeView == ViewSessions {
			listWidth = int(float64(m.width) * 0.25)
		} else {
			listWidth = m.width
		}

		m.list.SetShowHelp(false)
		m.list.SetSize(listWidth, contentHeight-1) // Make room for search input

		return m, m.focusFilterInput.Focus()
	}

	// Handle ':' for command palette (allow even without selection for filter commands)
	if keyStr == ":" {
		m.commandPalette = NewCommandPalette(m.mergedCommands, m.selectedSession(), m.width, m.height, m.activeView)
		m.state = stateCommandPalette
		return m, nil
	}

	// Check for recycled placeholder selection - allow delete action
	treeItem := m.selectedTreeItem()
	if treeItem != nil && treeItem.IsRecycledPlaceholder {
		return m.handleRecycledPlaceholderKey(keyStr, treeItem)
	}

	// Handle enter on project headers - open original repo directory
	if treeItem != nil && treeItem.IsHeader && m.handler.IsCommand(keyStr, "TmuxOpen") {
		return m.openRepoHeader(treeItem)
	}

	selected := m.selectedSession()
	if selected == nil {
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		return m, cmd
	}

	// If a window sub-item is selected, override TmuxWindow for template rendering
	if treeItem != nil && treeItem.IsWindowItem {
		m.handler.SetSelectedWindow(treeItem.WindowIndex)
	} else {
		m.handler.SetSelectedWindow("")
	}

	// Check if this key maps to a form command (intercept before Resolve to
	// avoid template errors from missing .Form data)
	if cmdName, cmd, hasForm := m.handler.ResolveFormCommand(keyStr, *selected); hasForm {
		return m.showFormOrExecute(cmdName, cmd, *selected, nil)
	}

	action, ok := m.handler.Resolve(keyStr, *selected)
	action = m.maybeOverrideWindowDelete(action, treeItem)
	if ok {
		// Check for resolution errors (e.g., template errors)
		if action.Err != nil {
			return m, m.notifyError("keybinding error: %v", action.Err)
		}
		if action.NeedsConfirm() {
			m.state = stateConfirming
			m.pending = action
			m.modal = NewModal("Confirm", action.Confirm)
			return m, nil
		}
		if action.Type == act.TypeRecycle {
			return m, m.startRecycle(action.SessionID)
		}
		// Handle doc review action
		if action.Type == act.TypeDocReview {
			cmd := HiveDocReviewCmd{Arg: ""}
			return m, cmd.Execute(&m)
		}
		// Handle rename action
		if action.Type == act.TypeRenameSession {
			return m.openRenameInput(selected)
		}
		// Handle set-theme action (requires args only available via command palette)
		if action.Type == act.TypeSetTheme {
			return m, m.ensureToastTick()
		}
		// Handle filter actions
		if m.handleFilterAction(action.Type) {
			return m.applyFilter()
		}
		// If exit is requested, execute synchronously and quit immediately
		// This avoids async message flow issues in some terminal contexts (e.g., tmux popups)
		if action.Exit {
			exec, err := m.cmdService.CreateExecutor(action)
			if err != nil {
				log.Error().Str("key", keyStr).Err(err).Msg("failed to create executor before exit")
			} else if err := command.ExecuteSync(context.Background(), exec); err != nil {
				log.Error().Str("key", keyStr).Err(err).Msg("command failed before exit")
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

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

// openRepoHeader handles enter on a project header by opening the original repo directory.
func (m Model) openRepoHeader(header *TreeItem) (tea.Model, tea.Cmd) {
	// Find the original repo path from discovered repos
	var repoPath string
	for _, repo := range m.discoveredRepos {
		if repo.Remote == header.RepoRemote {
			repoPath = repo.Path
			break
		}
	}
	if repoPath == "" {
		return m, m.notifyError("no local repository found for %s", header.RepoName)
	}

	// Render the hive-tmux command to open at the repo directory
	shellCmd, err := m.renderer.Render(
		`{{ hiveTmux }} {{ .Name | shq }} {{ .Path | shq }}`,
		struct{ Name, Path string }{Name: header.RepoName, Path: repoPath},
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

// selectedSession returns the currently selected session, or nil if none.
// Returns nil for headers and recycled placeholders.
// For window sub-items, returns the parent session.
func (m Model) selectedSession() *session.Session {
	item := m.list.SelectedItem()
	if item == nil {
		return nil
	}
	// Handle TreeItem (tree view mode)
	if treeItem, ok := item.(TreeItem); ok {
		if treeItem.IsHeader || treeItem.IsRecycledPlaceholder {
			return nil // Headers and recycled placeholders aren't sessions
		}
		if treeItem.IsWindowItem {
			return &treeItem.ParentSession
		}
		return &treeItem.Session
	}
	return nil
}

// selectedWindowStatus returns the WindowStatus for the currently selected window item,
// or nil if a session (not a window) is selected.
func (m Model) selectedWindowStatus() *WindowStatus {
	item := m.list.SelectedItem()
	if item == nil {
		return nil
	}
	treeItem, ok := item.(TreeItem)
	if !ok || !treeItem.IsWindowItem {
		return nil
	}
	if m.terminalStatuses == nil {
		return nil
	}
	ts, ok := m.terminalStatuses.Get(treeItem.ParentSession.ID)
	if !ok {
		return nil
	}
	for i := range ts.Windows {
		if ts.Windows[i].WindowIndex == treeItem.WindowIndex {
			return &ts.Windows[i]
		}
	}
	return nil
}

// selectedTreeItem returns the currently selected tree item, or nil if none.
func (m Model) selectedTreeItem() *TreeItem {
	item := m.list.SelectedItem()
	if item == nil {
		return nil
	}
	if treeItem, ok := item.(TreeItem); ok {
		return &treeItem
	}
	return nil
}

// maybeOverrideWindowDelete converts a delete action into a tmux window kill
// when a window sub-item is selected. This keeps "d" context-aware.
func (m Model) maybeOverrideWindowDelete(action Action, treeItem *TreeItem) Action {
	if treeItem == nil || !treeItem.IsWindowItem {
		return action
	}
	if action.Type != act.TypeDelete {
		return action
	}

	tmuxSession := treeItem.ParentSession.GetMeta(session.MetaTmuxSession)
	if tmuxSession == "" {
		tmuxSession = treeItem.ParentSession.Slug
	}
	if tmuxSession == "" {
		tmuxSession = treeItem.ParentSession.Name
	}
	if tmuxSession == "" || treeItem.WindowIndex == "" {
		action.Err = fmt.Errorf("unable to resolve tmux window target")
		return action
	}

	target := tmuxSession + ":" + treeItem.WindowIndex
	cmd, err := m.renderer.Render("tmux kill-window -t {{ .Target | shq }}", map[string]string{
		"Target": target,
	})
	if err != nil {
		action.Err = err
		return action
	}

	action.Type = act.TypeShell
	action.ShellCmd = cmd
	if treeItem.WindowName != "" {
		action.Confirm = fmt.Sprintf("Kill tmux window %q?", treeItem.WindowName)
	} else {
		action.Confirm = "Kill tmux window?"
	}
	return action
}

// hasEditorFocus returns true if any text input currently has focus.
// When an editor has focus, most keybindings should be blocked to allow normal typing.
func (m *Model) hasEditorFocus() bool {
	// Check session list filter (built-in bubbles filter)
	if m.list.SettingFilter() {
		return true
	}

	// Check focus mode filter
	if m.focusMode {
		return true
	}

	// Check message view filter
	if m.msgView != nil && m.msgView.IsFiltering() {
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
		if m.focusMode {
			// Route to focus filter input
			var cmd tea.Cmd
			m.focusFilterInput, cmd = m.focusFilterInput.Update(msg)
			m.updateFocusFilter(m.focusFilterInput.Value())
			return m, cmd
		}
		m.list, cmd = m.list.Update(msg)
	case ViewMessages:
		// MessagesView handles its own filtering internally, no Update method
		// Key events are handled by specific methods called from handleNormalKey
		return m, nil
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

// isActiveStatus returns true for any session with a live terminal (not missing).
func isActiveStatus(s terminal.Status) bool {
	return s != terminal.StatusMissing && s != ""
}

// navigateToNextActive moves the cursor to the next session with active terminal status.
// Wraps around to the beginning if no match is found after the current position.
// For multi-window sessions, also checks per-window statuses.
func (m *Model) navigateToNextActive(direction int) {
	items := m.list.Items()
	if len(items) == 0 || m.terminalStatuses == nil {
		return
	}

	current := m.list.Index()
	n := len(items)

	for step := 1; step < n; step++ {
		idx := (current + step*direction + n) % n
		treeItem, ok := items[idx].(TreeItem)
		if !ok || treeItem.IsHeader || treeItem.IsRecycledPlaceholder {
			continue
		}

		if treeItem.IsWindowItem {
			// Check per-window status
			if ts, ok := m.terminalStatuses.Get(treeItem.ParentSession.ID); ok {
				for i := range ts.Windows {
					if ts.Windows[i].WindowIndex == treeItem.WindowIndex && isActiveStatus(ts.Windows[i].Status) {
						m.list.Select(idx)
						return
					}
				}
			}
			continue
		}

		// Check top-level session status
		if ts, ok := m.terminalStatuses.Get(treeItem.Session.ID); ok {
			if isActiveStatus(ts.Status) {
				m.list.Select(idx)
				return
			}
		}
	}
}

// navigateSkippingHeaders moves the selection by direction (-1 for up, 1 for down).
// Headers are selectable (enter opens original repo). Only recycled placeholders are skipped.
func (m *Model) navigateSkippingHeaders(direction int) {
	items := m.list.Items()
	if len(items) == 0 {
		return
	}

	current := m.list.Index()
	target := current

	for {
		target += direction

		// Check bounds
		if target < 0 || target >= len(items) {
			return // Can't move further, stay at current position
		}

		// Skip recycled placeholders, allow everything else (including headers)
		if treeItem, ok := items[target].(TreeItem); ok && treeItem.IsRecycledPlaceholder {
			continue
		}
		m.list.Select(target)
		return
	}
}

// saveSelection snapshots the current selection for restore after a list rebuild.
func (m *Model) saveSelection() treeSelection {
	return saveTreeSelection(m.selectedTreeItem(), m.list.Index())
}

// restoreSelection applies a saved selection to the current list items.
func (m *Model) restoreSelection(sel treeSelection) {
	treeItems := listItemsToTreeItems(m.list.Items())
	m.list.Select(sel.restore(treeItems))
}

// listItemsToTreeItems extracts TreeItems from list items, preserving indices.
// Non-TreeItem entries are marked as headers so restore skips them.
func listItemsToTreeItems(items []list.Item) []TreeItem {
	result := make([]TreeItem, len(items))
	for i, item := range items {
		if ti, ok := item.(TreeItem); ok {
			result[i] = ti
		} else {
			result[i] = TreeItem{IsHeader: true}
		}
	}
	return result
}

// isFilterAction returns true if the action type is a filter action.
func isFilterAction(t act.Type) bool {
	switch t {
	case act.TypeFilterAll, act.TypeFilterActive,
		act.TypeFilterApproval, act.TypeFilterReady:
		return true
	default:
		return false
	}
}

// handleFilterAction checks if the action is a filter action and updates the status filter.
// Returns true if the action was a filter action (caller should call applyFilter).
func (m *Model) handleFilterAction(actionType act.Type) bool {
	switch actionType {
	case act.TypeFilterAll:
		m.statusFilter = ""
		return true
	case act.TypeFilterActive:
		m.statusFilter = terminal.StatusActive
		return true
	case act.TypeFilterApproval:
		m.statusFilter = terminal.StatusApproval
		return true
	case act.TypeFilterReady:
		m.statusFilter = terminal.StatusReady
		return true
	default:
		return false
	}
}

// selectedMessage returns the currently selected message, or nil if none.
func (m Model) selectedMessage() *messaging.Message {
	return m.msgView.SelectedMessage()
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

// shouldPollMessages returns true if messages should be polled.
func (m Model) shouldPollMessages() bool {
	return m.activeView == ViewMessages
}

// isModalActive returns true if any modal is currently open.
func (m Model) isModalActive() bool {
	return m.state != stateNormal
}

// stopFocusMode deactivates focus mode filtering.
func (m *Model) stopFocusMode() {
	m.focusMode = false
	m.focusFilter = ""

	// Restore list size and show help
	contentHeight := m.height - 3
	if contentHeight < 1 {
		contentHeight = 1
	}

	var listWidth int
	if m.previewEnabled && m.width >= 80 && m.activeView == ViewSessions {
		listWidth = int(float64(m.width) * 0.25)
	} else {
		listWidth = m.width
	}

	m.list.SetSize(listWidth, contentHeight)
	m.list.SetShowHelp(true)
}

// updateFocusFilter updates the filter and navigates to first match.
func (m *Model) updateFocusFilter(filter string) {
	m.focusFilter = filter
	if filter == "" {
		return // no navigation with empty filter
	}

	// Find first matching item
	filterLower := strings.ToLower(filter)

	for i, ti := range TreeItemsAll(m.list.Items()) {
		if ti.IsHeader {
			continue
		}

		filterValue := strings.ToLower(ti.FilterValue())
		if strings.Contains(filterValue, filterLower) {
			m.list.Select(i)
			return
		}
	}

	// No match found - cursor stays at current position
}

// applyFilter rebuilds the tree view from all sessions.
func (m Model) applyFilter() (tea.Model, tea.Cmd) {
	sel := m.saveSelection()

	// Filter sessions by terminal status if a filter is active
	sessions := m.allSessions
	if m.statusFilter != "" && m.terminalStatuses != nil {
		filtered := make([]session.Session, 0, len(m.allSessions))
		for _, s := range m.allSessions {
			if status, ok := m.terminalStatuses.Get(s.ID); ok {
				if status.Status == m.statusFilter {
					filtered = append(filtered, s)
				}
			}
		}
		sessions = filtered
	}

	// Group sessions by repository and build tree items
	groups := GroupSessionsByRepo(sessions, m.localRemote)
	items := BuildTreeItems(groups, m.localRemote)

	// Expand multi-window sessions into window sub-items
	items = m.expandWindowItems(items)

	// Calculate column widths across all sessions (use filtered set)
	*m.columnWidths = CalculateColumnWidths(sessions, nil)

	// Collect paths for git status fetching (use filtered sessions)
	// During background refresh, keep existing statuses to avoid flashing
	paths := make([]string, 0, len(sessions))
	for _, s := range sessions {
		paths = append(paths, s.Path)
		if !m.refreshing {
			m.gitStatuses.Set(s.Path, GitStatus{IsLoading: true})
		}
	}

	m.list.SetItems(items)
	m.restoreSelection(sel)
	m.state = stateNormal

	if len(paths) == 0 {
		m.refreshing = false
		return m, nil
	}
	// refreshing is cleared when gitStatusBatchCompleteMsg is received
	return m, fetchGitStatusBatch(m.service.Git(), paths, m.gitWorkers)
}

// rebuildWindowItems strips existing window sub-items from the list and re-expands
// based on current terminal statuses. Preserves the current selection.
func (m *Model) rebuildWindowItems() {
	items := m.list.Items()

	// Build sets of current and expected windows keyed by "sessionID:windowIndex:windowName"
	// so window renames trigger a rebuild.
	current := make(map[string]struct{})
	expected := make(map[string]struct{})
	for _, ti := range TreeItemsAll(items) {
		if ti.IsWindowItem {
			current[ti.ParentSession.ID+"\x1f"+ti.WindowIndex+"\x1f"+ti.WindowName] = struct{}{}
			continue
		}
		if !ti.IsSession() {
			continue
		}
		if ts, ok := m.terminalStatuses.Get(ti.Session.ID); ok && len(ts.Windows) > 1 {
			for _, w := range ts.Windows {
				expected[ti.Session.ID+"\x1f"+w.WindowIndex+"\x1f"+w.WindowName] = struct{}{}
			}
		}
	}

	// Only rebuild if the window sets actually differ.
	if len(current) == len(expected) {
		same := true
		for k := range current {
			if _, ok := expected[k]; !ok {
				same = false
				break
			}
		}
		if same {
			return
		}
	}

	sel := m.saveSelection()

	// Strip window items
	stripped := make([]list.Item, 0, len(items))
	for i, ti := range TreeItemsAll(items) {
		if ti.IsWindowItem {
			continue
		}
		stripped = append(stripped, items[i])
	}

	// Re-expand
	expanded := m.expandWindowItems(stripped)
	m.list.SetItems(expanded)
	m.restoreSelection(sel)
}

// expandWindowItems inserts window sub-items after each session that has multiple
// terminal windows. Single-window sessions are left unchanged.
func (m Model) expandWindowItems(items []list.Item) []list.Item {
	if m.terminalStatuses == nil {
		return items
	}

	expanded := make([]list.Item, 0, len(items))
	for _, item := range items {
		expanded = append(expanded, item)

		treeItem, ok := item.(TreeItem)
		if !ok || !treeItem.IsSession() {
			continue
		}

		ts, ok := m.terminalStatuses.Get(treeItem.Session.ID)
		if !ok || len(ts.Windows) <= 1 {
			continue
		}

		// Add a window sub-item for each window
		for i, w := range ts.Windows {
			windowItem := TreeItem{
				IsWindowItem:  true,
				WindowIndex:   w.WindowIndex,
				WindowName:    w.WindowName,
				ParentSession: treeItem.Session,
				IsLastWindow:  i == len(ts.Windows)-1,
				IsLastInRepo:  treeItem.IsLastInRepo,
				RepoPrefix:    treeItem.RepoPrefix,
			}
			expanded = append(expanded, windowItem)
		}
	}

	return expanded
}

// refreshGitStatuses returns a command that refreshes git status for all sessions.
func (m Model) refreshGitStatuses() tea.Cmd {
	items := m.list.Items()
	paths := make([]string, 0, len(items))

	for _, ti := range TreeItemsSessions(items) {
		paths = append(paths, ti.Session.Path)
		m.gitStatuses.Set(ti.Session.Path, GitStatus{IsLoading: true})
	}

	if len(paths) == 0 {
		return nil
	}

	return fetchGitStatusBatch(m.service.Git(), paths, m.gitWorkers)
}

// View renders the TUI.
func (m Model) View() tea.View {
	if m.quitting {
		return tea.NewView("")
	}

	// Build main view (header + content)
	mainView := m.renderTabView()

	// Ensure we have dimensions for modals
	w, h := m.width, m.height
	if w == 0 {
		w = 80
	}
	if h == 0 {
		h = 24
	}

	// Determine overlay content based on state
	var content string
	switch {
	case m.state == stateRunningRecycle:
		content = m.outputModal.Overlay(mainView, w, h)
	case m.state == stateCreatingSession && m.newSessionForm != nil:
		formContent := lipgloss.JoinVertical(
			lipgloss.Left,
			styles.ModalTitleStyle.Render("New Session"),
			"",
			m.newSessionForm.View(),
		)
		formOverlay := styles.ModalStyle.Render(formContent)

		bgLayer := lipgloss.NewLayer(mainView)
		formLayer := lipgloss.NewLayer(formOverlay)
		formW := lipgloss.Width(formOverlay)
		formH := lipgloss.Height(formOverlay)
		centerX := (w - formW) / 2
		centerY := (h - formH) / 2
		formLayer.X(centerX).Y(centerY).Z(1)

		compositor := lipgloss.NewCompositor(bgLayer, formLayer)
		content = compositor.Render()
	case m.state == stateFormInput && m.formDialog != nil:
		formContent := lipgloss.JoinVertical(
			lipgloss.Left,
			styles.ModalTitleStyle.Render(m.formDialog.Title),
			"",
			m.formDialog.View(),
		)
		formOverlay := styles.FormModalStyle.Render(formContent)

		bgLayer := lipgloss.NewLayer(mainView)
		formLayer := lipgloss.NewLayer(formOverlay)
		formW := lipgloss.Width(formOverlay)
		formH := lipgloss.Height(formOverlay)
		centerX := (w - formW) / 2
		centerY := (h - formH) / 2
		formLayer.X(centerX).Y(centerY).Z(1)

		compositor := lipgloss.NewCompositor(bgLayer, formLayer)
		content = compositor.Render()
	case m.state == statePreviewingMessage:
		content = m.previewModal.Overlay(mainView, w, h)
	case m.state == stateLoading:
		loadingView := lipgloss.JoinHorizontal(lipgloss.Left, m.spinner.View(), " "+m.loadingMessage)
		modal := NewModal("", loadingView)
		content = modal.Overlay(mainView, w, h)
	case m.state == stateConfirming:
		content = m.modal.Overlay(mainView, w, h)
	case m.state == stateCommandPalette && m.commandPalette != nil:
		content = m.commandPalette.Overlay(mainView, w, h)
	case m.state == stateShowingHelp && m.helpDialog != nil:
		content = m.helpDialog.Overlay(mainView, w, h)
	case m.state == stateShowingNotifications && m.notificationModal != nil:
		content = m.notificationModal.Overlay(mainView, w, h)
	case m.state == stateRenaming:
		renameContent := lipgloss.JoinVertical(
			lipgloss.Left,
			styles.ModalTitleStyle.Render("Rename Session"),
			"",
			m.renameInput.View(),
			"",
			styles.ModalHelpStyle.Render("enter: confirm • esc: cancel"),
		)
		renameOverlay := styles.ModalStyle.Width(50).Render(renameContent)
		bgLayer := lipgloss.NewLayer(mainView)
		renameLayer := lipgloss.NewLayer(renameOverlay)
		rW := lipgloss.Width(renameOverlay)
		rH := lipgloss.Height(renameOverlay)
		renameLayer.X((w - rW) / 2).Y((h - rH) / 2).Z(1)
		compositor := lipgloss.NewCompositor(bgLayer, renameLayer)
		content = compositor.Render()
	case m.docPickerModal != nil:
		content = m.docPickerModal.Overlay(mainView, w, h)
	default:
		content = mainView
	}

	// Apply toast overlay on top of everything
	if m.toastController.HasToasts() {
		content = m.toastView.Overlay(content, w, h)
	}

	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

// renderTabView renders the tab-based view layout.
func (m Model) renderTabView() string {
	// Build tab bar with tabs on left and branding on right
	showReviewTab := m.reviewView != nil && m.reviewView.CanShowInTabBar()
	showStoreTab := m.kvStore != nil && m.cfg.TUI.Views.Store

	// Render each tab with appropriate style
	renderTab := func(label string, view ViewType) string {
		if m.activeView == view {
			return styles.ViewSelectedStyle.Render(label)
		}
		return styles.ViewNormalStyle.Render(label)
	}

	// Build tab list
	tabs := []string{
		renderTab("Sessions", ViewSessions),
		renderTab("Messages", ViewMessages),
	}
	if showStoreTab || m.activeView == ViewStore {
		tabs = append(tabs, renderTab("Store", ViewStore))
	}
	if showReviewTab || m.activeView == ViewReview {
		tabs = append(tabs, renderTab("Review", ViewReview))
	}

	tabsLeft := strings.Join(tabs, " | ")

	// Add filter indicator if active
	if m.statusFilter != "" {
		filterLabel := string(m.statusFilter)
		tabsLeft = lipgloss.JoinHorizontal(lipgloss.Left, tabsLeft, "  ", styles.TextPrimaryBoldStyle.Render("["+filterLabel+"]"))
	}

	// Branding on right with background
	branding := styles.TabBrandingStyle.Render(styles.IconHive + " Hive")

	// Calculate spacing to push branding to right edge with even margins
	// Layout: [margin] tabs [spacer] branding [margin]
	margin := 1
	tabsWidth := lipgloss.Width(tabsLeft)
	brandingWidth := lipgloss.Width(branding)
	spacerWidth := max(m.width-tabsWidth-brandingWidth-(margin*2), 1)
	leftMargin := components.Pad(margin)
	spacer := components.Pad(spacerWidth)
	rightMargin := components.Pad(margin)

	header := lipgloss.JoinHorizontal(lipgloss.Left, leftMargin, tabsLeft, spacer, branding, rightMargin)

	// Horizontal dividers above and below header
	dividerWidth := m.width
	if dividerWidth < 1 {
		dividerWidth = 80 // default width before WindowSizeMsg
	}
	topDivider := styles.TextMutedStyle.Render(strings.Repeat("─", dividerWidth))
	headerDivider := styles.TextMutedStyle.Render(strings.Repeat("─", dividerWidth))

	// Calculate content height: total - top divider (1) - header (1) - bottom divider (1)
	contentHeight := max(m.height-3, 1)

	// Build content with fixed height to prevent layout shift
	var content string
	switch m.activeView {
	case ViewSessions:
		// Check if preview should be shown
		if m.previewEnabled && m.width >= 80 {
			content = m.renderDualColumnLayout(contentHeight)
		} else {
			// Reset delegate to show full info when not in preview mode
			m.treeDelegate.PreviewMode = false
			m.list.SetDelegate(m.treeDelegate)
			content = m.list.View()

			// Show focus mode filter input if active (at bottom to avoid layout shift)
			if m.focusMode {
				content = lipgloss.JoinVertical(lipgloss.Left, content, m.focusFilterInput.View())
			} else if m.list.SettingFilter() {
				// Fallback: show bubbles built-in filter if somehow active
				content = lipgloss.JoinVertical(lipgloss.Left, m.list.FilterInput.View(), content)
			}

			content = lipgloss.NewStyle().Height(contentHeight).Render(content)
		}
	case ViewMessages:
		content = m.msgView.View()
		content = lipgloss.NewStyle().Height(contentHeight).Render(content)
	case ViewStore:
		content = m.kvView.View()
		content = lipgloss.NewStyle().Height(contentHeight).Render(content)
	case ViewReview:
		if m.reviewView != nil {
			content = m.reviewView.View()
			content = lipgloss.NewStyle().Height(contentHeight).Render(content)
		}
	}

	return lipgloss.JoinVertical(lipgloss.Left, topDivider, header, headerDivider, content)
}

// renderDualColumnLayout renders sessions list and preview side by side.
func (m Model) renderDualColumnLayout(contentHeight int) string {
	// Update delegate to show minimal info in preview mode
	m.treeDelegate.PreviewMode = true
	m.list.SetDelegate(m.treeDelegate)

	// Calculate widths (25% list, 1 char divider, remaining for preview)
	listWidth := int(float64(m.width) * 0.25)
	if listWidth < 20 {
		listWidth = 20
	}

	// Account for divider (1 char) between list and preview
	dividerWidth := 1
	previewWidth := m.width - listWidth - dividerWidth

	// Get selected session and its terminal status
	selected := m.selectedSession()
	var previewContent string

	if selected != nil {
		// Check if this is the current session (would cause recursive preview)
		isSelf := m.isCurrentTmuxSession(selected)

		// Determine pane content: use per-window content if a window item is selected,
		// otherwise fall back to session-level content.
		var paneContent string
		if ws := m.selectedWindowStatus(); ws != nil {
			paneContent = ws.PaneContent
		} else if status, ok := m.terminalStatuses.Get(selected.ID); ok {
			paneContent = status.PaneContent
		}

		switch {
		case isSelf:
			// Show placeholder instead of recursive preview
			previewContent = m.renderPreviewHeader(selected, previewWidth-4) + "\n\n(current session, preventing recursive view)"
		case paneContent != "":
			// Account for padding: 2 chars on each side = 4 total
			usableWidth := previewWidth - 4

			// Build header
			header := m.renderPreviewHeader(selected, usableWidth)
			headerHeight := strings.Count(header, "\n") + 1

			// Calculate available lines for content
			outputHeight := max(contentHeight-headerHeight, 1)

			// Get content and truncate to width
			content := tailLines(paneContent, outputHeight)
			content = truncateLines(content, usableWidth)

			previewContent = header + "\n" + content
		default:
			previewContent = m.renderPreviewHeader(selected, previewWidth-4) + "\n\nNo pane content available"
		}
	} else {
		previewContent = "No session selected"
	}

	// Render list
	listView := m.list.View()
	if m.focusMode {
		listView = lipgloss.JoinVertical(lipgloss.Left, listView, m.focusFilterInput.View())
	} else if m.list.SettingFilter() {
		listView = lipgloss.JoinVertical(lipgloss.Left, m.list.FilterInput.View(), listView)
	}

	// Apply exact height to both panels
	listView = ensureExactHeight(listView, contentHeight)
	previewContent = ensureExactHeight(previewContent, contentHeight)

	// Apply exact width to list view to prevent bleeding into preview
	// The bubble tea list should already render at listWidth from SetSize,
	// but we enforce it here to ensure clean horizontal joining
	listView = ensureExactWidth(listView, listWidth)

	// Apply padding and exact width to preview content
	previewLines := strings.Split(previewContent, "\n")
	for i, line := range previewLines {
		previewLines[i] = "  " + line + "  "
	}
	previewContent = strings.Join(previewLines, "\n")
	previewContent = ensureExactWidth(previewContent, previewWidth)
	previewContent = styles.TextForegroundStyle.Render(previewContent)

	// Create vertical divider between list and preview
	dividerLines := make([]string, contentHeight)
	for i := range dividerLines {
		dividerLines[i] = styles.TextMutedStyle.Render("│")
	}
	divider := strings.Join(dividerLines, "\n")

	// Join horizontally - all three panels have exact matching heights
	return lipgloss.JoinHorizontal(lipgloss.Top, listView, divider, previewContent)
}

// renderPreviewHeader renders the preview header section with session metadata.
func (m Model) renderPreviewHeader(sess *session.Session, maxWidth int) string {
	iconsEnabled := m.cfg.TUI.IconsEnabled()

	// Styles
	nameStyle := styles.PreviewHeaderNameStyle
	separatorStyle := styles.TextMutedStyle
	idStyle := styles.TextSecondaryStyle
	branchStyle := styles.TextSecondaryStyle
	addStyle := styles.TextSuccessStyle
	delStyle := styles.TextErrorStyle
	dirtyStyle := styles.TextWarningStyle
	dividerStyle := styles.TextMutedStyle

	divider := strings.Repeat("─", maxWidth)

	// Build title line: "SessionName [window] • #abcd"
	shortID := sess.ID
	if len(shortID) > 4 {
		shortID = shortID[len(shortID)-4:]
	}
	title := nameStyle.Render(sess.Name)
	// Show window name if a specific window is selected
	if ws := m.selectedWindowStatus(); ws != nil {
		title += " " + styles.TextSecondaryStyle.Render("["+ws.WindowName+"]")
	}
	title += separatorStyle.Render(" • ") + idStyle.Render("#"+shortID)

	// Build status line with colors
	var statusParts []string

	// Git status
	if m.gitStatuses != nil {
		if status, ok := m.gitStatuses.Get(sess.Path); ok && !status.IsLoading && status.Error == nil {
			gitPart := branchStyle.Render("(")
			if iconsEnabled {
				gitPart += branchStyle.Render(styles.IconGitBranch + " ")
			}
			gitPart += branchStyle.Render(status.Branch + ")")
			gitPart += " " + addStyle.Render("+"+itoa(status.Additions))
			gitPart += " " + delStyle.Render("-"+itoa(status.Deletions))
			if status.HasChanges && iconsEnabled {
				gitPart += " " + dirtyStyle.Render(styles.IconGit)
			}
			statusParts = append(statusParts, gitPart)
		}
	}

	// Plugin statuses (neutral color)
	if m.pluginStatuses != nil {
		pluginOrder := []string{pluginGitHub, pluginBeads, pluginClaude}
		for _, name := range pluginOrder {
			store, ok := m.pluginStatuses[name]
			if !ok || store == nil {
				continue
			}
			status, ok := store.Get(sess.ID)
			if !ok || status.Label == "" {
				continue
			}

			var icon string
			if iconsEnabled {
				switch name {
				case pluginGitHub:
					icon = styles.IconGithub
				case pluginBeads:
					icon = styles.IconCheckList
				case pluginClaude:
					icon = styles.IconBrain
				}
			} else {
				icon = status.Icon
			}

			// Icon unstyled, only the label gets neutral color
			pluginPart := icon + separatorStyle.Render(status.Label)
			statusParts = append(statusParts, pluginPart)
		}
	}

	status := strings.Join(statusParts, separatorStyle.Render(" • "))

	// Build header
	var parts []string
	parts = append(parts, title)
	parts = append(parts, dividerStyle.Render(divider))
	if status != "" {
		parts = append(parts, status)
	}
	parts = append(parts, "")
	parts = append(parts, styles.TextMutedStyle.Render("Output"))
	parts = append(parts, dividerStyle.Render(divider))

	return strings.Join(parts, "\n")
}

// itoa converts an int to a string without importing strconv.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	if n < 0 {
		return "-" + itoa(-n)
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

// tailLines returns the last n lines from the input string.
func tailLines(s string, n int) string {
	if n <= 0 {
		return ""
	}
	lines := strings.Split(s, "\n")
	if len(lines) <= n {
		return s
	}
	return strings.Join(lines[len(lines)-n:], "\n")
}

// truncateLines truncates each line to fit within maxWidth visual characters.
// Uses wcwidth-based truncation to properly handle ANSI codes and multi-byte characters.
func truncateLines(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return s
	}

	lines := strings.Split(s, "\n")
	sb := builderPool.Get().(*strings.Builder)
	defer func() {
		sb.Reset()
		builderPool.Put(sb)
	}()

	for i, line := range lines {
		if i > 0 {
			sb.WriteByte('\n')
		}
		if ansi.StringWidth(line) > maxWidth {
			sb.WriteString(ansi.TruncateWc(line, maxWidth, ""))
		} else {
			sb.WriteString(line)
		}
	}

	return sb.String()
}

// ensureExactWidth ensures all lines in content have exactly the specified width
// by padding short lines with spaces or truncating long lines at the boundary.
// This is critical for lipgloss.JoinHorizontal to work correctly.
func ensureExactWidth(content string, width int) string {
	if width <= 0 {
		return content
	}

	lines := strings.Split(content, "\n")
	sb := builderPool.Get().(*strings.Builder)
	defer func() {
		sb.Reset()
		builderPool.Put(sb)
	}()

	for i, line := range lines {
		if i > 0 {
			sb.WriteByte('\n')
		}

		displayWidth := ansi.StringWidth(line)

		switch {
		case displayWidth == width:
			sb.WriteString(line)
		case displayWidth < width:
			// Pad with spaces to reach target width using cached padding
			sb.WriteString(line)
			sb.WriteString(components.Pad(width - displayWidth))
		default:
			// Line too wide - truncate at width boundary
			truncated := ansi.TruncateWc(line, width, "")
			sb.WriteString(truncated)
			// Pad if truncation made it shorter than width
			truncWidth := ansi.StringWidth(truncated)
			if truncWidth < width {
				sb.WriteString(components.Pad(width - truncWidth))
			}
		}
	}

	return sb.String()
}

// ensureExactHeight ensures content has exactly n lines by truncating or padding.
func ensureExactHeight(content string, n int) string {
	if n <= 0 {
		return ""
	}

	lines := strings.Split(content, "\n")

	if len(lines) > n {
		lines = lines[:n]
	} else {
		for len(lines) < n {
			lines = append(lines, "")
		}
	}

	return strings.Join(lines, "\n")
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
	m.treeDelegate.Styles = DefaultTreeDelegateStyles()
	m.list.SetDelegate(m.treeDelegate)
	// Clear cached animation colors so they regenerate from new theme
	activeAnimationColors = nil
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
