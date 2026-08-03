package sessions

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/rs/zerolog/log"

	act "github.com/colonyops/hive/internal/core/action"
	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/internal/core/eventbus"
	"github.com/colonyops/hive/internal/core/git"
	"github.com/colonyops/hive/internal/core/session"
	"github.com/colonyops/hive/internal/core/styles"
	"github.com/colonyops/hive/internal/core/terminal"
	"github.com/colonyops/hive/internal/core/tmux"
	"github.com/colonyops/hive/internal/core/workspace"
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
	Cfg           *config.Config
	Service       *hive.SessionService
	Handler       KeyResolver
	Status        *hive.StatusService
	PluginManager *plugins.Manager

	// Optional — nil disables the corresponding feature.
	LocalRemote string
	Workspaces  []string
	Renderer    *tmpl.Renderer
	Bus         *eventbus.EventBus
}

// View is the Bubble Tea sub-model for the sessions tab.
type View struct {
	allSessions  []session.Session
	statusFilter terminal.Status
	groupBy      string // "repo" or "group", runtime-togglable
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
	status             *hive.StatusService
	terminalStatuses   *kv.Store[string, hive.TerminalStatus]
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
	workspaces      []string
	discoveredRepos []workspace.DiscoveredRepo

	// Layout state
	width       int
	height      int
	active      bool
	refreshing  bool
	modalActive bool

	// Pending selection — overrides saveSelection on next applyFilter.
	pendingSelectID string

	// Template rendering
	renderer *tmpl.Renderer
}

// New creates a new sessions View. All KV stores, delegates, and plugins are
// initialized here so the parent Model can pass them through ViewOpts without
// constructing them itself.
func New(opts ViewOpts) *View {
	if opts.Cfg == nil || opts.Service == nil || opts.Handler == nil || opts.Status == nil || opts.PluginManager == nil {
		panic("sessions.New: Cfg, Service, Handler, Status, and PluginManager are required")
	}
	cfg := opts.Cfg

	gitStatuses := kv.New[string, GitStatus]()
	terminalStatuses := kv.New[string, hive.TerminalStatus]()
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

	l.SetShowHelp(false)

	focusInput := textinput.New()
	focusInput.Prompt = "/"
	focusInputStyles := textinput.DefaultStyles(true)
	focusInputStyles.Focused.Prompt = styles.ListFilterPromptStyle
	focusInputStyles.Cursor.Color = styles.ColorPrimary
	focusInput.SetStyles(focusInputStyles)

	// Detect current tmux session to prevent recursive preview
	currentTmux := tmux.DetectCurrentTmuxSession()

	previewTemplates := ParsePreviewTemplates(
		cfg.Views.Sessions.PreviewTitle,
		cfg.Views.Sessions.PreviewStatus,
	)

	pluginPollInterval := 5 * time.Second
	if cfg.Plugins.GitHub.ResultsCache > 0 {
		pluginPollInterval = cfg.Plugins.GitHub.ResultsCache
	}

	return &View{
		localRemote: opts.LocalRemote,
		groupBy:     cfg.Views.Sessions.GroupBy,
		cfg:         cfg,
		service:     opts.Service,
		bus:         opts.Bus,

		list:         l,
		treeDelegate: delegate,
		handler:      opts.Handler,
		columnWidths: columnWidths,

		gitStatuses: gitStatuses,
		gitWorkers:  cfg.Git.StatusWorkers,

		status:             opts.Status,
		terminalStatuses:   terminalStatuses,
		previewEnabled:     cfg.Views.Sessions.PreviewEnabled,
		previewTemplates:   previewTemplates,
		currentTmuxSession: currentTmux,

		pluginManager:      opts.PluginManager,
		pluginStatuses:     pluginStatuses,
		pluginPollInterval: pluginPollInterval,

		workspaces: opts.Workspaces,

		focusFilterInput: focusInput,
		renderer:         opts.Renderer,
	}
}

// --- Init ---

// Init returns the initial commands for the sessions view.
func (v *View) Init() tea.Cmd {
	cmds := []tea.Cmd{v.loadSessions()}

	if len(v.workspaces) > 0 {
		cmds = append(cmds, v.scanRepoDirs())
	}

	if v.status.Available() {
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
		v.refreshing = true
		return v.loadSessions()
	case tea.KeyPressMsg:
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
		return ErrorCmd(fmt.Errorf("failed to load sessions: %w", msg.err))
	}
	v.allSessions = msg.sessions
	cmds := []tea.Cmd{v.applyFilter()}
	if len(v.pluginStatuses) > 0 {
		sessions := make([]*session.Session, len(v.allSessions))
		for i := range v.allSessions {
			sessions[i] = &v.allSessions[i]
		}
		v.pluginManager.UpdateSessions(sessions)
		log.Debug().Int("sessionCount", len(sessions)).Msg("updated plugin manager sessions")
	}
	// Immediately fetch terminal status so newly created sessions are detected
	// without waiting for the next scheduled poll tick (up to 1500ms delay).
	if v.status.Available() && len(v.allSessions) > 0 {
		sessPtrs := make([]*session.Session, len(v.allSessions))
		for i := range v.allSessions {
			sessPtrs[i] = &v.allSessions[i]
		}
		cmds = append(cmds, FetchTerminalStatusBatch(v.status, sessPtrs, v.rootRepoTargets()))
	}
	return tea.Batch(cmds...)
}

func (v *View) handleGitStatusComplete(msg GitStatusBatchCompleteMsg) tea.Cmd {
	v.gitStatuses.SetBatch(msg.Results)
	v.refreshing = false
	return nil
}

func (v *View) handleTerminalStatusComplete(msg TerminalStatusBatchCompleteMsg) tea.Cmd {
	if v.terminalStatuses != nil {
		for sessionID, newStatus := range msg.Results {
			if newStatus.Error != nil {
				log.Debug().Err(newStatus.Error).Str("sessionID", sessionID).Msg("terminal status update contains error")
			}
		}

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
	cmds = append(cmds, FetchTerminalStatusBatch(v.status, sessPtrs, v.rootRepoTargets()))
	if v.status.Available() {
		cmds = append(cmds, StartTerminalPollTicker(v.cfg.Tmux.PollInterval))
	}
	return tea.Batch(cmds...)
}

// rootRepoTargets collects workspace checkouts currently shown as repo headers.
func (v *View) rootRepoTargets() []hive.RootRepoTarget {
	var targets []hive.RootRepoTarget
	for _, ti := range TreeItemsAll(v.list.Items()) {
		if ti.IsHeader && ti.RootPath != "" {
			targets = append(targets, hive.RootRepoTarget{Name: ti.RepoName, Path: ti.RootPath})
		}
	}
	return targets
}

func (v *View) handlePluginWorkerStarted(msg pluginWorkerStartedMsg) tea.Cmd {
	v.pluginResultsChan = msg.resultsChan
	log.Debug().Msg("plugin background worker started")
	return listenForPluginResult(v.pluginResultsChan)
}

func (v *View) handlePluginStatusUpdate(msg pluginStatusUpdateMsg) tea.Cmd {
	if msg.Err != nil {
		log.Warn().
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
	if msg.err != nil {
		return ErrorCmd(fmt.Errorf("repo scan error: %w", msg.err))
	}
	// Rebuild the tree so headers pick up root checkout paths; the scan
	// usually completes after the initial session load has built items.
	// Fetch root statuses immediately rather than waiting for the next poll
	// tick — this is the first moment root targets exist.
	return tea.Batch(
		v.applyFilter(),
		FetchTerminalStatusBatch(v.status, nil, v.rootRepoTargets()),
	)
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

func (v *View) handleKeyMsg(msg tea.KeyPressMsg) tea.Cmd {
	_, cmd := v.handleKey(msg)
	return cmd
}

func (v *View) handleKey(msg tea.KeyPressMsg) (*View, tea.Cmd) {
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

	if v.handler.IsAction(keyStr, act.TypeNextActive) {
		v.navigateToNextActive(1)
		return v, nil
	}
	if v.handler.IsAction(keyStr, act.TypePrevActive) {
		v.navigateToNextActive(-1)
		return v, nil
	}
	if v.handler.IsAction(keyStr, act.TypeSessionsNavigateUp) {
		v.navigateSkippingPlaceholders(-1)
		return v, nil
	}
	if v.handler.IsAction(keyStr, act.TypeSessionsNavigateDown) {
		v.navigateSkippingPlaceholders(1)
		return v, nil
	}
	if v.handler.IsAction(keyStr, act.TypeSessionsRefreshGitStatuses) {
		return v, v.RefreshGitStatuses()
	}
	if v.handler.IsAction(keyStr, act.TypeSessionsTogglePreview) && v.HasTerminalIntegration() {
		v.TogglePreview()
		return v, nil
	}
	if v.handler.IsAction(keyStr, act.TypeSessionsFilterStart) {
		v.focusMode = true
		v.focusFilter = ""
		v.focusFilterInput.Reset()
		v.focusFilterInput.SetValue("")

		// Tab chrome (3) + rule + help bar (2)
		contentHeight := v.height - 5
		if contentHeight < 2 {
			contentHeight = 2
		}

		var listWidth int
		if v.previewEnabled && v.width >= 80 {
			listWidth = int(float64(v.width) * 0.25)
		} else {
			listWidth = v.width
		}

		v.list.SetSize(listWidth, contentHeight-1) // Make room for search input

		return v, v.focusFilterInput.Focus()
	}
	if v.handler.IsAction(keyStr, act.TypeSessionsCommandPaletteOpen) {
		sess := v.SelectedSession()
		return v, func() tea.Msg { return CommandPaletteRequestMsg{Session: sess} }
	}

	if v.handler.IsAction(keyStr, act.TypeNewSession) && len(v.discoveredRepos) > 0 {
		return v, func() tea.Msg { return NewSessionRequestMsg{} }
	}

	treeItem := v.SelectedTreeItem()
	if treeItem != nil && treeItem.IsRecycledPlaceholder {
		return v.handleRecycledPlaceholderKey(keyStr, treeItem)
	}

	if treeItem != nil && treeItem.IsHeader && v.handler.IsCommand(keyStr, "TmuxOpen") {
		return v.handleRepoHeaderKey(treeItem)
	}

	selected := v.SelectedSession()

	// Set target override before Resolve so TmuxOpen/TmuxStart actions
	// target the selected pane/window sub-item instead of the default.
	switch {
	case treeItem != nil && treeItem.IsPaneItem:
		v.handler.SetSelectedTarget(treeItem.PaneID)
	case treeItem != nil && treeItem.IsWindowItem:
		v.handler.SetSelectedTarget(treeItem.WindowIndex)
	default:
		v.handler.SetSelectedTarget("")
	}

	// Resolve keybinding actions before the nil-session guard so that
	// session-independent actions (GroupToggle, filters, etc.) work even
	// when the cursor is on a header row.
	resolveTarget := session.Session{}
	if selected != nil {
		resolveTarget = *selected
	}
	action, actionOK := v.handler.Resolve(keyStr, resolveTarget)
	action = MaybeOverrideWindowDelete(action, treeItem, v.renderer)
	if actionOK {
		return v, func() tea.Msg { return ActionRequestMsg{Action: action} }
	}

	if selected == nil {
		var cmd tea.Cmd
		v.list, cmd = v.list.Update(msg)
		return v, cmd
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

	var cmd tea.Cmd
	v.list, cmd = v.list.Update(msg)
	return v, cmd
}

func (v *View) handleFilterKey(msg tea.KeyPressMsg, keyStr string) (*View, tea.Cmd) {
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
	repoPath := header.RootPath
	if repoPath == "" {
		// Headers built before the workspace scan finished lack RootPath.
		for _, repo := range v.discoveredRepos {
			if git.EquivalentRemote(repo.Remote, header.RepoRemote) {
				repoPath = repo.Path
				break
			}
		}
	}
	if repoPath == "" {
		return v, nil
	}

	return v, func() tea.Msg {
		return OpenRepoRequestMsg{
			Name:   header.RepoName,
			Remote: header.RepoRemote,
			Path:   repoPath,
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

		if treeItem.IsPaneItem && isActiveStatus(treeItem.PaneStatus) {
			v.list.Select(idx)
			return
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
	if v.pendingSelectID != "" {
		sel = TreeSelection{sessionID: v.pendingSelectID}
		v.pendingSelectID = ""
	}

	allSess := v.allSessions
	filteredSess := allSess
	statusFilter := v.statusFilter
	if statusFilter != "" && v.terminalStatuses != nil {
		filtered := make([]session.Session, 0, len(allSess))
		for _, s := range allSess {
			if status, ok := v.terminalStatuses.Get(s.ID); ok {
				if statusMatchesFilter(status.Status, statusFilter) {
					filtered = append(filtered, s)
				}
			}
		}
		filteredSess = filtered
	}

	localRemote := v.localRemote

	var groups []RepoGroup
	if v.groupBy == config.GroupByGroup {
		groups = GroupSessionsByTag(filteredSess)
	} else {
		groups = GroupSessionsByRepo(filteredSess, localRemote)
	}
	items := BuildTreeItems(groups, localRemote, v.discoveredRepos)
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
	for _, ti := range TreeItemsAll(items) {
		if ti.IsHeader && ti.RootPath != "" {
			paths = append(paths, ti.RootPath)
			if !v.refreshing {
				v.gitStatuses.Set(ti.RootPath, GitStatus{IsLoading: true})
			}
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
		if ti.IsPaneItem {
			current["p\x1f"+ti.ParentSession.ID+"\x1f"+ti.ParentWindow+"\x1f"+ti.PaneID] = struct{}{}
			continue
		}
		if ti.IsWindowItem {
			current["w\x1f"+ti.ParentSession.ID+"\x1f"+ti.WindowIndex+"\x1f"+ti.WindowName] = struct{}{}
			continue
		}
		if !ti.IsSession() {
			continue
		}
		if ts, ok := v.terminalStatuses.Get(ti.Session.ID); ok && hive.ShouldExposeWindows(ts.Windows) {
			for _, w := range ts.Windows {
				expected["w\x1f"+ti.Session.ID+"\x1f"+w.WindowIndex+"\x1f"+w.WindowName] = struct{}{}
				if len(w.Panes) > 1 {
					for _, p := range w.Panes {
						expected["p\x1f"+ti.Session.ID+"\x1f"+w.WindowIndex+"\x1f"+p.PaneID] = struct{}{}
					}
				}
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
		if ti.IsWindowItem || ti.IsPaneItem {
			continue
		}
		stripped = append(stripped, items[i])
	}

	expanded := v.expandWindowItems(stripped)
	v.list.SetItems(expanded)
	v.restoreSelection(sel)
}

// expandWindowItems inserts window and pane sub-items after sessions that have
// multiple terminal windows or multiple agent panes in one window.
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
		if !ok || !hive.ShouldExposeWindows(ts.Windows) {
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
			if len(w.Panes) > 1 {
				for j, p := range w.Panes {
					expanded = append(expanded, TreeItem{
						IsPaneItem:    true,
						PaneID:        p.PaneID,
						PaneTool:      p.Tool,
						PaneStatus:    p.Status,
						ParentWindow:  w.WindowIndex,
						ParentSession: treeItem.Session,
						IsLastPane:    j == len(w.Panes)-1,
						IsLastWindow:  i == len(ts.Windows)-1,
						IsLastInRepo:  treeItem.IsLastInRepo,
						RepoPrefix:    treeItem.RepoPrefix,
					})
				}
			}
		}
	}

	return expanded
}

// --- Focus mode ---

// stopFocusMode deactivates focus mode filtering.
func (v *View) stopFocusMode() {
	v.focusMode = false
	v.focusFilter = ""

	// Recalculate: tab chrome (3) + rule + help bar (2)
	contentHeight := v.height - 5
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
	// Calculate content height: total - tab chrome (3) - rule + help bar (2)
	contentHeight := max(v.height-5, 1)

	var body string
	if v.previewEnabled && v.width >= 80 {
		body = v.renderDualColumnLayout(contentHeight)
	} else {
		// Reset delegate to show full info when not in preview mode
		v.treeDelegate.PreviewMode = false
		v.list.SetDelegate(v.treeDelegate)
		body = v.list.View()

		// Show focus mode filter input if active (at bottom to avoid layout shift)
		if v.focusMode {
			body = lipgloss.JoinVertical(lipgloss.Left, body, v.focusFilterInput.View())
		} else if v.list.SettingFilter() {
			// Fallback: show bubbles built-in filter if somehow active
			body = lipgloss.JoinVertical(lipgloss.Left, v.list.FilterInput.View(), body)
		}

		body = lipgloss.NewStyle().Height(contentHeight).Render(body)
	}

	// Common footer: rule + help bar
	bar := components.StatusBar{Width: v.width}
	help := components.KeyHints(
		components.HintNav,
		components.HintFilter,
		components.HelpEntry{Key: "enter", Desc: "select"},
		components.HintHelp,
	)

	return body + "\n" + bar.Rule() + "\n" + bar.Render(help, "")
}

// renderDualColumnLayout renders sessions list and preview side by side.
func (v *View) renderDualColumnLayout(contentHeight int) string {
	v.treeDelegate.PreviewMode = true
	v.list.SetDelegate(v.treeDelegate)

	// Calculate widths (configurable split, 1 char divider, remaining for preview)
	splitPct := v.cfg.Views.Sessions.SplitRatioOrDefault(25)
	listWidth := v.width * splitPct / 100
	if listWidth < 20 {
		listWidth = 20
	}

	// Account for divider (1 char) between list and preview
	dividerWidth := 1
	previewWidth := v.width - listWidth - dividerWidth

	selected := v.SelectedSession()
	var previewContent string

	if selected == nil {
		previewContent = v.renderRootRepoPreview(contentHeight, previewWidth)
	}

	if selected != nil { //nolint:nestif
		// Check if this is the current session (would cause recursive preview)
		isSelf := v.isCurrentTmuxSession(selected)

		// Determine pane content: prefer the selected pane, then selected window, then session fallback.
		var paneContent string
		if ps := v.selectedPaneStatus(); ps != nil {
			paneContent = ps.PaneContent
		} else if ws := v.selectedWindowStatus(); ws != nil {
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
	} else if previewContent == "" {
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
	dividerStyle := styles.TextMutedStyle

	divider := strings.Repeat("─", maxWidth)

	// Build title line: "SessionName [window] • #abcd"
	shortID := sess.ID
	if len(shortID) > 4 {
		shortID = shortID[len(shortID)-4:]
	}
	title := nameStyle.Render(sess.Name)
	// Show window/pane name if a specific sub-item is selected
	if ps := v.selectedPaneStatus(); ps != nil {
		title += " " + styles.TextMutedStyle.Render("["+displayPaneID(ps.PaneID)+"]")
	} else if ws := v.selectedWindowStatus(); ws != nil {
		title += " " + styles.TextSecondaryStyle.Render("["+ws.WindowName+"]")
	}
	title += separatorStyle.Render(" • ") + idStyle.Render("#"+shortID)

	// Build status line with colors
	var statusParts []string

	// Git status
	if gitPart := v.previewGitStatusPart(sess.Path, iconsEnabled); gitPart != "" {
		statusParts = append(statusParts, gitPart)
	}

	// Plugin statuses (neutral color)
	if v.pluginStatuses != nil {
		pluginOrder := []string{PluginGitHub}
		for _, name := range pluginOrder {
			store, ok := v.pluginStatuses[name]
			if !ok || store == nil {
				continue
			}
			status, ok := store.Get(sess.ID)
			if !ok || status.Label == "" {
				continue
			}

			icon := status.Icon
			if iconsEnabled && name == PluginGitHub {
				icon = styles.IconGithub
			}

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

// previewGitStatusPart formats the git status fragment for a preview header,
// or returns "" when no loaded status exists for the path.
func (v *View) previewGitStatusPart(path string, iconsEnabled bool) string {
	if v.gitStatuses == nil {
		return ""
	}
	status, ok := v.gitStatuses.Get(path)
	if !ok || status.IsLoading || status.Error != nil {
		return ""
	}

	branchStyle := styles.TextSecondaryStyle
	gitPart := branchStyle.Render("(")
	if iconsEnabled {
		gitPart += branchStyle.Render(styles.IconGitBranch + " ")
	}
	gitPart += branchStyle.Render(status.Branch + ")")
	gitPart += " " + styles.TextSuccessStyle.Render("+"+fmt.Sprintf("%d", status.Additions))
	gitPart += " " + styles.TextErrorStyle.Render("-"+fmt.Sprintf("%d", status.Deletions))
	if status.HasChanges && iconsEnabled {
		gitPart += " " + styles.TextWarningStyle.Render(styles.IconGit)
	}
	return gitPart
}

// renderRootRepoPreview renders the preview pane for a selected repo header's
// root checkout, or returns "" when the selection isn't a header with a
// discovered workspace checkout.
func (v *View) renderRootRepoPreview(contentHeight, previewWidth int) string {
	ti := v.SelectedTreeItem()
	if ti == nil || !ti.IsHeader || ti.RootPath == "" {
		return ""
	}

	usableWidth := previewWidth - 4
	header := v.renderRootPreviewHeader(ti, usableWidth)

	// The root repo's tmux session is named after the repo; previewing it
	// while hive runs inside it would capture hive's own pane recursively.
	if v.currentTmuxSession != "" && v.currentTmuxSession == ti.RepoName {
		return header + "\n\n(current session, preventing recursive view)"
	}

	status, ok := v.terminalStatuses.Get(hive.RootStatusKey(ti.RootPath))
	if !ok || status.PaneContent == "" {
		return header + "\n\nNo pane content available"
	}

	headerHeight := strings.Count(header, "\n") + 1
	outputHeight := max(contentHeight-headerHeight, 1)
	content := tailLines(status.PaneContent, outputHeight)
	content = truncateLines(content, usableWidth)
	return header + "\n" + content
}

// renderRootPreviewHeader renders the preview header for a root checkout:
// repo name, git status, and the checkout path in place of a session ID.
func (v *View) renderRootPreviewHeader(ti *TreeItem, maxWidth int) string {
	separatorStyle := styles.TextMutedStyle
	dividerStyle := styles.TextMutedStyle
	divider := strings.Repeat("─", max(maxWidth, 1))

	title := styles.PreviewHeaderNameStyle.Render(ti.RepoName) +
		separatorStyle.Render(" • root")

	var parts []string
	parts = append(parts, title)
	parts = append(parts, dividerStyle.Render(divider))
	status := v.previewGitStatusPart(ti.RootPath, v.cfg.TUI.IconsEnabled())
	if status != "" {
		parts = append(parts, status)
	}
	parts = append(parts, styles.TextMutedStyle.Render(ti.RootPath))
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

	if tmuxSession := sess.Metadata[session.MetaTmuxSession]; tmuxSession != "" {
		if v.currentTmuxSession == tmuxSession {
			return true
		}
	}

	return false
}

// --- Status/Filter ---

// statusMatchesFilter reports whether a published status satisfies a filter
// value. "approval" includes question: question renders at the approval tier.
func statusMatchesFilter(status terminal.Status, filter terminal.Status) bool {
	if status == filter {
		return true
	}
	return filter == terminal.StatusApproval && status == terminal.StatusQuestion
}

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

// selectedPaneStatus returns the PaneStatus for the currently selected pane item,
// or nil if a session/window is selected.
func (v *View) selectedPaneStatus() *hive.PaneStatus {
	item := v.list.SelectedItem()
	if item == nil {
		return nil
	}
	treeItem, ok := item.(TreeItem)
	if !ok || !treeItem.IsPaneItem {
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
		if ts.Windows[i].WindowIndex != treeItem.ParentWindow {
			continue
		}
		for j := range ts.Windows[i].Panes {
			if ts.Windows[i].Panes[j].PaneID == treeItem.PaneID {
				return &ts.Windows[i].Panes[j]
			}
		}
	}
	return nil
}

// selectedWindowStatus returns the WindowStatus for the currently selected window item,
// or nil if a session (not a window) is selected.
func (v *View) selectedWindowStatus() *hive.WindowStatus {
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
		repos, err := workspace.ScanRepoDirs(context.Background(), v.workspaces, v.service.Git())
		if err != nil {
			log.Warn().Err(err).Msg("repo directory scan encountered errors")
		}
		return reposDiscoveredMsg{repos: repos, err: err}
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

	for _, ti := range TreeItemsAll(items) {
		switch {
		case ti.IsSession():
			paths = append(paths, ti.Session.Path)
			v.gitStatuses.Set(ti.Session.Path, GitStatus{IsLoading: true})
		case ti.IsHeader && ti.RootPath != "":
			paths = append(paths, ti.RootPath)
			v.gitStatuses.Set(ti.RootPath, GitStatus{IsLoading: true})
		}
	}

	if len(paths) == 0 {
		return nil
	}

	return FetchGitStatusBatch(v.service.Git(), paths, v.gitWorkers)
}

// scheduleSessionRefresh returns a command that schedules the next session refresh.
func (v *View) scheduleSessionRefresh() tea.Cmd {
	interval := v.cfg.Views.Sessions.RefreshInterval
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

// SelectAtRow moves the list cursor to the item at the given row within the
// visible list area (row 0 = first visible item on the current page).
// x is the terminal column of the click; clicks in the preview pane are ignored.
func (v *View) SelectAtRow(x, row int) tea.Cmd {
	if v.previewEnabled && v.width >= 80 {
		splitPct := v.cfg.Views.Sessions.SplitRatioOrDefault(25)
		listWidth := v.width * splitPct / 100
		if x >= listWidth {
			return nil
		}
	}
	if row < 0 {
		return nil
	}
	perPage := v.list.Paginator.PerPage
	if perPage <= 0 {
		return nil
	}
	globalIndex := v.list.Paginator.Page*perPage + row
	if globalIndex >= len(v.list.VisibleItems()) {
		return nil
	}
	v.list.Select(globalIndex)
	return nil
}

// SetSize updates the view dimensions.
func (v *View) SetSize(width, height int) {
	v.width = width
	v.height = height

	// Account for tab chrome (3) + rule + help bar (2)
	contentHeight := height - 5
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

// SelectOnNextRefresh arranges for the session with the given ID to be
// selected after the next list rebuild (e.g. following session creation).
func (v *View) SelectOnNextRefresh(sessionID string) {
	v.pendingSelectID = sessionID
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
	if ti.IsPaneItem || ti.IsWindowItem {
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
func (v *View) DiscoveredRepos() []workspace.DiscoveredRepo {
	return v.discoveredRepos
}

// TerminalStatuses returns the terminal status store.
func (v *View) TerminalStatuses() *kv.Store[string, hive.TerminalStatus] {
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

// HelpSections returns view-specific help sections for the help dialog.
func (v *View) HelpSections() []components.HelpDialogSection {
	navEntries := []components.HelpEntry{
		{Key: "↑/k", Desc: "move up"},
		{Key: "↓/j", Desc: "move down"},
		{Key: "J/K", Desc: "next/prev active session"},
		{Key: "enter", Desc: "select session"},
		{Key: "/", Desc: "filter"},
		{Key: "g", Desc: "refresh git status"},
	}

	if v.previewEnabled {
		navEntries = append(navEntries, components.HelpEntry{Key: "v", Desc: "toggle preview"})
	}

	navEntries = append(navEntries, components.HelpEntry{Key: "R", Desc: "rename session"})

	sections := []components.HelpDialogSection{
		{Title: "Navigation", Entries: navEntries},
	}

	if v.handler != nil {
		actionEntries := parseHelpEntries(v.handler.HelpEntries())
		if len(actionEntries) > 0 {
			sections = append(sections, components.HelpDialogSection{
				Title:   "Actions",
				Entries: actionEntries,
			})
		}
	}

	return sections
}

// parseHelpEntries converts "key help" formatted strings to HelpEntry slices.
func parseHelpEntries(raw []string) []components.HelpEntry {
	entries := make([]components.HelpEntry, 0, len(raw))
	for _, e := range raw {
		parts := strings.SplitN(e, " ", 2)
		if len(parts) == 2 {
			entries = append(entries, components.HelpEntry{Key: parts[0], Desc: parts[1]})
		}
	}
	return entries
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
	return v.status.Available()
}

// TogglePreview toggles the preview sidebar on/off.
func (v *View) TogglePreview() {
	v.previewEnabled = !v.previewEnabled
}

// ToggleGroupBy switches between repo and group tree view modes and rebuilds the tree.
func (v *View) ToggleGroupBy() tea.Cmd {
	if v.groupBy == config.GroupByGroup {
		v.groupBy = config.GroupByRepo
	} else {
		v.groupBy = config.GroupByGroup
	}
	return v.applyFilter()
}

// GroupBy returns the current tree view grouping mode.
func (v *View) GroupBy() string {
	return v.groupBy
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
			log.Debug().Msg("plugin results channel closed")
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
