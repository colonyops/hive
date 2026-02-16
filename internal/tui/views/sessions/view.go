package sessions

import (
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/internal/core/eventbus"
	"github.com/colonyops/hive/internal/core/session"
	"github.com/colonyops/hive/internal/core/styles"
	"github.com/colonyops/hive/internal/core/terminal"
	"github.com/colonyops/hive/internal/hive"
	"github.com/colonyops/hive/internal/hive/plugins"
	"github.com/colonyops/hive/pkg/kv"
	"github.com/colonyops/hive/pkg/tmpl"
)

// ViewOpts configures a new sessions View.
type ViewOpts struct {
	Cfg             *config.Config
	Service         *hive.SessionService
	Handler         KeyResolver
	TerminalManager *terminal.Manager
	PluginManager   *plugins.Manager
	LocalRemote     string
	RepoDirs        []string
	Renderer        *tmpl.Renderer
	Bus             *eventbus.EventBus
}

// View is the Bubble Tea sub-model for the sessions tab.
type View struct {
	ctrl    *Controller
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
	width      int
	height     int
	active     bool
	refreshing bool

	// Template rendering
	renderer *tmpl.Renderer
}

// New creates a new sessions View. All KV stores, delegates, and plugins are
// initialized here so the parent Model can pass them through ViewOpts without
// constructing them itself.
func New(opts ViewOpts) *View {
	cfg := opts.Cfg

	ctrl := NewController()
	ctrl.SetLocalRemote(opts.LocalRemote)

	gitStatuses := kv.New[string, GitStatus]()
	terminalStatuses := kv.New[string, TerminalStatus]()
	columnWidths := &ColumnWidths{}

	// Initialize plugin status stores for each enabled plugin
	pluginStatuses := make(map[string]*kv.Store[string, plugins.Status])
	if opts.PluginManager != nil {
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
	l.SetShowTitle(false)
	l.SetShowFilter(false)

	// Style filter input
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

	// Minimal help keybindings
	l.AdditionalShortHelpKeys = func() []key.Binding {
		return []key.Binding{
			key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑/↓", "navigate")),
			key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		}
	}

	// Create dedicated focus filter input
	focusInput := textinput.New()
	focusInput.Prompt = "/"
	focusInputStyles := textinput.DefaultStyles(true)
	focusInputStyles.Focused.Prompt = styles.ListFilterPromptStyle
	focusInputStyles.Cursor.Color = styles.ColorPrimary
	focusInput.SetStyles(focusInputStyles)

	// Detect current tmux session to prevent recursive preview
	currentTmux := DetectCurrentTmuxSession()

	// Parse preview templates
	previewTemplates := ParsePreviewTemplates(
		cfg.TUI.Preview.TitleTemplate,
		cfg.TUI.Preview.StatusTemplate,
	)

	// Determine plugin poll interval
	pluginPollInterval := 5 * time.Second
	if cfg.Plugins.GitHub.ResultsCache > 0 {
		pluginPollInterval = cfg.Plugins.GitHub.ResultsCache
	}

	return &View{
		ctrl:    ctrl,
		cfg:     cfg,
		service: opts.Service,
		bus:     opts.Bus,

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

// Init returns the initial commands for the sessions view.
func (v *View) Init() tea.Cmd {
	// Placeholder: commands will be wired in a later step.
	return nil
}

// Update handles messages for the sessions view.
func (v *View) Update(msg tea.Msg) (*View, tea.Cmd) {
	// Placeholder: message handling will be wired in a later step.
	_ = msg
	return v, nil
}

// View renders the sessions view.
func (v *View) View() string {
	// Placeholder: rendering will be wired in a later step.
	return v.list.View()
}

// SetSize updates the view dimensions.
func (v *View) SetSize(width, height int) {
	v.width = width
	v.height = height
	v.list.SetSize(width, height)
}

// SetActive marks whether this view is the currently active tab.
func (v *View) SetActive(active bool) {
	v.active = active
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

// AllSessions returns all sessions from the controller.
func (v *View) AllSessions() []session.Session {
	return v.ctrl.AllSessions()
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
