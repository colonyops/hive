package commands

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/colonyops/hive/internal/core/kv"
	"github.com/colonyops/hive/internal/core/session"
	"github.com/colonyops/hive/internal/core/styles"
	"github.com/colonyops/hive/internal/core/terminal"
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
	items        []pickItem
	filtered     []pickItem
	cursor       int
	selected     *pickItem
	width        int
	height       int
	currentSlug  string
	termMgr      *terminal.Manager
	statuses     map[string]terminal.Status
	pollInterval time.Duration
	recentsMap   map[string]time.Time
	kvStore      kv.KV
	statusFilter string // "" = all, or "active", "approval", "ready", "missing"
}

func newPickModel(items []pickItem, currentSlug string, termMgr *terminal.Manager, pollInterval time.Duration, kvStore kv.KV, recentsMap map[string]time.Time, statusFilter string) pickModel {
	ti := textinput.New()
	ti.Placeholder = "search sessions..."
	ti.Prompt = "> "
	ti.CharLimit = 64

	inputStyles := textinput.DefaultStyles(true)
	inputStyles.Cursor.Color = styles.ColorPrimary
	inputStyles.Focused.Placeholder = lipgloss.NewStyle().Foreground(styles.ColorMuted)
	ti.SetStyles(inputStyles)

	m := pickModel{
		input:        ti,
		items:        items,
		filtered:     items,
		currentSlug:  currentSlug,
		termMgr:      termMgr,
		statuses:     make(map[string]terminal.Status),
		pollInterval: pollInterval,
		kvStore:      kvStore,
		recentsMap:   recentsMap,
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
		return m, tea.Batch(refreshStatusCmd(m.termMgr, m.items), tickCmd(m.pollInterval))

	case statusRefreshMsg:
		m.statuses = msg.statuses
		if msg.items != nil {
			m.items = msg.items
			m.applyFilter()
		}
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
			statusCycle := []string{"", "active", "approval", "ready", "missing"}
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
			if strings.Contains(strings.ToLower(item.DisplayName()), query) {
				filtered = append(filtered, item)
			}
		}
		m.filtered = filtered
	}

	if m.statusFilter != "" {
		var statusFiltered []pickItem
		for _, item := range m.filtered {
			s := m.statuses[item.statusKey()]
			if string(s) == m.statusFilter {
				statusFiltered = append(statusFiltered, item)
			}
		}
		m.filtered = statusFiltered
	}

	sortItems(m.filtered, m.statuses, m.recentsMap)

	// Clamp cursor
	if m.cursor >= len(m.filtered) {
		m.cursor = max(len(m.filtered)-1, 0)
	}
}

func sortItems(items []pickItem, statuses map[string]terminal.Status, recentsMap map[string]time.Time) {
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		aRecent := recentsMap[a.Session.ID]
		bRecent := recentsMap[b.Session.ID]

		// Recents first
		aIsRecent := !aRecent.IsZero()
		bIsRecent := !bRecent.IsZero()
		if aIsRecent != bIsRecent {
			return aIsRecent
		}
		if aIsRecent && bIsRecent {
			return aRecent.After(bRecent) // most recent first
		}

		// Then by status priority
		aStatus := statuses[a.statusKey()]
		bStatus := statuses[b.statusKey()]
		return statusSortOrder(aStatus) < statusSortOrder(bStatus)
	})
}

func statusSortOrder(s terminal.Status) int {
	switch s {
	case terminal.StatusApproval:
		return 0
	case terminal.StatusActive:
		return 1
	case terminal.StatusReady:
		return 2
	case terminal.StatusMissing:
		return 3
	default:
		return 4
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

	if len(m.filtered) == 0 {
		if len(m.items) == 0 {
			b.WriteString(styles.TextMutedStyle.Render("  No active sessions"))
		} else {
			b.WriteString(styles.TextMutedStyle.Render("  No matches"))
		}
		b.WriteString("\n")
	} else {
		for i, item := range m.filtered {
			// Cursor indicator
			if i == m.cursor {
				b.WriteString(styles.TextPrimaryStyle.Render("► "))
			} else {
				b.WriteString("  ")
			}

			// Status indicator from live polling
			status := terminal.StatusMissing
			if s, ok := m.statuses[item.statusKey()]; ok {
				status = s
			}
			indicator := styles.RenderStatusIndicator(status)

			// Session name
			name := item.DisplayName()

			// Short ID: last 4 chars
			id := item.Session.ID
			if len(id) > 4 {
				id = id[len(id)-4:]
			}
			shortID := styles.TextMutedStyle.Render("#" + id)

			if item.IsCurrent {
				b.WriteString(styles.TextMutedStyle.Render(fmt.Sprintf("%s %s  %s", indicator, name, shortID)))
			} else {
				fmt.Fprintf(&b, "%s %s  %s", indicator, name, shortID)
			}

			if item.IsRecent {
				fmt.Fprintf(&b, " %s", styles.TextWarningStyle.Render("◆"))
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
				for k, v := range item.Session.Metadata {
					metadata[k] = v
				}
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
			validStatuses := map[string]bool{"active": true, "approval": true, "ready": true, "missing": true}
			if flagStatus != "" && !validStatuses[flagStatus] {
				return fmt.Errorf("invalid --status %q: valid values are active, approval, ready, missing", flagStatus)
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

			m := newPickModel(items, currentSlug, cmd.app.Terminal, cmd.app.Config.Tmux.PollInterval, cmd.app.KV, recents, flagStatus)
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
// If windowName is non-empty, it also selects that window after switching.
func switchTmux(name string, windowName string) error {
	if strings.TrimSpace(os.Getenv("TMUX")) != "" {
		cmd := exec.Command("tmux", "switch-client", "-t", name)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return err
		}
	} else {
		cmd := exec.Command("tmux", "attach-session", "-t", name)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return err
		}
	}

	if windowName != "" {
		// Best-effort window selection after switching
		cmd := exec.Command("tmux", "select-window", "-t", name+":"+windowName)
		_ = cmd.Run()
	}
	return nil
}
