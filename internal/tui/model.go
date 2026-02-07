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

	"github.com/hay-kot/hive/internal/core/config"
	"github.com/hay-kot/hive/internal/core/git"
	"github.com/hay-kot/hive/internal/core/messaging"
	"github.com/hay-kot/hive/internal/core/session"
	"github.com/hay-kot/hive/internal/data/db"
	"github.com/hay-kot/hive/internal/hive"
	"github.com/hay-kot/hive/internal/integration/terminal"
	"github.com/hay-kot/hive/internal/plugins"
	"github.com/hay-kot/hive/internal/stores"
	"github.com/hay-kot/hive/internal/styles"
	"github.com/hay-kot/hive/internal/tui/command"
	"github.com/hay-kot/hive/internal/tui/components"
	"github.com/hay-kot/hive/internal/tui/views/review"
	"github.com/hay-kot/hive/pkg/kv"
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
)

// Key constants for event handling.
const (
	keyEnter = "enter"
	keyCtrlC = "ctrl+c"
)

// Options configures the TUI behavior.
type Options struct {
	LocalRemote     string            // Remote URL of current directory (empty if not in git repo)
	MsgStore        messaging.Store   // Message store for pub/sub events (optional)
	TerminalManager *terminal.Manager // Terminal integration manager (optional)
	PluginManager   *plugins.Manager  // Plugin manager (optional)
	DB              *db.DB            // Database connection for stores
}

// PendingCreate holds data for a session to create after TUI exits.
type PendingCreate struct {
	Remote string
	Name   string
}

// Model is the main Bubble Tea model for the TUI.
type Model struct {
	cfg            *config.Config
	service        *hive.Service
	cmdService     *command.Service
	list           list.Model
	handler        *KeybindingResolver
	state          UIState
	modal          Modal
	pending        Action
	width          int
	height         int
	err            error
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

	// Recycle streaming state
	outputModal   OutputModal
	recycleOutput <-chan string
	recycleDone   <-chan error
	recycleCancel context.CancelFunc

	// Layout
	activeView ViewType // which view is shown
	refreshing bool     // true during background session refresh

	// Messages
	msgStore     messaging.Store
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

	// Pending action for after TUI exits
	pendingCreate *PendingCreate

	// Pending recycled sessions for batch delete
	pendingRecycledSessions []session.Session

	// Review view
	reviewView *review.View

	// Document picker (shown on Sessions view to start reviews)
	docPickerModal *review.DocumentPickerModal
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

// New creates a new TUI model.
func New(service *hive.Service, cfg *config.Config, opts Options) Model {
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
	filterStyles.Focused.Prompt = lipgloss.NewStyle().PaddingLeft(1).Foreground(lipgloss.Color("#7aa2f7")).Bold(true)
	filterStyles.Cursor.Color = lipgloss.Color("#7aa2f7")
	l.FilterInput.SetStyles(filterStyles)

	// Style help to match messages view (consistent gray, bullet separators, left padding)
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#565f89"))
	l.Help.Styles.ShortKey = helpStyle
	l.Help.Styles.ShortDesc = helpStyle
	l.Help.Styles.ShortSeparator = helpStyle
	l.Help.Styles.FullKey = helpStyle
	l.Help.Styles.FullDesc = helpStyle
	l.Help.Styles.FullSeparator = helpStyle
	l.Help.ShortSeparator = " • "
	l.Styles.HelpStyle = lipgloss.NewStyle().PaddingLeft(1)

	// Compute merged commands: system → plugins → user
	var mergedCommands map[string]config.UserCommand
	if opts.PluginManager != nil {
		mergedCommands = opts.PluginManager.MergedCommands(config.DefaultUserCommands(), cfg.UserCommands)
	} else {
		mergedCommands = cfg.MergedUserCommands()
	}

	handler := NewKeybindingResolver(cfg.Keybindings, mergedCommands)
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
	cmdService := command.NewService(service, service)

	// Add minimal keybindings to list help - just navigation and help trigger
	l.AdditionalShortHelpKeys = func() []key.Binding {
		return []key.Binding{
			key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑/↓", "navigate")),
			key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		}
	}

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#7aa2f7")) // blue, lipgloss v1 for bubbles v1

	// Create message view
	msgView := NewMessagesView()

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

	reviewView := review.New(docs, contextDir, reviewStore)

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

// Init initializes the model.
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.loadSessions(), m.spinner.Tick}
	// Start message polling if we have a store
	if m.msgStore != nil {
		cmds = append(cmds, loadMessages(m.msgStore, m.topicFilter, time.Time{}))
		cmds = append(cmds, schedulePollTick())
	}
	// Start session refresh timer
	if cmd := m.scheduleSessionRefresh(); cmd != nil {
		cmds = append(cmds, cmd)
	}
	// Scan for repositories if configured
	if len(m.repoDirs) > 0 {
		cmds = append(cmds, m.scanRepoDirs())
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
func (m Model) executeAction(action Action) tea.Cmd {
	return func() tea.Msg {
		cmdAction := command.Action{
			Type:      command.ActionType(action.Type),
			SessionID: action.SessionID,
			ShellCmd:  action.ShellCmd,
		}

		exec, err := m.cmdService.CreateExecutor(cmdAction)
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
	case tea.WindowSizeMsg:
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
		m.msgView.SetSize(msg.Width, contentHeight-1)

		// Set review view size
		if m.reviewView != nil {
			m.reviewView.SetSize(msg.Width, contentHeight)
		}
		return m, nil

	case messagesLoadedMsg:
		if msg.err != nil {
			// Silently ignore message loading errors
			return m, nil
		}
		// Append new messages if any
		if len(msg.messages) > 0 {
			m.allMessages = append(m.allMessages, msg.messages...)
			// Update message view with reversed order (newest first)
			reversed := make([]messaging.Message, len(m.allMessages))
			for i, message := range m.allMessages {
				reversed[len(m.allMessages)-1-i] = message
			}
			m.msgView.SetMessages(reversed)
		}
		// Always update poll time so we don't re-fetch the same messages
		m.lastPollTime = time.Now()
		return m, nil

	case pollTickMsg:
		// Only poll if messages are visible
		if m.shouldPollMessages() && m.msgStore != nil {
			return m, tea.Batch(
				loadMessages(m.msgStore, m.topicFilter, m.lastPollTime),
				schedulePollTick(),
			)
		}
		// Keep scheduling poll ticks even if not actively polling
		return m, schedulePollTick()

	case sessionRefreshTickMsg:
		// Refresh sessions when Sessions view is active and no modal open
		if m.activeView == ViewSessions && !m.isModalActive() {
			m.refreshing = true
			return m, tea.Batch(
				m.loadSessions(),
				m.scheduleSessionRefresh(),
			)
		}
		// Keep scheduling refresh ticks even if not actively refreshing
		return m, m.scheduleSessionRefresh()

	case sessionsLoadedMsg:
		if msg.err != nil {
			log.Error().Err(msg.err).Msg("failed to load sessions")
			m.err = msg.err
			m.state = stateNormal
			return m, nil
		}
		// Store all sessions for filtering
		m.allSessions = msg.sessions
		// Apply filter and update list
		filteredModel, cmd := m.applyFilter()
		// Update plugin manager with new sessions (triggers background refresh)
		if m.pluginManager != nil && len(m.pluginStatuses) > 0 {
			sessions := make([]*session.Session, len(m.allSessions))
			for i := range m.allSessions {
				sessions[i] = &m.allSessions[i]
			}
			m.pluginManager.UpdateSessions(sessions)
			log.Debug().Int("sessionCount", len(sessions)).Msg("updated plugin manager sessions")
		}
		return filteredModel, cmd

	case gitStatusBatchCompleteMsg:
		m.gitStatuses.SetBatch(msg.Results)
		m.refreshing = false
		return m, nil

	case terminalPollTickMsg:
		// Start next poll cycle
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

	case terminalStatusBatchCompleteMsg:
		if m.terminalStatuses != nil {
			m.terminalStatuses.SetBatch(msg.Results)
			// Re-expand window items if multi-window sessions appeared or changed
			m.rebuildWindowItems()
		}
		return m, nil

	case pluginWorkerStartedMsg:
		// Store the channel and start listening for results
		m.pluginResultsChan = msg.resultsChan
		log.Debug().Msg("plugin background worker started")
		return m, listenForPluginResult(m.pluginResultsChan)

	case pluginStatusUpdateMsg:
		// Update the specific plugin's status store with the single result
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
		// Update delegate's reference to plugin statuses
		m.treeDelegate.PluginStatuses = m.pluginStatuses
		// Force list to re-render with updated plugin status
		m.list.SetDelegate(m.treeDelegate)
		// Continue listening for more results
		return m, listenForPluginResult(m.pluginResultsChan)

	case animationTickMsg:
		// Advance animation frame
		m.animationFrame = (m.animationFrame + 1) % AnimationFrameCount
		// Update the delegate with new frame
		m.treeDelegate.AnimationFrame = m.animationFrame
		m.list.SetDelegate(m.treeDelegate)
		// Schedule next tick
		return m, scheduleAnimationTick()

	case actionCompleteMsg:
		if msg.err != nil {
			log.Error().Err(msg.err).Msg("action failed")
			m.err = msg.err
			m.state = stateNormal
			m.pending = Action{}
			return m, nil
		}
		m.state = stateNormal
		m.pending = Action{}
		// Reload sessions after action
		return m, m.loadSessions()

	case recycleStartedMsg:
		m.state = stateRunningRecycle
		m.outputModal = NewOutputModal("Recycling session...")
		m.recycleOutput = msg.output
		m.recycleDone = msg.done
		m.recycleCancel = msg.cancel
		return m, tea.Batch(
			listenForRecycleOutput(msg.output, msg.done),
			m.outputModal.Spinner().Tick,
		)

	case recycleOutputMsg:
		m.outputModal.AddLine(msg.line)
		// Keep listening for more output
		return m, listenForRecycleOutput(m.recycleOutput, m.recycleDone)

	case recycleCompleteMsg:
		m.outputModal.SetComplete(msg.err)
		m.recycleOutput = nil
		m.recycleDone = nil
		m.recycleCancel = nil
		// Stay in stateRunningRecycle until user dismisses
		return m, nil

	case reposDiscoveredMsg:
		m.discoveredRepos = msg.repos
		// Help keybindings remain minimal - full list shown via ? dialog
		return m, nil

	case review.DocumentChangeMsg:
		// Forward to review view if it's active
		if m.reviewView != nil {
			*m.reviewView, _ = m.reviewView.Update(msg)
		}
		return m, nil

	case review.ReviewFinalizedMsg:
		// Copy feedback to clipboard
		if err := m.copyToClipboard(msg.Feedback); err != nil {
			m.err = fmt.Errorf("failed to copy feedback: %w", err)
		} else {
			m.err = nil // Clear any previous errors
		}
		return m, nil

	case review.OpenDocumentMsg:
		// Handle document opening (from HiveDocReview command)
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		// Document path is provided, tell review view to load it
		if m.reviewView != nil {
			// Find and load the document
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

	case tea.KeyMsg:
		// Handle document picker modal if active (on Sessions view)
		if m.docPickerModal != nil {
			modal, cmd := m.docPickerModal.Update(msg)
			m.docPickerModal = modal

			if m.docPickerModal.SelectedDocument() != nil {
				// User selected a document - switch to review view and load it
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
				// User cancelled picker
				m.docPickerModal = nil
				return m, cmd
			}

			return m, cmd
		}

		return m.handleKey(msg)

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	// Route all other messages to the form when creating session
	if m.state == stateCreatingSession && m.newSessionForm != nil {
		return m.updateNewSessionForm(msg)
	}

	// Update the appropriate view based on active view
	var cmd tea.Cmd
	switch m.activeView {
	case ViewSessions:
		m.list, cmd = m.list.Update(msg)
	case ViewMessages:
		// Messages view handles its own updates
	case ViewReview:
		if m.reviewView != nil {
			*m.reviewView, cmd = m.reviewView.Update(msg)
		}
	}
	return m, cmd
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
	if m.state == stateRunningRecycle {
		return m.handleRecycleModalKey(keyStr)
	}
	if m.state == stateConfirming {
		return m.handleConfirmModalKey(keyStr)
	}

	// When filtering in either list, pass most keys except quit
	if m.list.SettingFilter() || m.msgView.IsFiltering() {
		return m.handleFilteringKey(msg, keyStr)
	}

	// Handle normal state
	return m.handleNormalKey(msg, keyStr)
}

// handleNewSessionFormKey handles keys when new session form is shown.
func (m Model) handleNewSessionFormKey(msg tea.KeyMsg, keyStr string) (tea.Model, tea.Cmd) {
	if keyStr == keyCtrlC {
		m.quitting = true
		return m, tea.Quit
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

// handleRecycleModalKey handles keys when recycle modal is shown.
func (m Model) handleRecycleModalKey(keyStr string) (tea.Model, tea.Cmd) {
	switch keyStr {
	case keyCtrlC:
		if m.recycleCancel != nil {
			m.recycleCancel()
		}
		m.quitting = true
		return m, tea.Quit
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
			if action.Type == ActionTypeRecycle {
				return m, m.startRecycle(action.SessionID)
			}
			// Handle batch delete of recycled sessions
			if action.Type == ActionTypeDeleteRecycledBatch {
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
	if !cmdExists || cmd.Action != config.ActionDelete {
		return m, nil // Only delete is allowed on recycled placeholders
	}

	// Show confirmation modal for deleting all recycled sessions
	confirmMsg := fmt.Sprintf("Permanently delete %d recycled session(s)?", treeItem.RecycledCount)
	m.state = stateConfirming
	m.pending = Action{
		Type:    ActionTypeDeleteRecycledBatch,
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
		m.quitting = true
		return m, tea.Quit
	case "esc", "?", "q":
		m.state = stateNormal
		m.helpDialog = nil
		return m, nil
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
		{Key: "enter", Desc: "select session"},
		{Key: "/", Desc: "filter"},
		{Key: "tab", Desc: "switch view"},
		{Key: ":", Desc: "command palette"},
		{Key: "g", Desc: "refresh git status"},
	}

	// Add new session if repos discovered
	if len(m.discoveredRepos) > 0 {
		navEntries = append(navEntries, components.HelpEntry{Key: "n", Desc: "new session"})
	}

	// Add preview toggle if terminal integration enabled
	if m.terminalManager != nil && m.terminalManager.HasEnabledIntegrations() {
		navEntries = append(navEntries, components.HelpEntry{Key: "v", Desc: "toggle preview"})
	}

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
		m.quitting = true
		return m, tea.Quit
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
		m.quitting = true
		return m, tea.Quit
	}

	// Update the palette
	var cmd tea.Cmd
	m.commandPalette, cmd = m.commandPalette.Update(msg)

	// Check if user selected a command
	if entry, args, ok := m.commandPalette.SelectedCommand(); ok {
		selected := m.selectedSession()

		// Check if this is a doc review action (doesn't require a session)
		if entry.Command.Action == config.ActionDocReview {
			m.state = stateNormal
			cmd := HiveDocReviewCmd{Arg: ""}
			return m, cmd.Execute(&m)
		}

		// Check if this is a filter action (doesn't require a session)
		if isFilterAction(entry.Command.Action) {
			m.state = stateNormal
			// Resolve action type directly from command action
			var actionType ActionType
			switch entry.Command.Action {
			case config.ActionFilterAll:
				actionType = ActionTypeFilterAll
			case config.ActionFilterActive:
				actionType = ActionTypeFilterActive
			case config.ActionFilterApproval:
				actionType = ActionTypeFilterApproval
			case config.ActionFilterReady:
				actionType = ActionTypeFilterReady
			}
			m.handleFilterAction(actionType)
			return m.applyFilter()
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

		// Check for resolution errors (e.g., template errors)
		if action.Err != nil {
			m.state = stateNormal
			m.err = action.Err
			return m, nil
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
			cmdAction := command.Action{
				Type:      command.ActionType(action.Type),
				SessionID: action.SessionID,
				ShellCmd:  action.ShellCmd,
			}
			exec, err := m.cmdService.CreateExecutor(cmdAction)
			if err != nil {
				log.Error().Str("command", action.Key).Err(err).Msg("failed to create executor before exit")
			} else if err := command.ExecuteSync(context.Background(), exec); err != nil {
				log.Error().Str("command", action.Key).Err(err).Msg("command failed before exit")
			}
			m.quitting = true
			return m, tea.Quit
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
		m.quitting = true
		return m, tea.Quit
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
			m.quitting = true
			return m, tea.Quit
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
		m.quitting = true
		return m, tea.Quit
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

	// Review view focused - forward keys to review view
	if m.isReviewFocused() && m.reviewView != nil {
		var cmd tea.Cmd
		*m.reviewView, cmd = m.reviewView.Update(msg)
		return m, cmd
	}

	return m, nil
}

// handleTabKey handles tab key for switching views.
// If Review tab is visible (has active session), it's included in cycling.
func (m Model) handleTabKey() (tea.Model, tea.Cmd) {
	// Check if Review tab should be visible
	showReviewTab := m.reviewView != nil && m.reviewView.CanShowInTabBar()

	switch m.activeView {
	case ViewSessions:
		m.activeView = ViewMessages
	case ViewMessages:
		if showReviewTab {
			// If Review is visible, cycle to it
			m.activeView = ViewReview
		} else {
			// Otherwise cycle back to Sessions
			m.activeView = ViewSessions
		}
	case ViewReview:
		// From Review, cycle back to Sessions
		m.activeView = ViewSessions
	}
	m.handler.SetActiveView(m.activeView)
	return m, nil
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

	// Handle 'n' for new session (only if repos are discovered)
	if keyStr == "n" && len(m.discoveredRepos) > 0 {
		// Determine preselected remote
		preselectedRemote := m.localRemote
		if selected := m.selectedSession(); selected != nil {
			preselectedRemote = selected.Remote
		}
		// Build map of existing session names for validation
		existingNames := make(map[string]bool, len(m.allSessions))
		for _, s := range m.allSessions {
			existingNames[s.Name] = true
		}
		m.newSessionForm = NewNewSessionForm(m.discoveredRepos, preselectedRemote, existingNames)
		m.state = stateCreatingSession
		return m, m.newSessionForm.Init()
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

	selected := m.selectedSession()
	if selected == nil {
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		return m, cmd
	}

	// If a window sub-item is selected, override TmuxWindow for template rendering
	if treeItem != nil && treeItem.IsWindowItem {
		m.handler.SetSelectedWindow(treeItem.WindowName)
	} else {
		m.handler.SetSelectedWindow("")
	}

	action, ok := m.handler.Resolve(keyStr, *selected)
	if ok {
		// Check for resolution errors (e.g., template errors)
		if action.Err != nil {
			m.err = action.Err
			return m, nil
		}
		if action.NeedsConfirm() {
			m.state = stateConfirming
			m.pending = action
			m.modal = NewModal("Confirm", action.Confirm)
			return m, nil
		}
		if action.Type == ActionTypeRecycle {
			return m, m.startRecycle(action.SessionID)
		}
		// Handle doc review action
		if action.Type == ActionTypeDocReview {
			cmd := HiveDocReviewCmd{Arg: ""}
			return m, cmd.Execute(&m)
		}
		// Handle filter actions
		if m.handleFilterAction(action.Type) {
			return m.applyFilter()
		}
		// If exit is requested, execute synchronously and quit immediately
		// This avoids async message flow issues in some terminal contexts (e.g., tmux popups)
		if action.Exit {
			cmdAction := command.Action{
				Type:      command.ActionType(action.Type),
				SessionID: action.SessionID,
				ShellCmd:  action.ShellCmd,
			}
			exec, err := m.cmdService.CreateExecutor(cmdAction)
			if err != nil {
				log.Error().Str("key", keyStr).Err(err).Msg("failed to create executor before exit")
			} else if err := command.ExecuteSync(context.Background(), exec); err != nil {
				log.Error().Str("key", keyStr).Err(err).Msg("command failed before exit")
			}
			m.quitting = true
			return m, tea.Quit
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

// hasEditorFocus returns true if any text input currently has focus.
// When an editor has focus, most keybindings should be blocked to allow normal typing.
func (m *Model) hasEditorFocus() bool {
	// Check session list filter
	if m.list.SettingFilter() {
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
	default:
		// For other states (normal, confirming, loading, etc.),
		// delegate to active view
	}

	// Route to active view
	switch m.activeView {
	case ViewSessions:
		m.list, cmd = m.list.Update(msg)
	case ViewMessages:
		// MessagesView handles its own filtering internally, no Update method
		// Key events are handled by specific methods called from handleNormalKey
		return m, nil
	case ViewReview:
		if m.reviewView != nil {
			*m.reviewView, cmd = m.reviewView.Update(msg)
		}
	}

	return m, cmd
}

// navigateSkippingHeaders moves the selection by direction (-1 for up, 1 for down),
// skipping over header items which are not actionable.
func (m *Model) navigateSkippingHeaders(direction int) {
	items := m.list.Items()
	if len(items) == 0 {
		return
	}

	current := m.list.Index()
	target := current

	// Move in the given direction until we find a non-header or hit bounds
	for {
		target += direction

		// Check bounds
		if target < 0 || target >= len(items) {
			return // Can't move further, stay at current position
		}

		// Check if target is a header
		if treeItem, ok := items[target].(TreeItem); ok {
			if !treeItem.IsHeader {
				// Found a non-header, select it
				m.list.Select(target)
				return
			}
			// It's a header, keep looking
		} else {
			// Not a TreeItem, select it
			m.list.Select(target)
			return
		}
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

// isFilterAction returns true if the action string is a filter action.
func isFilterAction(action string) bool {
	switch action {
	case config.ActionFilterAll, config.ActionFilterActive,
		config.ActionFilterApproval, config.ActionFilterReady:
		return true
	}
	return false
}

// handleFilterAction checks if the action is a filter action and updates the status filter.
// Returns true if the action was a filter action (caller should call applyFilter).
func (m *Model) handleFilterAction(actionType ActionType) bool {
	switch actionType {
	case ActionTypeFilterAll:
		m.statusFilter = ""
		return true
	case ActionTypeFilterActive:
		m.statusFilter = terminal.StatusActive
		return true
	case ActionTypeFilterApproval:
		m.statusFilter = terminal.StatusApproval
		return true
	case ActionTypeFilterReady:
		m.statusFilter = terminal.StatusReady
		return true
	case ActionTypeNone, ActionTypeRecycle, ActionTypeDelete, ActionTypeDeleteRecycledBatch, ActionTypeShell, ActionTypeDocReview:
		return false
	}
	return false
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

// shouldPollMessages returns true if messages should be polled.
func (m Model) shouldPollMessages() bool {
	return m.activeView == ViewMessages
}

// isModalActive returns true if any modal is currently open.
func (m Model) isModalActive() bool {
	return m.state != stateNormal
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

	// Build sets of current and expected windows keyed by "sessionID:windowIndex"
	// to detect actual changes. Window index is unique per session (names can repeat).
	current := make(map[string]struct{})
	expected := make(map[string]struct{})
	for _, item := range items {
		ti, ok := item.(TreeItem)
		if !ok {
			continue
		}
		if ti.IsWindowItem {
			current[ti.ParentSession.ID+":"+ti.WindowIndex] = struct{}{}
			continue
		}
		if ti.IsHeader || ti.IsRecycledPlaceholder {
			continue
		}
		if ts, ok := m.terminalStatuses.Get(ti.Session.ID); ok && len(ts.Windows) > 1 {
			for _, w := range ts.Windows {
				expected[ti.Session.ID+":"+w.WindowIndex] = struct{}{}
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
	for _, item := range items {
		if ti, ok := item.(TreeItem); ok && ti.IsWindowItem {
			continue
		}
		stripped = append(stripped, item)
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
		if !ok || treeItem.IsHeader || treeItem.IsRecycledPlaceholder || treeItem.IsWindowItem {
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

	for _, item := range items {
		treeItem, ok := item.(TreeItem)
		if !ok || treeItem.IsHeader || treeItem.IsWindowItem || treeItem.IsRecycledPlaceholder {
			continue
		}
		path := treeItem.Session.Path
		paths = append(paths, path)
		// Mark as loading
		m.gitStatuses.Set(path, GitStatus{IsLoading: true})
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

	// Helper to create view with alt screen enabled
	newView := func(content string) tea.View {
		v := tea.NewView(content)
		v.AltScreen = true
		return v
	}

	// Overlay output modal if running recycle
	if m.state == stateRunningRecycle {
		return newView(m.outputModal.Overlay(mainView, w, h))
	}

	// Overlay new session form (render directly without Modal's Confirm/Cancel buttons)
	if m.state == stateCreatingSession && m.newSessionForm != nil {
		formContent := lipgloss.JoinVertical(
			lipgloss.Left,
			modalTitleStyle.Render("New Session"),
			"",
			m.newSessionForm.View(),
		)
		formOverlay := modalStyle.Render(formContent)

		// Use Compositor/Layer for true overlay (background remains visible)
		bgLayer := lipgloss.NewLayer(mainView)
		formLayer := lipgloss.NewLayer(formOverlay)
		formW := lipgloss.Width(formOverlay)
		formH := lipgloss.Height(formOverlay)
		centerX := (w - formW) / 2
		centerY := (h - formH) / 2
		formLayer.X(centerX).Y(centerY).Z(1)

		compositor := lipgloss.NewCompositor(bgLayer, formLayer)
		return newView(compositor.Render())
	}

	// Overlay message preview modal
	if m.state == statePreviewingMessage {
		return newView(m.previewModal.Overlay(mainView, w, h))
	}

	// Overlay loading spinner if loading
	if m.state == stateLoading {
		loadingView := lipgloss.JoinHorizontal(lipgloss.Left, m.spinner.View(), " "+m.loadingMessage)
		modal := NewModal("", loadingView)
		return newView(modal.Overlay(mainView, w, h))
	}

	// Overlay modal if confirming
	if m.state == stateConfirming {
		return newView(m.modal.Overlay(mainView, w, h))
	}

	// Overlay command palette
	if m.state == stateCommandPalette && m.commandPalette != nil {
		return newView(m.commandPalette.Overlay(mainView, w, h))
	}

	// Overlay help dialog
	if m.state == stateShowingHelp && m.helpDialog != nil {
		return newView(m.helpDialog.Overlay(mainView, w, h))
	}

	// Overlay document picker modal (shown on Sessions view)
	if m.docPickerModal != nil {
		return newView(m.docPickerModal.Overlay(mainView, w, h))
	}

	return newView(mainView)
}

// renderTabView renders the tab-based view layout.
func (m Model) renderTabView() string {
	// Build tab bar with tabs on left and branding on right
	var sessionsTab, messagesTab, reviewTab string

	// Check if Review tab should be shown (only when there's an active review session)
	showReviewTab := m.reviewView != nil && m.reviewView.CanShowInTabBar()

	switch m.activeView {
	case ViewSessions:
		sessionsTab = viewSelectedStyle.Render("Sessions")
		messagesTab = viewNormalStyle.Render("Messages")
		if showReviewTab {
			reviewTab = viewNormalStyle.Render("Review")
		}
	case ViewMessages:
		sessionsTab = viewNormalStyle.Render("Sessions")
		messagesTab = viewSelectedStyle.Render("Messages")
		if showReviewTab {
			reviewTab = viewNormalStyle.Render("Review")
		}
	case ViewReview:
		sessionsTab = viewNormalStyle.Render("Sessions")
		messagesTab = viewNormalStyle.Render("Messages")
		reviewTab = viewSelectedStyle.Render("Review")
	}

	// Build tabs with conditional Review tab
	var tabsLeft string
	if showReviewTab || m.activeView == ViewReview {
		tabsLeft = lipgloss.JoinHorizontal(lipgloss.Left, sessionsTab, " | ", messagesTab, " | ", reviewTab)
	} else {
		tabsLeft = lipgloss.JoinHorizontal(lipgloss.Left, sessionsTab, " | ", messagesTab)
	}

	// Add filter indicator if active
	if m.statusFilter != "" {
		filterStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#7aa2f7")).
			Bold(true)
		filterLabel := string(m.statusFilter)
		tabsLeft = lipgloss.JoinHorizontal(lipgloss.Left, tabsLeft, "  ", filterStyle.Render("["+filterLabel+"]"))
	}

	// Branding on right with background
	brandingStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("#3b4261")).
		Foreground(lipgloss.Color("#c0caf5")).
		Padding(0, 1)
	branding := brandingStyle.Render(styles.IconHive + " Hive")

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
	dividerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#565f89"))
	dividerWidth := m.width
	if dividerWidth < 1 {
		dividerWidth = 80 // default width before WindowSizeMsg
	}
	topDivider := dividerStyle.Render(strings.Repeat("─", dividerWidth))
	headerDivider := dividerStyle.Render(strings.Repeat("─", dividerWidth))

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
			content = lipgloss.NewStyle().Height(contentHeight).Render(content)
		}
	case ViewMessages:
		content = m.msgView.View()
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

	// Create vertical divider between list and preview
	dividerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#565f89"))
	dividerLines := make([]string, contentHeight)
	for i := range dividerLines {
		dividerLines[i] = dividerStyle.Render("│")
	}
	divider := strings.Join(dividerLines, "\n")

	// Join horizontally - all three panels have exact matching heights
	return lipgloss.JoinHorizontal(lipgloss.Top, listView, divider, previewContent)
}

// renderPreviewHeader renders the preview header section with session metadata.
func (m Model) renderPreviewHeader(sess *session.Session, maxWidth int) string {
	iconsEnabled := m.cfg.TUI.IconsEnabled()

	// Styles
	nameStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7aa2f7"))
	separatorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#565f89"))
	idStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#bb9af7")) // purple, same as tree view
	branchStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#73daca"))
	addStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#9ece6a"))
	delStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#f7768e"))
	dirtyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#e0af68"))
	dividerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#565f89"))

	divider := strings.Repeat("─", maxWidth)

	// Build title line: "SessionName [window] • #abcd"
	shortID := sess.ID
	if len(shortID) > 4 {
		shortID = shortID[len(shortID)-4:]
	}
	title := nameStyle.Render(sess.Name)
	// Show window name if a specific window is selected
	if ws := m.selectedWindowStatus(); ws != nil {
		windowStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#73daca"))
		title += " " + windowStyle.Render("["+ws.WindowName+"]")
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
		pluginOrder := []string{"github", "beads", "claude"}
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
				case "github":
					icon = styles.IconGithub
				case "beads":
					icon = styles.IconCheckList
				case "claude":
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
	parts = append(parts, lipgloss.NewStyle().Foreground(lipgloss.Color("#9aa5ce")).Render("Output"))
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
		cmdAction := command.Action{
			Type:      command.ActionTypeRecycle,
			SessionID: sessionID,
		}

		exec, err := m.cmdService.CreateExecutor(cmdAction)
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
			cmdAction := command.Action{
				Type:      command.ActionTypeDelete,
				SessionID: sess.ID,
			}

			exec, err := m.cmdService.CreateExecutor(cmdAction)
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
