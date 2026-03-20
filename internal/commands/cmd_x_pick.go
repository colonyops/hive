package commands

import (
	"context"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/colonyops/hive/internal/core/git"
	"github.com/colonyops/hive/internal/core/kv"
	"github.com/colonyops/hive/internal/core/session"
	"github.com/colonyops/hive/internal/core/styles"
	"github.com/colonyops/hive/internal/core/terminal"
	terminaltmux "github.com/colonyops/hive/internal/core/terminal/tmux"
	"github.com/colonyops/hive/internal/core/tmux"
	"github.com/colonyops/hive/pkg/iojson"
	"github.com/urfave/cli/v3"
)

const (
	recentsTTL       = 30 * time.Minute
	recentsKeyPrefix = "picker.recent."
)

func loadRecents(ctx context.Context, kvStore kv.KV) map[string]time.Time {
	keys, err := kvStore.ListKeys(ctx)
	if err != nil {
		return nil
	}
	recents := make(map[string]time.Time)
	for _, key := range keys {
		if !strings.HasPrefix(key, recentsKeyPrefix) {
			continue
		}
		sessionID := strings.TrimPrefix(key, recentsKeyPrefix)
		entry, err := kvStore.GetRaw(ctx, key)
		if err != nil {
			continue
		}
		recents[sessionID] = entry.UpdatedAt
	}
	return recents
}

// pickItem represents a selectable item in the session picker.
type pickItem struct {
	Session     session.Session
	WindowName  string // non-empty = window row (Phase 3)
	WindowIndex string // tmux window index (Phase 3)
	IsRecent    bool   // Phase 4
	IsCurrent   bool   // current tmux session
}

// DisplayName returns the display string for this item.
func (p pickItem) DisplayName() string {
	if p.WindowName != "" {
		return p.Session.Name + "/" + p.WindowName
	}
	return p.Session.Name
}

// statusKey returns the map key used for status lookups.
// Window items use "sessionID/windowIndex", others use "sessionID".
func (p pickItem) statusKey() string {
	if p.WindowIndex != "" {
		return p.Session.ID + "/" + p.WindowIndex
	}
	return p.Session.ID
}

// statusTickMsg triggers a terminal status refresh cycle.
type statusTickMsg struct{}

// statusRefreshMsg carries refreshed terminal statuses and expanded items.
type statusRefreshMsg struct {
	statuses map[string]terminal.Status // keyed by statusKey()
	items    []pickItem                 // expanded item list (nil = no change)
}

// pickModel is the Bubble Tea model for the session picker.
type pickModel struct {
	input        textinput.Model
	baseItems    []pickItem // original per-session items, used for polling
	items        []pickItem // display items (may include expanded window sub-items)
	filtered     []pickItem
	cursor       int
	selected     *pickItem
	width        int
	height       int
	currentSlug  string
	termMgr      *terminal.Manager
	statuses     map[string]terminal.Status
	statusLoaded bool // true after first status poll completes
	pollInterval time.Duration
	statusFilter string // "" = all, or "active", "approval", "ready", "missing"
}

func newPickModel(baseItems, items []pickItem, currentSlug string, termMgr *terminal.Manager, pollInterval time.Duration, recentsMap map[string]time.Time, statusFilter string) pickModel {
	ti := textinput.New()
	ti.Prompt = "/ "
	ti.CharLimit = 64

	inputStyles := textinput.DefaultStyles(true)
	inputStyles.Cursor.Color = styles.ColorPrimary
	inputStyles.Focused.Placeholder = lipgloss.NewStyle().Foreground(styles.ColorMuted)
	ti.SetStyles(inputStyles)
	ti.Focus() // Must focus after SetStyles; Init() has value receiver so focus there is lost

	// Sort once: top 3 recents, then alphabetical. This order stays stable.
	sortItemsInitial(baseItems, recentsMap)
	sortItemsInitial(items, recentsMap)

	m := pickModel{
		input:        ti,
		baseItems:    baseItems,
		items:        items,
		filtered:     items,
		currentSlug:  currentSlug,
		termMgr:      termMgr,
		statuses:     make(map[string]terminal.Status),
		pollInterval: pollInterval,
		statusFilter: statusFilter,
	}

	return m
}

func (m pickModel) Init() tea.Cmd {
	return tea.Batch(m.input.Focus(), tickCmd(m.pollInterval))
}

func (m pickModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case statusTickMsg:
		return m, tea.Batch(refreshStatusCmd(m.termMgr, m.baseItems), tickCmd(m.pollInterval))

	case statusRefreshMsg:
		m.statuses = msg.statuses
		m.statusLoaded = true
		if msg.items != nil {
			m.items = msg.items
		}
		m.applyFilter()
		return m, nil

	case tea.KeyPressMsg:
		switch msg.String() {
		case "up", "ctrl+k":
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil
		case "down", "ctrl+j":
			if m.cursor < len(m.filtered)-1 {
				m.cursor++
			}
			return m, nil
		case "enter":
			if len(m.filtered) > 0 && m.cursor < len(m.filtered) {
				item := m.filtered[m.cursor]
				m.selected = &item
			}
			return m, tea.Quit
		case "tab":
			// "" = hide missing (default), "all" = show everything, then specific statuses
			statusCycle := []string{"", "all", "active", "approval", "ready", "missing"}
			idx := 0
			for i, s := range statusCycle {
				if s == m.statusFilter {
					idx = i
					break
				}
			}
			m.statusFilter = statusCycle[(idx+1)%len(statusCycle)]
			m.applyFilter()
			return m, nil
		case "esc", "ctrl+c":
			return m, tea.Quit
		}
	}

	// Forward to textinput
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)

	// Re-filter on text change
	m.applyFilter()

	return m, cmd
}

// applyFilter filters items by case-insensitive substring match on DisplayName.
func (m *pickModel) applyFilter() {
	query := strings.ToLower(m.input.Value())
	if query == "" {
		m.filtered = m.items
	} else {
		var filtered []pickItem
		for _, item := range m.items {
			searchable := strings.ToLower(git.ExtractRepoName(item.Session.Remote) + " " + item.DisplayName())
			if strings.Contains(searchable, query) {
				filtered = append(filtered, item)
			}
		}
		m.filtered = filtered
	}

	switch m.statusFilter {
	case "all":
		// Show everything, no further filtering
	case "":
		// Default: hide sessions with no tmux session (StatusMissing).
		// If statuses haven't loaded yet (empty map), show all items.
		// Always keep the current session: while the picker is open its pane
		// shows no AI output, so detection returns StatusMissing even though
		// the tmux session clearly exists.
		if len(m.statuses) > 0 {
			var visible []pickItem
			for _, item := range m.filtered {
				if item.IsCurrent || m.statuses[item.statusKey()] != terminal.StatusMissing {
					visible = append(visible, item)
				}
			}
			m.filtered = visible
		}
	default:
		// Specific status filter
		var statusFiltered []pickItem
		for _, item := range m.filtered {
			if string(m.statuses[item.statusKey()]) == m.statusFilter {
				statusFiltered = append(statusFiltered, item)
			}
		}
		m.filtered = statusFiltered
	}

	// Clamp cursor
	if m.cursor >= len(m.filtered) {
		m.cursor = max(len(m.filtered)-1, 0)
	}
}

const maxRecents = 3

// sortItemsInitial sorts items once at startup: top N recents first (by time),
// then everything else alphabetically. This order is stable across status polls.
func sortItemsInitial(items []pickItem, recentsMap map[string]time.Time) {
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		aRecent := recentsMap[a.Session.ID]
		bRecent := recentsMap[b.Session.ID]
		aIsRecent := !aRecent.IsZero()
		bIsRecent := !bRecent.IsZero()

		if aIsRecent != bIsRecent {
			return aIsRecent
		}
		if aIsRecent && bIsRecent {
			return aRecent.After(bRecent)
		}

		// Alphabetical by name
		return strings.ToLower(a.DisplayName()) < strings.ToLower(b.DisplayName())
	})

	// Cap IsRecent to top N
	recentCount := 0
	for i := range items {
		if items[i].IsRecent {
			recentCount++
			if recentCount > maxRecents {
				items[i].IsRecent = false
			}
		}
	}
}

func (m pickModel) View() tea.View {
	var b strings.Builder

	// Input line
	b.WriteString(m.input.View())
	b.WriteString("\n")

	if m.statusFilter != "" {
		b.WriteString(styles.TextMutedStyle.Render(fmt.Sprintf("  [filter: %s]", m.statusFilter)))
		b.WriteString("\n")
	}

	switch {
	case !m.statusLoaded:
		b.WriteString(styles.TextMutedStyle.Render("  Loading..."))
		b.WriteString("\n")
	case len(m.filtered) == 0:
		if len(m.items) == 0 {
			b.WriteString(styles.TextMutedStyle.Render("  No active sessions"))
		} else {
			b.WriteString(styles.TextMutedStyle.Render("  No matches"))
		}
		b.WriteString("\n")
	default:
		// Layout: "› [>] name···        repo #abcd ◆"
		// Fixed-width parts: cursor(2) + status(3) + spaces(3) + #id(5) = 13
		// Right side (repo + #id) is right-aligned; name fills remaining space.
		const fixedWidth = 13 // cursor(2) + status(3) + 3 gaps + #id(5)

		for i, item := range m.filtered {
			isSelected := i == m.cursor

			// Cursor indicator (Secondary/cyan, matching tree view's SelectedBorder)
			if isSelected {
				b.WriteString(styles.TextSecondaryStyle.Render(styles.IconSelector) + " ")
			} else {
				b.WriteString("  ")
			}

			// Status indicator from live polling
			status := terminal.StatusMissing
			if s, ok := m.statuses[item.statusKey()]; ok {
				status = s
			}
			indicator := styles.RenderStatusIndicator(status)

			name := item.DisplayName()
			repo := git.ExtractRepoName(item.Session.Remote)
			id := item.Session.ID
			if len(id) > 4 {
				id = id[len(id)-4:]
			}

			// Diamond goes after name, so account for it in name column width
			recentSuffix := ""
			recentWidth := 0
			if item.IsRecent {
				recentSuffix = " ◆"
				recentWidth = 2
			}

			// Right-side text (repo + #id), right-aligned
			rightText := repo + " #" + id
			rightWidth := len(rightText)

			// Available width for the name + recent indicator
			availWidth := max(m.width-fixedWidth-rightWidth-recentWidth, 10)

			// Truncate name if needed
			if len(name) > availWidth {
				name = name[:availWidth-1] + "…"
			}
			namePad := strings.Repeat(" ", max(availWidth-len(name), 0))

			// Selected item: name in Primary+Bold (matches tree view Selected style)
			nameStyle := styles.TextForegroundStyle
			if isSelected {
				nameStyle = styles.TextPrimaryBoldStyle
			}

			if item.IsCurrent {
				mutedStyle := lipgloss.NewStyle().Foreground(styles.ColorMuted).Faint(true)
				fmt.Fprintf(&b, "%s %s", indicator, mutedStyle.Render(name))
				if recentSuffix != "" {
					b.WriteString(styles.TextWarningStyle.Render(recentSuffix))
				}
				fmt.Fprintf(&b, "%s %s %s", namePad, mutedStyle.Render(repo), mutedStyle.Render("#"+id))
			} else {
				fmt.Fprintf(&b, "%s %s", indicator, nameStyle.Render(name))
				if recentSuffix != "" {
					b.WriteString(styles.TextWarningStyle.Render(recentSuffix))
				}
				fmt.Fprintf(&b, "%s %s %s", namePad, styles.TextMutedStyle.Render(repo), styles.TextSecondaryStyle.Render("#"+id))
			}

			b.WriteString("\n")
		}
	}

	// Help line
	b.WriteString("\n")
	b.WriteString(styles.TextMutedStyle.Render("  ↑↓ navigate · tab filter · enter select · esc cancel"))

	v := tea.NewView(b.String())
	v.AltScreen = true
	return v
}

// tickCmd returns a command that sends a statusTickMsg after the given duration.
func tickCmd(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg {
		return statusTickMsg{}
	})
}

// refreshStatusCmd returns a command that refreshes terminal statuses for all items.
// It also expands multi-window sessions into individual window rows.
func refreshStatusCmd(mgr *terminal.Manager, items []pickItem) tea.Cmd {
	return func() tea.Msg {
		if mgr == nil || !mgr.HasEnabledIntegrations() {
			return statusRefreshMsg{items: items}
		}
		mgr.RefreshAll()

		statuses := make(map[string]terminal.Status)
		var expanded []pickItem
		ctx := context.Background()

		for _, item := range items {
			// Skip window sub-items; we only expand from base session items
			if item.WindowIndex != "" {
				continue
			}

			metadata := item.Session.Metadata
			if item.Session.Path != "" {
				metadata = make(map[string]string, len(item.Session.Metadata)+1)
				maps.Copy(metadata, item.Session.Metadata)
				metadata["_session_path"] = item.Session.Path
			}

			info, integration, err := mgr.DiscoverSession(ctx, item.Session.Slug, metadata)
			if err != nil || info == nil || integration == nil {
				statuses[item.Session.ID] = terminal.StatusMissing
				expanded = append(expanded, item)
				continue
			}

			// Get overall status
			status, sErr := integration.GetStatus(ctx, info)
			if sErr != nil {
				status = terminal.StatusMissing
			}

			// Try multi-window expansion
			disc, ok := integration.(terminal.AllWindowsDiscoverer)
			if !ok {
				statuses[item.Session.ID] = status
				expanded = append(expanded, item)
				continue
			}

			allInfos, dErr := disc.DiscoverAllWindows(ctx, item.Session.Slug, metadata)
			if dErr != nil || len(allInfos) <= 1 {
				// Single window or error — keep as single item
				statuses[item.Session.ID] = status
				expanded = append(expanded, item)
				continue
			}

			// Multi-window: expand into individual window rows
			for _, wi := range allInfos {
				wStatus, wErr := integration.GetStatus(ctx, wi)
				if wErr != nil {
					wStatus = terminal.StatusMissing
				}

				windowItem := pickItem{
					Session:     item.Session,
					WindowName:  wi.WindowName,
					WindowIndex: wi.Pane,
					IsCurrent:   item.IsCurrent,
					IsRecent:    item.IsRecent,
				}
				statuses[windowItem.statusKey()] = wStatus
				expanded = append(expanded, windowItem)
			}
		}

		return statusRefreshMsg{statuses: statuses, items: expanded}
	}
}

func (cmd *ExperimentalCmd) pickCmd() *cli.Command {
	var (
		flagStatus      string
		flagRepo        string
		flagPrint       bool
		flagFormat      string
		flagHideCurrent bool
		flagNoRecents   bool
	)

	return &cli.Command{
		Name:  "pick",
		Usage: "Fuzzy-pick a session and switch tmux to it",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "status", Aliases: []string{"s"}, Usage: "Filter by terminal status (active, approval, ready, missing)", Destination: &flagStatus},
			&cli.StringFlag{Name: "repo", Aliases: []string{"r"}, Usage: "Filter by repository remote URL (substring match)", Destination: &flagRepo},
			&cli.BoolFlag{Name: "print", Aliases: []string{"p"}, Usage: "Print selected session info instead of switching tmux", Destination: &flagPrint},
			&cli.StringFlag{Name: "format", Aliases: []string{"f"}, Value: "id", Usage: "Output format for --print (id, name, path, json)", Destination: &flagFormat},
			&cli.BoolFlag{Name: "hide-current", Usage: "Hide the current tmux session from the list", Destination: &flagHideCurrent},
			&cli.BoolFlag{Name: "no-recents", Usage: "Don't prioritize recently-used sessions", Destination: &flagNoRecents},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			// Validate flags
			validStatuses := map[string]bool{"all": true, "active": true, "approval": true, "ready": true, "missing": true}
			if flagStatus != "" && !validStatuses[flagStatus] {
				return fmt.Errorf("invalid --status %q: valid values are all, active, approval, ready, missing", flagStatus)
			}

			validFormats := map[string]bool{"id": true, "name": true, "path": true, "json": true}
			if !validFormats[flagFormat] {
				return fmt.Errorf("invalid --format %q: valid values are id, name, path, json", flagFormat)
			}

			sessions, err := cmd.app.Sessions.ListSessions(ctx)
			if err != nil {
				return fmt.Errorf("listing sessions: %w", err)
			}

			// Filter to active sessions only
			var items []pickItem
			currentSlug := tmux.DetectCurrentTmuxSession()

			for _, s := range sessions {
				if s.State != session.StateActive {
					continue
				}
				if flagRepo != "" && !strings.Contains(strings.ToLower(s.Remote), strings.ToLower(flagRepo)) {
					continue
				}
				if flagHideCurrent && s.Slug == currentSlug {
					continue
				}
				items = append(items, pickItem{
					Session:   s,
					IsCurrent: s.Slug == currentSlug,
				})
			}

			var recents map[string]time.Time
			if !flagNoRecents {
				recents = loadRecents(ctx, cmd.app.KV)
				for i := range items {
					if _, ok := recents[items[i].Session.ID]; ok {
						items[i].IsRecent = true
					}
				}
			}

			// Create terminal manager (same as TUI) since cmd.app.Terminal is nil at app level
			termMgr := terminal.NewManager([]string{"tmux"})
			tmuxIntegration := terminaltmux.New(cmd.app.Config.Tmux.PreviewWindowMatcher)
			if tmuxIntegration.Available() {
				termMgr.Register(tmuxIntegration)
			}

			// Pre-fetch statuses synchronously so the first render has data.
			// Keep baseItems as the original per-session slice; refreshStatusCmd
			// uses these as polling inputs and must not receive window sub-items
			// (it skips them, causing multi-window sessions to vanish after the
			// first tick).
			baseItems := items
			initialRefresh := refreshStatusCmd(termMgr, baseItems)().(statusRefreshMsg)
			displayItems := baseItems
			if initialRefresh.items != nil {
				displayItems = initialRefresh.items
			}

			m := newPickModel(baseItems, displayItems, currentSlug, termMgr, cmd.app.Config.Tmux.PollInterval, recents, flagStatus)
			m.statuses = initialRefresh.statuses
			m.statusLoaded = true
			m.applyFilter()
			p := tea.NewProgram(m)
			finalModel, err := p.Run()
			if err != nil {
				return fmt.Errorf("running picker: %w", err)
			}

			result, ok := finalModel.(pickModel)
			if !ok || result.selected == nil {
				return nil
			}

			_ = cmd.app.KV.SetTTL(ctx, recentsKeyPrefix+result.selected.Session.ID, result.selected.Session.Name, recentsTTL)

			if flagPrint {
				switch flagFormat {
				case "id":
					_, err = fmt.Fprintln(c.Root().Writer, result.selected.Session.ID)
				case "name":
					_, err = fmt.Fprintln(c.Root().Writer, result.selected.Session.Name)
				case "path":
					_, err = fmt.Fprintln(c.Root().Writer, result.selected.Session.Path)
				case "json":
					err = iojson.WriteLine(c.Root().Writer, result.selected.Session)
				}
				return err
			}

			slug := result.selected.Session.Slug
			return switchTmux(slug, result.selected.WindowName)
		},
	}
}

// switchTmux switches to or attaches the named tmux session.
// If windowName is non-empty, it selects that window before attaching so the
// correct window is visible on entry (attach-session blocks until detach, so
// any post-attach select-window would never run outside tmux).
func switchTmux(name string, windowName string) error {
	target := name
	if windowName != "" {
		target = name + ":" + windowName
	}

	if strings.TrimSpace(os.Getenv("TMUX")) != "" {
		cmd := exec.Command("tmux", "switch-client", "-t", target)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	if windowName != "" {
		// Select the window before attaching; attach-session blocks until detach.
		cmd := exec.Command("tmux", "select-window", "-t", target)
		_ = cmd.Run()
	}

	cmd := exec.Command("tmux", "attach-session", "-t", name)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
