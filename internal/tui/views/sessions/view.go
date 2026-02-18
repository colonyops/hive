package sessions

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/rs/zerolog/log"

	act "github.com/colonyops/hive/internal/core/action"
	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/internal/core/eventbus"
	"github.com/colonyops/hive/internal/core/session"
	"github.com/colonyops/hive/internal/core/styles"
	"github.com/colonyops/hive/internal/core/terminal"
	"github.com/colonyops/hive/internal/hive"
	"github.com/colonyops/hive/internal/hive/plugins"
	"github.com/colonyops/hive/internal/tui/components"
	"github.com/colonyops/hive/pkg/kv"
	"github.com/colonyops/hive/pkg/tmpl"
)

// Buffer pools for reducing allocations in rendering.
var builderPool = sync.Pool{
	New: func() any {
		return &strings.Builder{}
	},
}

// ViewOpts configures a new sessions View.
type ViewOpts struct {
	// Required — nil causes a panic at construction time.
	Cfg             *config.Config
	Service         *hive.SessionService
	Handler         KeyResolver
	TerminalManager *terminal.Manager
	PluginManager   *plugins.Manager

	// Optional — nil disables the corresponding feature.
	LocalRemote string
	RepoDirs    []string
	Renderer    *tmpl.Renderer
	Bus         *eventbus.EventBus
}

// View is the Bubble Tea sub-model for the sessions tab.
type View struct {
	allSessions  []session.Session
	statusFilter terminal.Status
	localRemote  string

	cfg     *config.Config
	service *hive.SessionService
	bus     *eventbus.EventBus

	// List and tree rendering
	list         list.Model
	treeDelegate TreeDelegate
	handler      KeyResolver
	columnWidths *ColumnWidths

	// Git integration
	gitStatuses *kv.Store[string, GitStatus]
	gitWorkers  int

	// Terminal integration
	terminalManager    *terminal.Manager
	terminalStatuses   *kv.Store[string, TerminalStatus]
	previewEnabled     bool
	previewTemplates   *PreviewTemplates
	currentTmuxSession string

	// Plugin integration
	pluginManager      *plugins.Manager
	pluginStatuses     map[string]*kv.Store[string, plugins.Status]
	pluginResultsChan  <-chan plugins.Result
	pluginPollInterval time.Duration

	// Status animation
	animationFrame int

	// Focus mode filtering
	focusMode        bool
	focusFilter      string
	focusFilterInput textinput.Model

	// Repository discovery
	repoDirs        []string
	discoveredRepos []DiscoveredRepo

	// Layout state
	width       int
	height      int
	active      bool
	refreshing  bool
	modalActive bool

	// Template rendering
	renderer *tmpl.Renderer
}

// New creates a new sessions View. All KV stores, delegates, and plugins are
// initialized here so the parent Model can pass them through ViewOpts without
// constructing them itself.
func New(opts ViewOpts) *View {
	if opts.Cfg == nil || opts.Service == nil || opts.Handler == nil || opts.TerminalManager == nil || opts.PluginManager == nil {
		panic("sessions.New: Cfg, Service, Handler, TerminalManager, and PluginManager are required")
	}
	cfg := opts.Cfg

	gitStatuses := kv.New[string, GitStatus]()
	terminalStatuses := kv.New[string, TerminalStatus]()
	columnWidths := &ColumnWidths{}

	pluginStatuses := make(map[string]*kv.Store[string, plugins.Status])
	for _, p := range opts.PluginManager.EnabledPlugins() {
		if p.StatusProvider() != nil {
			pluginStatuses[p.Name()] = kv.New[string, plugins.Status]()
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
	l.SetShowTitle(false)
	l.SetShowFilter(false)

	l.FilterInput.Prompt = "Filter: "
	filterStyles := textinput.DefaultStyles(true)
	filterStyles.Focused.Prompt = styles.ListFilterPromptStyle
	filterStyles.Cursor.Color = styles.ColorPrimary
	l.FilterInput.SetStyles(filterStyles)

	// Style help to match messages view
	helpStyle := styles.TextMutedStyle
	l.Help.Styles.ShortKey = helpStyle
	l.Help.Styles.ShortDesc = helpStyle
	l.Help.Styles.ShortSeparator = helpStyle
	l.Help.Styles.FullKey = helpStyle
	l.Help.Styles.FullDesc = helpStyle
	l.Help.Styles.FullSeparator = helpStyle
	l.Help.ShortSeparator = " • "
	l.Styles.HelpStyle = styles.ListHelpContainerStyle

	l.AdditionalShortHelpKeys = func() []key.Binding {
		return []key.Binding{
			key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑/↓", "navigate")),
			key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		}
	}

	focusInput := textinput.New()
	focusInput.Prompt = "/"
	focusInputStyles := textinput.DefaultStyles(true)
	focusInputStyles.Focused.Prompt = styles.ListFilterPromptStyle
	focusInputStyles.Cursor.Color = styles.ColorPrimary
	focusInput.SetStyles(focusInputStyles)

	// Detect current tmux session to prevent recursive preview
	currentTmux := DetectCurrentTmuxSession()

	previewTemplates := ParsePreviewTemplates(
		cfg.TUI.Preview.TitleTemplate,
		cfg.TUI.Preview.StatusTemplate,
	)

	pluginPollInterval := 5 * time.Second
	if cfg.Plugins.GitHub.ResultsCache > 0 {
		pluginPollInterval = cfg.Plugins.GitHub.ResultsCache
	}

	return &View{
		localRemote: opts.LocalRemote,
		cfg:         cfg,
		service:     opts.Service,
		bus:         opts.Bus,

		list:         l,
		treeDelegate: delegate,
		handler:      opts.Handler,
		columnWidths: columnWidths,

		gitStatuses: gitStatuses,
		gitWorkers:  cfg.Git.StatusWorkers,

		terminalManager:    opts.TerminalManager,
		terminalStatuses:   terminalStatuses,
		previewEnabled:     cfg.TUI.PreviewEnabled,
		previewTemplates:   previewTemplates,
		currentTmuxSession: currentTmux,

		pluginManager:      opts.PluginManager,
		pluginStatuses:     pluginStatuses,
		pluginPollInterval: pluginPollInterval,

		repoDirs: opts.RepoDirs,

		focusFilterInput: focusInput,
		renderer:         opts.Renderer,
	}
}

// --- Init ---

// Init returns the initial commands for the sessions view.
func (v *View) Init() tea.Cmd {
	cmds := []tea.Cmd{v.loadSessions()}

	if len(v.repoDirs) > 0 {
		cmds = append(cmds, v.scanRepoDirs())
	}

	if v.terminalManager.HasEnabledIntegrations() {
		cmds = append(cmds, StartTerminalPollTicker(v.cfg.Tmux.PollInterval))
		cmds = append(cmds, scheduleAnimationTick())
	}

	if len(v.pluginStatuses) > 0 {
		cmds = append(cmds, v.startPluginWorker())
	}

	if cmd := v.scheduleSessionRefresh(); cmd != nil {
		cmds = append(cmds, cmd)
	}

	return tea.Batch(cmds...)
}

// --- Update ---

// Update handles messages for the sessions view.
func (v *View) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case sessionsLoadedMsg:
		return v.handleSessionsLoaded(msg)
	case GitStatusBatchCompleteMsg:
		return v.handleGitStatusComplete(msg)
	case TerminalStatusBatchCompleteMsg:
		return v.handleTerminalStatusComplete(msg)
	case TerminalPollTickMsg:
		return v.handleTerminalPollTick()
	case pluginWorkerStartedMsg:
		return v.handlePluginWorkerStarted(msg)
	case pluginStatusUpdateMsg:
		return v.handlePluginStatusUpdate(msg)
	case reposDiscoveredMsg:
		return v.handleReposDiscovered(msg)
	case sessionRefreshTickMsg:
		return v.handleSessionRefreshTick()
	case animationTickMsg:
		return v.handleAnimationTick()
	case RefreshSessionsMsg:
		return v.loadSessions()
	case tea.KeyMsg:
		return v.handleKeyMsg(msg)
	default:
		// Forward all other messages to list.Update()
		var cmd tea.Cmd
		v.list, cmd = v.list.Update(msg)
		return cmd
	}
}

// --- Internal message handlers ---

func (v *View) handleSessionsLoaded(msg sessionsLoadedMsg) tea.Cmd {
	if msg.err != nil {
		log.Error().Err(msg.err).Msg("failed to load sessions")
		return nil
	}
	v.allSessions = msg.sessions
	cmd := v.applyFilter()
	if len(v.pluginStatuses) > 0 {
		sessions := make([]*session.Session, len(v.allSessions))
		for i := range v.allSessions {
			sessions[i] = &v.allSessions[i]
		}
		v.pluginManager.UpdateSessions(sessions)
		log.Debug().Int("sessionCount", len(sessions)).Msg("updated plugin manager sessions")
	}
	return cmd
}

func (v *View) handleGitStatusComplete(msg GitStatusBatchCompleteMsg) tea.Cmd {
	v.gitStatuses.SetBatch(msg.Results)
	v.refreshing = false
	return nil
}

func (v *View) handleTerminalStatusComplete(msg TerminalStatusBatchCompleteMsg) tea.Cmd {
	if v.terminalStatuses != nil {
		if v.bus != nil {
			for sessionID, newStatus := range msg.Results {
				oldStatus, exists := v.terminalStatuses.Get(sessionID)
				prevStatus := oldStatus.Status
				if !exists {
					prevStatus = terminal.StatusMissing
				}
				if prevStatus != newStatus.Status {
					sess := v.findByID(sessionID)
					if sess == nil {
						continue
					}
					v.bus.PublishAgentStatusChanged(eventbus.AgentStatusChangedPayload{
						Session:   sess,
						OldStatus: prevStatus,
						NewStatus: newStatus.Status,
					})
				}
			}
		}

		v.terminalStatuses.SetBatch(msg.Results)
		v.rebuildWindowItems()
	}
	return nil
}

func (v *View) handleTerminalPollTick() tea.Cmd {
	var cmds []tea.Cmd
	allSess := v.allSessions
	sessPtrs := make([]*session.Session, len(allSess))
	for i := range allSess {
		sessPtrs[i] = &v.allSessions[i]
	}
	cmds = append(cmds, FetchTerminalStatusBatch(v.terminalManager, sessPtrs, v.gitWorkers))
	if v.terminalManager.HasEnabledIntegrations() {
		cmds = append(cmds, StartTerminalPollTicker(v.cfg.Tmux.PollInterval))
	}
	return tea.Batch(cmds...)
}

func (v *View) handlePluginWorkerStarted(msg pluginWorkerStartedMsg) tea.Cmd {
	v.pluginResultsChan = msg.resultsChan
	log.Debug().Msg("plugin background worker started")
	return listenForPluginResult(v.pluginResultsChan)
}

func (v *View) handlePluginStatusUpdate(msg pluginStatusUpdateMsg) tea.Cmd {
	if msg.Err != nil {
		log.Debug().
			Err(msg.Err).
			Str("plugin", msg.PluginName).
			Str("session", msg.SessionID).
			Msg("plugin status update failed")
	} else if store, ok := v.pluginStatuses[msg.PluginName]; ok {
		store.Set(msg.SessionID, msg.Status)
		log.Debug().
			Str("plugin", msg.PluginName).
			Str("session", msg.SessionID).
			Str("label", msg.Status.Label).
			Msg("plugin status updated")
	}
	v.treeDelegate.PluginStatuses = v.pluginStatuses
	v.list.SetDelegate(v.treeDelegate)
	return listenForPluginResult(v.pluginResultsChan)
}

func (v *View) handleReposDiscovered(msg reposDiscoveredMsg) tea.Cmd {
	v.discoveredRepos = msg.repos
	return nil
}

func (v *View) handleSessionRefreshTick() tea.Cmd {
	if v.active && !v.modalActive {
		v.refreshing = true
		return tea.Batch(
			v.loadSessions(),
			v.scheduleSessionRefresh(),
		)
	}
	return v.scheduleSessionRefresh()
}

func (v *View) handleAnimationTick() tea.Cmd {
	v.animationFrame = (v.animationFrame + 1) % AnimationFrameCount
	v.treeDelegate.AnimationFrame = v.animationFrame
	v.list.SetDelegate(v.treeDelegate)
	return scheduleAnimationTick()
}

// --- Key handling ---

func (v *View) handleKeyMsg(msg tea.KeyMsg) tea.Cmd {
	_, cmd := v.handleKey(msg)
	return cmd
}

func (v *View) handleKey(msg tea.KeyMsg) (*View, tea.Cmd) {
	keyStr := msg.String()

	// Handle focus mode filtering
	if v.focusMode {
		return v.handleFilterKey(msg, keyStr)
	}

	// Handle list filter mode
	if v.list.SettingFilter() {
		var cmd tea.Cmd
		v.list, cmd = v.list.Update(msg)
		return v, cmd
	}

	switch keyStr {
	case "up", "k":
		v.navigateSkippingPlaceholders(-1)
		return v, nil
	case "down", "j":
		v.navigateSkippingPlaceholders(1)
		return v, nil
	}

	if v.handler.IsAction(keyStr, act.TypeNextActive) {
		v.navigateToNextActive(1)
		return v, nil
	}
	if v.handler.IsAction(keyStr, act.TypePrevActive) {
		v.navigateToNextActive(-1)
		return v, nil
	}

	if v.handler.IsAction(keyStr, act.TypeNewSession) && len(v.discoveredRepos) > 0 {
		return v, func() tea.Msg { return NewSessionRequestMsg{} }
	}

	if keyStr == "/" {
		v.focusMode = true
		v.focusFilter = ""
		v.focusFilterInput.Reset()
		v.focusFilterInput.SetValue("")

		contentHeight := v.height - 3
		if contentHeight < 2 {
			contentHeight = 2
		}

		var listWidth int
		if v.previewEnabled && v.width >= 80 {
			listWidth = int(float64(v.width) * 0.25)
		} else {
			listWidth = v.width
		}

		v.list.SetShowHelp(false)
		v.list.SetSize(listWidth, contentHeight-1) // Make room for search input

		return v, v.focusFilterInput.Focus()
	}

	if keyStr == ":" {
		sess := v.SelectedSession()
		return v, func() tea.Msg { return CommandPaletteRequestMsg{Session: sess} }
	}

	treeItem := v.SelectedTreeItem()
	if treeItem != nil && treeItem.IsRecycledPlaceholder {
		return v.handleRecycledPlaceholderKey(keyStr, treeItem)
	}

	if treeItem != nil && treeItem.IsHeader && v.handler.IsCommand(keyStr, "TmuxOpen") {
		return v.handleRepoHeaderKey(treeItem)
	}

	selected := v.SelectedSession()
	if selected == nil {
		var cmd tea.Cmd
		v.list, cmd = v.list.Update(msg)
		return v, cmd
	}

	if treeItem != nil && treeItem.IsWindowItem {
		v.handler.SetSelectedWindow(treeItem.WindowIndex)
	} else {
		v.handler.SetSelectedWindow("")
	}

	if cmdName, cmd, hasForm := v.handler.ResolveFormCommand(keyStr, *selected); hasForm {
		return v, func() tea.Msg {
			return FormCommandRequestMsg{
				Name:    cmdName,
				Cmd:     cmd,
				Session: *selected,
			}
		}
	}

	action, ok := v.handler.Resolve(keyStr, *selected)
	action = MaybeOverrideWindowDelete(action, treeItem, v.renderer)
	if ok {
		return v, func() tea.Msg { return ActionRequestMsg{Action: action} }
	}

	var cmd tea.Cmd
	v.list, cmd = v.list.Update(msg)
	return v, cmd
}

func (v *View) handleFilterKey(msg tea.KeyMsg, keyStr string) (*View, tea.Cmd) {
	switch keyStr {
	case "esc":
		v.stopFocusMode()
		return v, nil
	case "enter":
		v.stopFocusMode()
		return v, nil
	default:
		var cmd tea.Cmd
		v.focusFilterInput, cmd = v.focusFilterInput.Update(msg)
		// Update filter and navigate on every keystroke
		v.updateFocusFilter(v.focusFilterInput.Value())
		return v, cmd
	}
}

func (v *View) handleRecycledPlaceholderKey(keyStr string, treeItem *TreeItem) (*View, tea.Cmd) {
	if !v.handler.IsAction(keyStr, act.TypeDelete) {
		return v, nil
	}
	recycledSessions := treeItem.RecycledSessions
	return v, func() tea.Msg {
		return RecycledDeleteRequestMsg{Sessions: recycledSessions}
	}
}

func (v *View) handleRepoHeaderKey(header *TreeItem) (*View, tea.Cmd) {
	// Find the original repo path from discovered repos
	var repoPath string
	for _, repo := range v.discoveredRepos {
		if repo.Remote == header.RepoRemote {
			repoPath = repo.Path
			break
		}
	}
	if repoPath == "" {
		return v, nil
	}

	return v, func() tea.Msg {
		return OpenRepoRequestMsg{
			Name:   header.RepoName,
			Remote: header.RepoRemote,
		}
	}
}

// --- Navigation ---

// navigateToNextActive moves the cursor to the next session with active terminal status.
// Wraps around to the beginning if no match is found after the current position.
// For multi-window sessions, also checks per-window statuses.
func (v *View) navigateToNextActive(direction int) {
	items := v.list.Items()
	if len(items) == 0 || v.terminalStatuses == nil {
		return
	}

	current := v.list.Index()
	n := len(items)

	for step := 1; step < n; step++ {
		idx := (current + step*direction + n) % n
		treeItem, ok := items[idx].(TreeItem)
		if !ok || treeItem.IsHeader || treeItem.IsRecycledPlaceholder {
			continue
		}

		if treeItem.IsWindowItem {
			// Check per-window status
			if ts, ok := v.terminalStatuses.Get(treeItem.ParentSession.ID); ok {
				for i := range ts.Windows {
					if ts.Windows[i].WindowIndex == treeItem.WindowIndex && isActiveStatus(ts.Windows[i].Status) {
						v.list.Select(idx)
						return
					}
				}
			}
			continue
		}

		// Check top-level session status
		if ts, ok := v.terminalStatuses.Get(treeItem.Session.ID); ok {
			if isActiveStatus(ts.Status) {
				v.list.Select(idx)
				return
			}
		}
	}
}

// navigateSkippingPlaceholders moves the selection by direction (-1 for up, 1 for down).
// Recycled placeholders are skipped; all other items (including headers) are selectable.
func (v *View) navigateSkippingPlaceholders(direction int) {
	items := v.list.Items()
	if len(items) == 0 {
		return
	}

	current := v.list.Index()
	target := current

	for {
		target += direction

		if target < 0 || target >= len(items) {
			return // Can't move further, stay at current position
		}

		// Skip recycled placeholders, allow everything else (including headers)
		if treeItem, ok := items[target].(TreeItem); ok && treeItem.IsRecycledPlaceholder {
			continue
		}
		v.list.Select(target)
		return
	}
}

// saveSelection snapshots the current selection for restore after a list rebuild.
func (v *View) saveSelection() TreeSelection {
	return SaveTreeSelection(v.SelectedTreeItem(), v.list.Index())
}

// restoreSelection applies a saved selection to the current list items.
func (v *View) restoreSelection(sel TreeSelection) {
	treeItems := ListItemsToTreeItems(v.list.Items())
	v.list.Select(sel.Restore(treeItems))
}

// --- Tree manipulation ---

// applyFilter rebuilds the tree view from all sessions.
func (v *View) applyFilter() tea.Cmd {
	sel := v.saveSelection()

	allSess := v.allSessions
	filteredSess := allSess
	statusFilter := v.statusFilter
	if statusFilter != "" && v.terminalStatuses != nil {
		filtered := make([]session.Session, 0, len(allSess))
		for _, s := range allSess {
			if status, ok := v.terminalStatuses.Get(s.ID); ok {
				if status.Status == statusFilter {
					filtered = append(filtered, s)
				}
			}
		}
		filteredSess = filtered
	}

	localRemote := v.localRemote

	groups := GroupSessionsByRepo(filteredSess, localRemote)
	items := BuildTreeItems(groups, localRemote)
	items = v.expandWindowItems(items)
	*v.columnWidths = CalculateColumnWidths(filteredSess, nil)

	// Collect paths for git status fetching (use filtered sessions)
	// During background refresh, keep existing statuses to avoid flashing
	paths := make([]string, 0, len(filteredSess))
	for _, s := range filteredSess {
		paths = append(paths, s.Path)
		if !v.refreshing {
			v.gitStatuses.Set(s.Path, GitStatus{IsLoading: true})
		}
	}

	v.list.SetItems(items)
	v.restoreSelection(sel)

	if len(paths) == 0 {
		v.refreshing = false
		return nil
	}
	// refreshing is cleared when GitStatusBatchCompleteMsg is received
	return FetchGitStatusBatch(v.service.Git(), paths, v.gitWorkers)
}

// rebuildWindowItems strips existing window sub-items from the list and re-expands
// based on current terminal statuses. Preserves the current selection.
func (v *View) rebuildWindowItems() {
	items := v.list.Items()

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
		if ts, ok := v.terminalStatuses.Get(ti.Session.ID); ok && len(ts.Windows) > 1 {
			for _, w := range ts.Windows {
				expected[ti.Session.ID+"\x1f"+w.WindowIndex+"\x1f"+w.WindowName] = struct{}{}
			}
		}
	}

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

	sel := v.saveSelection()

	stripped := make([]list.Item, 0, len(items))
	for i, ti := range TreeItemsAll(items) {
		if ti.IsWindowItem {
			continue
		}
		stripped = append(stripped, items[i])
	}

	expanded := v.expandWindowItems(stripped)
	v.list.SetItems(expanded)
	v.restoreSelection(sel)
}

// expandWindowItems inserts window sub-items after each session that has multiple
// terminal windows. Single-window sessions are left unchanged.
func (v *View) expandWindowItems(items []list.Item) []list.Item {
	if v.terminalStatuses == nil {
		return items
	}

	expanded := make([]list.Item, 0, len(items))
	for _, item := range items {
		expanded = append(expanded, item)

		treeItem, ok := item.(TreeItem)
		if !ok || !treeItem.IsSession() {
			continue
		}

		ts, ok := v.terminalStatuses.Get(treeItem.Session.ID)
		if !ok || len(ts.Windows) <= 1 {
			continue
		}

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

// --- Focus mode ---

// stopFocusMode deactivates focus mode filtering.
func (v *View) stopFocusMode() {
	v.focusMode = false
	v.focusFilter = ""

	contentHeight := v.height - 3
	if contentHeight < 1 {
		contentHeight = 1
	}

	var listWidth int
	if v.previewEnabled && v.width >= 80 {
		listWidth = int(float64(v.width) * 0.25)
	} else {
		listWidth = v.width
	}

	v.list.SetSize(listWidth, contentHeight)
	v.list.SetShowHelp(true)
}

// updateFocusFilter updates the filter and navigates to first match.
func (v *View) updateFocusFilter(filter string) {
	v.focusFilter = filter
	if filter == "" {
		return // no navigation with empty filter
	}

	filterLower := strings.ToLower(filter)

	for i, ti := range TreeItemsAll(v.list.Items()) {
		if ti.IsHeader {
			continue
		}

		filterValue := strings.ToLower(ti.FilterValue())
		if strings.Contains(filterValue, filterLower) {
			v.list.Select(i)
			return
		}
	}

	// No match found - cursor stays at current position
}

// --- Rendering ---

// View renders the sessions view.
func (v *View) View() string {
	// Calculate content height: total - top divider (1) - header (1) - bottom divider (1)
	contentHeight := max(v.height-3, 1)

	if v.previewEnabled && v.width >= 80 {
		return v.renderDualColumnLayout(contentHeight)
	}

	// Reset delegate to show full info when not in preview mode
	v.treeDelegate.PreviewMode = false
	v.list.SetDelegate(v.treeDelegate)
	content := v.list.View()

	// Show focus mode filter input if active (at bottom to avoid layout shift)
	if v.focusMode {
		content = lipgloss.JoinVertical(lipgloss.Left, content, v.focusFilterInput.View())
	} else if v.list.SettingFilter() {
		// Fallback: show bubbles built-in filter if somehow active
		content = lipgloss.JoinVertical(lipgloss.Left, v.list.FilterInput.View(), content)
	}

	content = lipgloss.NewStyle().Height(contentHeight).Render(content)
	return content
}

// renderDualColumnLayout renders sessions list and preview side by side.
func (v *View) renderDualColumnLayout(contentHeight int) string {
	v.treeDelegate.PreviewMode = true
	v.list.SetDelegate(v.treeDelegate)

	// Calculate widths (25% list, 1 char divider, remaining for preview)
	listWidth := int(float64(v.width) * 0.25)
	if listWidth < 20 {
		listWidth = 20
	}

	// Account for divider (1 char) between list and preview
	dividerWidth := 1
	previewWidth := v.width - listWidth - dividerWidth

	selected := v.SelectedSession()
	var previewContent string

	if selected != nil {
		// Check if this is the current session (would cause recursive preview)
		isSelf := v.isCurrentTmuxSession(selected)

		// Determine pane content: use per-window content if a window item is selected,
		// otherwise fall back to session-level content.
		var paneContent string
		if ws := v.selectedWindowStatus(); ws != nil {
			paneContent = ws.PaneContent
		} else if status, ok := v.terminalStatuses.Get(selected.ID); ok {
			paneContent = status.PaneContent
		}

		switch {
		case isSelf:
			// Show placeholder instead of recursive preview
			previewContent = v.renderPreviewHeader(selected, previewWidth-4) + "\n\n(current session, preventing recursive view)"
		case paneContent != "":
			// Account for padding: 2 chars on each side = 4 total
			usableWidth := previewWidth - 4

			header := v.renderPreviewHeader(selected, usableWidth)
			headerHeight := strings.Count(header, "\n") + 1

			outputHeight := max(contentHeight-headerHeight, 1)
			content := tailLines(paneContent, outputHeight)
			content = truncateLines(content, usableWidth)

			previewContent = header + "\n" + content
		default:
			previewContent = v.renderPreviewHeader(selected, previewWidth-4) + "\n\nNo pane content available"
		}
	} else {
		previewContent = "No session selected"
	}

	listView := v.list.View()
	if v.focusMode {
		listView = lipgloss.JoinVertical(lipgloss.Left, listView, v.focusFilterInput.View())
	} else if v.list.SettingFilter() {
		listView = lipgloss.JoinVertical(lipgloss.Left, v.list.FilterInput.View(), listView)
	}

	listView = ensureExactHeight(listView, contentHeight)
	previewContent = ensureExactHeight(previewContent, contentHeight)

	// Apply exact width to list view to prevent bleeding into preview
	listView = ensureExactWidth(listView, listWidth)

	previewLines := strings.Split(previewContent, "\n")
	for i, line := range previewLines {
		previewLines[i] = "  " + line + "  "
	}
	previewContent = strings.Join(previewLines, "\n")
	previewContent = ensureExactWidth(previewContent, previewWidth)
	previewContent = styles.TextForegroundStyle.Render(previewContent)

	dividerLines := make([]string, contentHeight)
	for i := range dividerLines {
		dividerLines[i] = styles.TextMutedStyle.Render("│")
	}
	divider := strings.Join(dividerLines, "\n")

	// Join horizontally - all three panels have exact matching heights
	return lipgloss.JoinHorizontal(lipgloss.Top, listView, divider, previewContent)
}

// renderPreviewHeader renders the preview header section with session metadata.
func (v *View) renderPreviewHeader(sess *session.Session, maxWidth int) string {
	iconsEnabled := v.cfg.TUI.IconsEnabled()

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
	if ws := v.selectedWindowStatus(); ws != nil {
		title += " " + styles.TextSecondaryStyle.Render("["+ws.WindowName+"]")
	}
	title += separatorStyle.Render(" • ") + idStyle.Render("#"+shortID)

	// Build status line with colors
	var statusParts []string

	// Git status
	if v.gitStatuses != nil {
		if status, ok := v.gitStatuses.Get(sess.Path); ok && !status.IsLoading && status.Error == nil {
			gitPart := branchStyle.Render("(")
			if iconsEnabled {
				gitPart += branchStyle.Render(styles.IconGitBranch + " ")
			}
			gitPart += branchStyle.Render(status.Branch + ")")
			gitPart += " " + addStyle.Render("+"+fmt.Sprintf("%d", status.Additions))
			gitPart += " " + delStyle.Render("-"+fmt.Sprintf("%d", status.Deletions))
			if status.HasChanges && iconsEnabled {
				gitPart += " " + dirtyStyle.Render(styles.IconGit)
			}
			statusParts = append(statusParts, gitPart)
		}
	}

	// Plugin statuses (neutral color)
	if v.pluginStatuses != nil {
		pluginOrder := []string{PluginGitHub, PluginBeads, PluginClaude}
		for _, name := range pluginOrder {
			store, ok := v.pluginStatuses[name]
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
				case PluginGitHub:
					icon = styles.IconGithub
				case PluginBeads:
					icon = styles.IconCheckList
				case PluginClaude:
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

// isCurrentTmuxSession returns true if the given session matches the current tmux session.
// This prevents recursive preview when hive is previewing its own pane.
func (v *View) isCurrentTmuxSession(sess *session.Session) bool {
	if v.currentTmuxSession == "" {
		return false
	}

	if v.currentTmuxSession == sess.Slug {
		return true
	}

	// Check prefix match (tmux session might be slug_suffix or slug-suffix)
	if strings.HasPrefix(v.currentTmuxSession, sess.Slug+"_") ||
		strings.HasPrefix(v.currentTmuxSession, sess.Slug+"-") {
		return true
	}

	if tmuxSession := sess.Metadata[session.MetaTmuxSession]; tmuxSession != "" {
		if v.currentTmuxSession == tmuxSession {
			return true
		}
	}

	return false
}

// --- Status/Filter ---

// handleFilterAction checks if the action is a filter action and updates the status filter.
// Returns true if the action was a filter action (caller should call applyFilter).
func (v *View) handleFilterAction(actionType act.Type) bool {
	switch actionType {
	case act.TypeFilterAll:
		v.statusFilter = ""
		return true
	case act.TypeFilterActive:
		v.statusFilter = terminal.StatusActive
		return true
	case act.TypeFilterApproval:
		v.statusFilter = terminal.StatusApproval
		return true
	case act.TypeFilterReady:
		v.statusFilter = terminal.StatusReady
		return true
	default:
		return false
	}
}

// selectedWindowStatus returns the WindowStatus for the currently selected window item,
// or nil if a session (not a window) is selected.
func (v *View) selectedWindowStatus() *WindowStatus {
	item := v.list.SelectedItem()
	if item == nil {
		return nil
	}
	treeItem, ok := item.(TreeItem)
	if !ok || !treeItem.IsWindowItem {
		return nil
	}
	if v.terminalStatuses == nil {
		return nil
	}
	ts, ok := v.terminalStatuses.Get(treeItem.ParentSession.ID)
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

// --- Commands ---

// loadSessions returns a command that loads sessions from the service.
func (v *View) loadSessions() tea.Cmd {
	return func() tea.Msg {
		sessions, err := v.service.ListSessions(context.Background())
		return sessionsLoadedMsg{sessions: sessions, err: err}
	}
}

// scanRepoDirs returns a command that scans configured directories for git repositories.
func (v *View) scanRepoDirs() tea.Cmd {
	return func() tea.Msg {
		repos, err := ScanRepoDirs(context.Background(), v.repoDirs, v.service.Git())
		if err != nil {
			log.Debug().Err(err).Msg("repo directory scan encountered errors")
		}
		return reposDiscoveredMsg{repos: repos}
	}
}

// startPluginWorker returns a command that starts the background plugin worker.
func (v *View) startPluginWorker() tea.Cmd {
	return func() tea.Msg {
		resultsChan := v.pluginManager.StartBackgroundWorker(context.Background(), v.pluginPollInterval)
		return pluginWorkerStartedMsg{resultsChan: resultsChan}
	}
}

// RefreshGitStatuses returns a command that refreshes git status for all sessions.
func (v *View) RefreshGitStatuses() tea.Cmd {
	items := v.list.Items()
	paths := make([]string, 0, len(items))

	for _, ti := range TreeItemsSessions(items) {
		paths = append(paths, ti.Session.Path)
		v.gitStatuses.Set(ti.Session.Path, GitStatus{IsLoading: true})
	}

	if len(paths) == 0 {
		return nil
	}

	return FetchGitStatusBatch(v.service.Git(), paths, v.gitWorkers)
}

// scheduleSessionRefresh returns a command that schedules the next session refresh.
func (v *View) scheduleSessionRefresh() tea.Cmd {
	interval := v.cfg.TUI.RefreshInterval
	if interval == 0 {
		return nil // Disabled
	}
	return tea.Tick(interval, func(time.Time) tea.Msg {
		return sessionRefreshTickMsg{}
	})
}

// --- Polling helpers ---

// Animation constants.
const animationTickInterval = 100 * time.Millisecond

// scheduleAnimationTick returns a command that schedules the next animation frame.
func scheduleAnimationTick() tea.Cmd {
	return tea.Tick(animationTickInterval, func(time.Time) tea.Msg {
		return animationTickMsg{}
	})
}

// --- Public accessors ---

// SetSize updates the view dimensions.
func (v *View) SetSize(width, height int) {
	v.width = width
	v.height = height

	// Account for preview mode when setting list dimensions
	contentHeight := height - 3
	if contentHeight < 1 {
		contentHeight = 1
	}

	if v.previewEnabled && width >= 80 {
		listWidth := int(float64(width) * 0.25)
		v.list.SetSize(listWidth, contentHeight)
	} else {
		v.list.SetSize(width, contentHeight)
	}
}

// SetActive marks whether this view is the currently active tab.
func (v *View) SetActive(active bool) {
	v.active = active
}

// SetModalActive informs the view whether the parent has a modal open.
// This affects whether periodic refresh polling fires.
func (v *View) SetModalActive(active bool) {
	v.modalActive = active
}

// HasEditorFocus returns true if a text input is active (list filter or focus mode).
func (v *View) HasEditorFocus() bool {
	return v.list.SettingFilter() || v.focusMode
}

// FocusMode returns true when focus mode filtering is active.
func (v *View) FocusMode() bool {
	return v.focusMode
}

// SelectedSession returns the currently selected session, or nil.
// Returns nil for headers and recycled placeholders.
// For window sub-items, returns the parent session.
func (v *View) SelectedSession() *session.Session {
	item := v.list.SelectedItem()
	if item == nil {
		return nil
	}
	ti, ok := item.(TreeItem)
	if !ok {
		return nil
	}
	if ti.IsHeader || ti.IsRecycledPlaceholder {
		return nil
	}
	if ti.IsWindowItem {
		return &ti.ParentSession
	}
	return &ti.Session
}

// SelectedTreeItem returns the currently selected tree item, or nil.
func (v *View) SelectedTreeItem() *TreeItem {
	item := v.list.SelectedItem()
	if item == nil {
		return nil
	}
	ti, ok := item.(TreeItem)
	if !ok {
		return nil
	}
	return &ti
}

// AllSessions returns all sessions.
func (v *View) AllSessions() []session.Session {
	return v.allSessions
}

// DiscoveredRepos returns the discovered repositories.
func (v *View) DiscoveredRepos() []DiscoveredRepo {
	return v.discoveredRepos
}

// TerminalStatuses returns the terminal status store.
func (v *View) TerminalStatuses() *kv.Store[string, TerminalStatus] {
	return v.terminalStatuses
}

// GitStatuses returns the git status store.
func (v *View) GitStatuses() *kv.Store[string, GitStatus] {
	return v.gitStatuses
}

// PluginStatuses returns the plugin status stores.
func (v *View) PluginStatuses() map[string]*kv.Store[string, plugins.Status] {
	return v.pluginStatuses
}

// PreviewEnabled returns whether the preview sidebar is enabled.
func (v *View) PreviewEnabled() bool {
	return v.previewEnabled
}

// SetPreviewEnabled sets whether the preview sidebar is enabled.
func (v *View) SetPreviewEnabled(enabled bool) {
	v.previewEnabled = enabled
}

// IsSettingFilter returns true when the list filter input is active.
func (v *View) IsSettingFilter() bool {
	return v.list.SettingFilter()
}

// StatusFilter returns the current terminal status filter.
func (v *View) StatusFilter() terminal.Status {
	return v.statusFilter
}

// findByID returns the session with the given ID, or nil if not found.
func (v *View) findByID(id string) *session.Session {
	for i := range v.allSessions {
		if v.allSessions[i].ID == id {
			return &v.allSessions[i]
		}
	}
	return nil
}

// ApplyStatusFilter sets the filter based on the action type and rebuilds the view.
func (v *View) ApplyStatusFilter(actionType act.Type) {
	v.handleFilterAction(actionType)
	v.applyFilter()
}

// HasTerminalIntegration returns true if a terminal manager is configured with enabled integrations.
func (v *View) HasTerminalIntegration() bool {
	return v.terminalManager.HasEnabledIntegrations()
}

// TogglePreview toggles the preview sidebar on/off.
func (v *View) TogglePreview() {
	v.previewEnabled = !v.previewEnabled
}

// ApplyTheme resets delegate styles and clears cached animation colors for a theme change.
func (v *View) ApplyTheme() {
	v.treeDelegate.Styles = DefaultTreeDelegateStyles()
	v.list.SetDelegate(v.treeDelegate)
	ClearAnimationColors()
}

// LocalRemote returns the local remote URL.
func (v *View) LocalRemote() string {
	return v.localRemote
}

// --- Package-level utility functions ---

// isActiveStatus returns true for any session with a live terminal (not missing).
func isActiveStatus(s terminal.Status) bool {
	return s != terminal.StatusMissing && s != ""
}

// IsFilterAction returns true if the action type is a filter action.
func IsFilterAction(t act.Type) bool {
	switch t {
	case act.TypeFilterAll, act.TypeFilterActive,
		act.TypeFilterApproval, act.TypeFilterReady:
		return true
	default:
		return false
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

// MaybeOverrideWindowDelete converts a delete action into a tmux window kill
// when a window sub-item is selected. This keeps "d" context-aware.
func MaybeOverrideWindowDelete(action act.Action, treeItem *TreeItem, renderer *tmpl.Renderer) act.Action {
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
	cmd, err := renderer.Render("tmux kill-window -t {{ .Target | shq }}", map[string]string{
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

// --- Rendering helpers (package-level) ---

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
