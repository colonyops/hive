package commands

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/colonyops/hive/internal/core/session"
	"github.com/colonyops/hive/internal/core/styles"
	"github.com/colonyops/hive/internal/core/terminal"
	"github.com/colonyops/hive/internal/core/tmux"
	"github.com/urfave/cli/v3"
)

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
}

func newPickModel(items []pickItem, currentSlug string, termMgr *terminal.Manager, pollInterval time.Duration) pickModel {
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

	// Clamp cursor
	if m.cursor >= len(m.filtered) {
		m.cursor = max(len(m.filtered)-1, 0)
	}
}

func (m pickModel) View() tea.View {
	var b strings.Builder

	// Input line
	b.WriteString(m.input.View())
	b.WriteString("\n")

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

			b.WriteString("\n")
		}
	}

	// Help line
	b.WriteString("\n")
	b.WriteString(styles.TextMutedStyle.Render("  ↑↓ navigate · enter select · esc cancel"))

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
				}
				statuses[windowItem.statusKey()] = wStatus
				expanded = append(expanded, windowItem)
			}
		}

		return statusRefreshMsg{statuses: statuses, items: expanded}
	}
}

func (cmd *ExperimentalCmd) pickCmd() *cli.Command {
	return &cli.Command{
		Name:  "pick",
		Usage: "Fuzzy-pick a session and switch tmux to it",
		Action: func(ctx context.Context, c *cli.Command) error {
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
				items = append(items, pickItem{
					Session:   s,
					IsCurrent: s.Slug == currentSlug,
				})
			}

			m := newPickModel(items, currentSlug, cmd.app.Terminal, cmd.app.Config.Tmux.PollInterval)
			p := tea.NewProgram(m)
			finalModel, err := p.Run()
			if err != nil {
				return fmt.Errorf("running picker: %w", err)
			}

			result, ok := finalModel.(pickModel)
			if !ok || result.selected == nil {
				return nil
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
