package tui

import (
	"fmt"
	"image/color"
	"io"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/hay-kot/hive/internal/core/session"
	"github.com/hay-kot/hive/internal/core/styles"
	"github.com/hay-kot/hive/internal/core/terminal"
	"github.com/hay-kot/hive/internal/hive/plugins"
	"github.com/hay-kot/hive/internal/tui/components"
	"github.com/hay-kot/hive/pkg/kv"
)

// Tree characters for rendering the session tree.
const (
	treeBranch = "├─"
	treeLast   = "└─"
)

// Status indicators for sessions.
const (
	statusActive   = "[●]" // green - agent actively working
	statusApproval = "[!]" // yellow - needs approval/permission
	statusReady    = "[>]" // cyan - ready for next input
	statusUnknown  = "[?]" // dim - no terminal found
	statusRecycled = "[○]" // gray - session recycled
)

// Animation constants.
const (
	// AnimationFrameCount is the total number of frames in the fade animation.
	AnimationFrameCount = 12
)

// activeAnimationColors caches the pulse colors derived from the current theme.
var (
	activeAnimationColors []color.Color
	activeAnimationSeed   uint32 // R channel of seed color, used to detect theme changes
)

// generatePulseColors creates a symmetric fade animation from a seed color.
// It dims the color to minBrightness (0.0–1.0) at the midpoint and returns to full brightness.
func generatePulseColors(base color.Color, frames int, minBrightness float64) []color.Color {
	r, g, b, _ := base.RGBA()
	br, bg, bb := float64(r>>8), float64(g>>8), float64(b>>8)

	half := frames / 2
	colors := make([]color.Color, frames)
	for i := range frames {
		// Triangle wave: 1.0 at edges, minBrightness at midpoint
		var t float64
		if i <= half {
			t = float64(i) / float64(half)
		} else {
			t = float64(frames-i) / float64(half)
		}
		scale := 1.0 - t*(1.0-minBrightness)

		colors[i] = lipgloss.Color(fmt.Sprintf("#%02x%02x%02x",
			uint8(br*scale),
			uint8(bg*scale),
			uint8(bb*scale),
		))
	}
	return colors
}

// Star indicator for current repository.
const currentRepoIndicator = "◆"

// renderStatusIndicator returns the styled status indicator for a session.
// For active sessions with terminal integration, it uses terminal status.
// For recycled sessions or when no terminal status is available, it falls back to session state.
// The animFrame parameter controls the fade animation for active status (0 to AnimationFrameCount-1).
func renderStatusIndicator(state session.State, termStatus *TerminalStatus, styles TreeDelegateStyles, animFrame int) string {
	// Recycled sessions always show recycled indicator
	if state == session.StateRecycled {
		return styles.StatusRecycled.Render(statusRecycled)
	}

	// If we have terminal status for active sessions, use it
	if state == session.StateActive && termStatus != nil {
		switch termStatus.Status {
		case terminal.StatusActive:
			return renderActiveIndicator(animFrame)
		case terminal.StatusApproval:
			return styles.StatusApproval.Render(statusApproval)
		case terminal.StatusReady:
			return styles.StatusReady.Render(statusReady)
		case terminal.StatusMissing:
			return styles.StatusUnknown.Render(statusUnknown)
		}
	}

	// Default: active session without terminal status shows as unknown
	// We only show active (green) when we have positive confirmation of activity
	if state == session.StateActive {
		return styles.StatusUnknown.Render(statusUnknown)
	}

	return styles.StatusRecycled.Render(statusRecycled)
}

// renderActiveIndicator renders the active status with fade animation.
func renderActiveIndicator(frame int) string {
	// Regenerate colors if the theme's green changed
	r, _, _, _ := styles.ColorSuccess.RGBA() //nolint:dogsled // RGBA returns 4 values, only r is needed for seed
	if activeAnimationColors == nil || activeAnimationSeed != r {
		activeAnimationColors = generatePulseColors(styles.ColorSuccess, AnimationFrameCount, 0.80)
		activeAnimationSeed = r
	}

	if frame < 0 || frame >= len(activeAnimationColors) {
		frame = 0
	}
	style := lipgloss.NewStyle().Foreground(activeAnimationColors[frame])
	return style.Render(statusActive)
}

// TreeItem represents an item in the tree view.
// It can be either a repo header, a session entry, or a recycled placeholder.
type TreeItem struct {
	// IsHeader indicates this is a repo header, not a session.
	IsHeader bool

	// Header fields (only used when IsHeader is true)
	RepoName      string
	RepoRemote    string // Git remote URL for the repo group
	IsCurrentRepo bool

	// Session fields (only used when IsHeader is false and IsRecycledPlaceholder is false)
	Session      session.Session
	IsLastInRepo bool   // Used to render └─ vs ├─
	RepoPrefix   string // The repo name for filtering purposes

	// Recycled placeholder fields (only used when IsRecycledPlaceholder is true)
	IsRecycledPlaceholder bool
	RecycledCount         int
	RecycledSessions      []session.Session // Actual recycled sessions for deletion support

	// Window sub-item fields (only used when IsWindowItem is true)
	IsWindowItem  bool
	WindowIndex   string
	WindowName    string
	ParentSession session.Session // Session this window belongs to
	IsLastWindow  bool            // For └─ vs ├─ rendering within the window group
}

// FilterValue returns the value used for filtering.
// Headers are not filterable (return empty).
// Sessions return "repoName sessionName" to allow searching by either.
// Recycled placeholders return "repoName recycled" to allow filtering.
func (i TreeItem) FilterValue() string {
	if i.IsHeader {
		return ""
	}
	if i.IsRecycledPlaceholder {
		return i.RepoPrefix + " recycled"
	}
	if i.IsWindowItem {
		return i.RepoPrefix + " " + i.ParentSession.Name + " " + i.WindowName
	}
	return i.RepoPrefix + " " + i.Session.Name
}

// BuildTreeItems converts repo groups into tree items for the list.
func BuildTreeItems(groups []RepoGroup, localRemote string) []list.Item {
	if len(groups) == 0 {
		return nil
	}

	items := make([]list.Item, 0)

	for _, group := range groups {
		// Add header
		header := TreeItem{
			IsHeader:      true,
			RepoName:      group.Name,
			RepoRemote:    group.Remote,
			IsCurrentRepo: group.Remote == localRemote,
		}
		items = append(items, header)

		// Determine if recycled placeholder will be the last item
		hasRecycled := group.RecycledCount > 0

		// Add active sessions
		for idx, s := range group.Sessions {
			isLast := idx == len(group.Sessions)-1 && !hasRecycled
			item := TreeItem{
				IsHeader:     false,
				Session:      s,
				IsLastInRepo: isLast,
				RepoPrefix:   group.Name,
			}
			items = append(items, item)
		}

		// Add recycled placeholder if there are recycled sessions
		if hasRecycled {
			placeholder := TreeItem{
				IsRecycledPlaceholder: true,
				RecycledCount:         group.RecycledCount,
				RecycledSessions:      group.RecycledSessions,
				IsLastInRepo:          true,
				RepoPrefix:            group.Name,
			}
			items = append(items, placeholder)
		}
	}

	return items
}

// TreeDelegateStyles defines the styles for the tree delegate.
type TreeDelegateStyles struct {
	// Header styles
	HeaderNormal   lipgloss.Style
	HeaderSelected lipgloss.Style
	HeaderStar     lipgloss.Style

	// Session styles
	TreeLine       lipgloss.Style
	SessionName    lipgloss.Style
	SessionBranch  lipgloss.Style
	SessionID      lipgloss.Style
	StatusActive   lipgloss.Style
	StatusApproval lipgloss.Style
	StatusReady    lipgloss.Style
	StatusUnknown  lipgloss.Style
	StatusRecycled lipgloss.Style

	// Selection styles
	Selected       lipgloss.Style
	SelectedBorder lipgloss.Style
	FilterMatch    lipgloss.Style
	SelectedMatch  lipgloss.Style
}

// DefaultTreeDelegateStyles returns the default styles for tree rendering.
func DefaultTreeDelegateStyles() TreeDelegateStyles {
	return TreeDelegateStyles{
		HeaderNormal:   lipgloss.NewStyle().Bold(true).Foreground(styles.ColorForeground),
		HeaderSelected: lipgloss.NewStyle().Bold(true).Foreground(styles.ColorPrimary),
		HeaderStar:     lipgloss.NewStyle().Foreground(styles.ColorWarning),

		TreeLine:       lipgloss.NewStyle().Foreground(styles.ColorMuted),
		SessionName:    lipgloss.NewStyle().Foreground(styles.ColorForeground),
		SessionBranch:  lipgloss.NewStyle().Foreground(styles.ColorMuted),
		SessionID:      lipgloss.NewStyle().Foreground(styles.ColorSecondary),
		StatusActive:   lipgloss.NewStyle().Foreground(styles.ColorSuccess),
		StatusApproval: lipgloss.NewStyle().Foreground(styles.ColorWarning),
		StatusReady:    lipgloss.NewStyle().Foreground(styles.ColorSecondary),
		StatusUnknown:  lipgloss.NewStyle().Foreground(styles.ColorMuted).Faint(true),
		StatusRecycled: lipgloss.NewStyle().Foreground(styles.ColorMuted),

		Selected:       lipgloss.NewStyle().Foreground(styles.ColorPrimary).Bold(true),
		SelectedBorder: lipgloss.NewStyle().Foreground(styles.ColorPrimary),
		FilterMatch:    lipgloss.NewStyle().Underline(true),
		SelectedMatch:  lipgloss.NewStyle().Underline(true).Foreground(styles.ColorPrimary).Bold(true),
	}
}

// RenderRepoHeader renders a repository header line.
func RenderRepoHeader(item TreeItem, isSelected bool, styles TreeDelegateStyles) string {
	// Repo name
	nameStyle := styles.HeaderNormal
	if isSelected {
		nameStyle = styles.HeaderSelected
	}
	result := nameStyle.Render(item.RepoName)

	// Append indicator for current repo
	if item.IsCurrentRepo {
		result += " " + styles.HeaderStar.Render(currentRepoIndicator)
	}

	return result
}

// RenderSessionLine renders a session entry with tree prefix.
func RenderSessionLine(item TreeItem, isSelected bool, gitBranch string, termStatus *TerminalStatus, styles TreeDelegateStyles, animFrame int) string {
	// Tree prefix
	var prefix string
	if item.IsLastInRepo {
		prefix = treeLast
	} else {
		prefix = treeBranch
	}
	prefixStyled := styles.TreeLine.Render(prefix)

	// Status indicator - use terminal status for active sessions
	statusStr := renderStatusIndicator(item.Session.State, termStatus, styles, animFrame)

	// Session name
	nameStyle := styles.SessionName
	if isSelected {
		nameStyle = styles.Selected
	}
	name := nameStyle.Render(item.Session.Name)

	// Branch (from git status or fallback)
	branch := ""
	if gitBranch != "" {
		branch = styles.SessionBranch.Render(" (" + gitBranch + ")")
	}

	// Short ID (last 4 chars of session ID)
	shortID := item.Session.ID
	if len(shortID) > 4 {
		shortID = shortID[len(shortID)-4:]
	}
	id := styles.SessionID.Render(" #" + shortID)

	return fmt.Sprintf("%s %s %s%s%s", prefixStyled, statusStr, name, branch, id)
}

// ColumnWidths holds the calculated widths for aligned columns.
type ColumnWidths struct {
	Name   int
	Branch int
	ID     int
}

// CalculateColumnWidths calculates the maximum widths for each column within a repo group.
func CalculateColumnWidths(sessions []session.Session, gitBranches map[string]string) ColumnWidths {
	var widths ColumnWidths

	for _, s := range sessions {
		if len(s.Name) > widths.Name {
			widths.Name = len(s.Name)
		}

		branch := gitBranches[s.Path]
		if len(branch) > widths.Branch {
			widths.Branch = len(branch)
		}

		shortID := s.ID
		if len(shortID) > 4 {
			shortID = shortID[len(shortID)-4:]
		}
		if len(shortID) > widths.ID {
			widths.ID = len(shortID)
		}
	}

	return widths
}

// PadRight pads a string to the right with spaces to reach the desired width.
func PadRight(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + components.Pad(width-len(s))
}

// TreeDelegate handles rendering of tree items in the list.
type TreeDelegate struct {
	Styles           TreeDelegateStyles
	GitStatuses      *kv.Store[string, GitStatus]
	TerminalStatuses *kv.Store[string, TerminalStatus]
	PluginStatuses   map[string]*kv.Store[string, plugins.Status] // plugin name -> session ID -> status
	ColumnWidths     *ColumnWidths
	AnimationFrame   int  // Current frame for status animations
	PreviewMode      bool // When true, show minimal info (session names only)
	IconsEnabled     bool // When true, show nerd font icons
}

// NewTreeDelegate creates a new tree delegate with default styles.
func NewTreeDelegate() TreeDelegate {
	return TreeDelegate{
		Styles: DefaultTreeDelegateStyles(),
	}
}

// Height returns the height of each item.
// Headers are 1 line, sessions are 1 line.
func (d TreeDelegate) Height() int {
	return 1
}

// Spacing returns the spacing between items.
func (d TreeDelegate) Spacing() int {
	return 0
}

// Update handles item updates.
func (d TreeDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd {
	return nil
}

// Render renders a single tree item.
func (d TreeDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	treeItem, ok := item.(TreeItem)
	if !ok {
		return
	}

	isSelected := index == m.Index()

	// Build the line content
	var line string
	switch {
	case treeItem.IsHeader:
		line = d.renderHeader(treeItem, isSelected, m, index)
	case treeItem.IsRecycledPlaceholder:
		line = d.renderRecycledPlaceholder(treeItem, isSelected)
	case treeItem.IsWindowItem:
		line = d.renderWindow(treeItem, isSelected)
	default:
		line = d.renderSession(treeItem, isSelected, m, index)
	}

	// Selection indicator
	var prefix string
	if isSelected {
		prefix = d.Styles.SelectedBorder.Render("┃") + " "
	} else {
		prefix = "  "
	}

	_, _ = fmt.Fprintf(w, "%s%s", prefix, line)
}

// renderHeader renders a repository header.
func (d TreeDelegate) renderHeader(item TreeItem, isSelected bool, _ list.Model, _ int) string {
	// Repo name
	nameStyle := d.Styles.HeaderNormal
	if isSelected {
		nameStyle = d.Styles.HeaderSelected
	}
	result := nameStyle.Render(item.RepoName)

	// Append indicator for current repo
	if item.IsCurrentRepo {
		result += " " + d.Styles.HeaderStar.Render(currentRepoIndicator)
	}

	return result
}

// renderRecycledPlaceholder renders the collapsed recycled sessions placeholder.
func (d TreeDelegate) renderRecycledPlaceholder(item TreeItem, isSelected bool) string {
	// Tree prefix
	var prefix string
	if item.IsLastInRepo {
		prefix = treeLast
	} else {
		prefix = treeBranch
	}
	prefixStyled := d.Styles.TreeLine.Render(prefix)

	// Status indicator (recycled)
	statusStr := d.Styles.StatusRecycled.Render(statusRecycled)

	// Label with count
	labelStyle := d.Styles.StatusRecycled
	if isSelected {
		labelStyle = d.Styles.Selected
	}
	label := labelStyle.Render(fmt.Sprintf("Recycled (%d)", item.RecycledCount))

	return fmt.Sprintf("%s %s %s", prefixStyled, statusStr, label)
}

// renderSession renders a session entry.
func (d TreeDelegate) renderSession(item TreeItem, isSelected bool, m list.Model, index int) string {
	// Tree prefix
	var prefix string
	if item.IsLastInRepo {
		prefix = treeLast
	} else {
		prefix = treeBranch
	}
	prefixStyled := d.Styles.TreeLine.Render(prefix)

	// Get terminal status if available
	var termStatus *TerminalStatus
	if d.TerminalStatuses != nil {
		if ts, ok := d.TerminalStatuses.Get(item.Session.ID); ok {
			termStatus = &ts
		}
	}

	// Status indicator - use terminal status for active sessions
	statusStr := renderStatusIndicator(item.Session.State, termStatus, d.Styles, d.AnimationFrame)

	// Session name with filter matching
	nameStyle := d.Styles.SessionName
	matchStyle := d.Styles.FilterMatch
	if isSelected {
		nameStyle = d.Styles.Selected
		matchStyle = d.Styles.SelectedMatch
	}

	// Apply Claude plugin style (context usage color) if present
	if d.PluginStatuses != nil {
		if claudeStore, ok := d.PluginStatuses[pluginClaude]; ok {
			if status, ok := claudeStore.Get(item.Session.ID); ok {
				// Claude plugin returns style (color) but no label/icon
				// Use Inherit to merge the color while preserving selection state
				nameStyle = nameStyle.Inherit(status.Style)
			}
		}
	}

	// Get filter matches
	matches := m.MatchesForItem(index)
	matchSet := make(map[int]bool, len(matches))
	for _, idx := range matches {
		matchSet[idx] = true
	}

	// FilterValue is "repoName sessionName", so name offset is len(repoName)+1
	nameOffset := len([]rune(item.RepoPrefix)) + 1
	name := d.renderWithMatches(item.Session.Name, nameOffset, matchSet, nameStyle, matchStyle)

	// Pad name to align columns (add spaces after styled name)
	namePadding := ""
	if d.ColumnWidths != nil && d.ColumnWidths.Name > 0 {
		padLen := d.ColumnWidths.Name - len(item.Session.Name)
		if padLen > 0 {
			namePadding = components.Pad(padLen)
		}
	}

	// Short ID (always show)
	shortID := item.Session.ID
	if len(shortID) > 4 {
		shortID = shortID[len(shortID)-4:]
	}
	id := d.Styles.SessionID.Render(" #" + shortID)

	// In preview mode, show minimal info (status + name + ID only)
	if d.PreviewMode {
		return fmt.Sprintf("%s %s %s%s", prefixStyled, statusStr, name, id)
	}

	// Full mode: show ID, git status, and plugin statuses
	gitInfo := d.renderGitStatus(item.Session.Path)
	pluginInfo := d.renderPluginStatuses(item.Session.ID)

	return fmt.Sprintf("%s %s %s%s%s%s%s", prefixStyled, statusStr, name, namePadding, id, gitInfo, pluginInfo)
}

// renderWindow renders a window sub-item nested under a session.
func (d TreeDelegate) renderWindow(item TreeItem, isSelected bool) string {
	// Deeper indent: "│     ├─" or "│     └─" depending on position
	var connector string
	if item.IsLastWindow {
		connector = treeLast
	} else {
		connector = treeBranch
	}

	// The parent session's tree line continues vertically
	var parentLine string
	if item.IsLastInRepo {
		parentLine = "    " // parent was └─, no continuing line
	} else {
		parentLine = "│   " // parent was ├─, line continues
	}
	prefixStyled := d.Styles.TreeLine.Render(parentLine + connector)

	// Get per-window terminal status from the delegate's store
	var windowStatus *WindowStatus
	if d.TerminalStatuses != nil {
		if ts, ok := d.TerminalStatuses.Get(item.ParentSession.ID); ok {
			for i := range ts.Windows {
				if ts.Windows[i].WindowIndex == item.WindowIndex {
					windowStatus = &ts.Windows[i]
					break
				}
			}
		}
	}

	// Status indicator
	var statusStr string
	if windowStatus != nil {
		termStatus := &TerminalStatus{Status: windowStatus.Status}
		statusStr = renderStatusIndicator(session.StateActive, termStatus, d.Styles, d.AnimationFrame)
	} else {
		statusStr = d.Styles.StatusUnknown.Render(statusUnknown)
	}

	// Window name
	nameStyle := d.Styles.SessionName
	if isSelected {
		nameStyle = d.Styles.Selected
	}
	name := nameStyle.Render(item.WindowName)

	// Only show a compact index when needed to disambiguate window names.
	// Also show when the window name is empty.
	showIndex := item.WindowName == ""
	if !showIndex && d.TerminalStatuses != nil {
		if ts, ok := d.TerminalStatuses.Get(item.ParentSession.ID); ok {
			for i := range ts.Windows {
				if ts.Windows[i].WindowName == item.WindowName && ts.Windows[i].WindowIndex != item.WindowIndex {
					showIndex = true
					break
				}
			}
		}
	}

	var indexStr string
	if showIndex {
		indexStr = d.Styles.SessionBranch.Render(fmt.Sprintf(" #%s", item.WindowIndex))
	}

	return fmt.Sprintf("%s %s %s%s", prefixStyled, statusStr, name, indexStr)
}

// renderGitStatus returns the formatted git status for a session path.
func (d TreeDelegate) renderGitStatus(path string) string {
	if d.GitStatuses == nil {
		return styles.TextMutedStyle.Render(" ...")
	}

	status, ok := d.GitStatuses.Get(path)
	if !ok || status.IsLoading {
		return styles.TextMutedStyle.Render(" ...")
	}

	if status.Error != nil {
		return ""
	}

	// Format with icons: ( branch) +N -N [dirty icon]
	// Format without icons: (branch) +N -N • clean/dirty
	var branch string
	if d.IconsEnabled {
		branch = d.Styles.SessionBranch.Render(" (" + styles.IconGitBranch + " " + status.Branch + ")")
	} else {
		branch = d.Styles.SessionBranch.Render(" (" + status.Branch + ")")
	}
	additions := styles.TextSuccessStyle.Render(fmt.Sprintf(" +%d", status.Additions))
	deletions := styles.TextErrorStyle.Render(fmt.Sprintf(" -%d", status.Deletions))

	var indicator string
	if d.IconsEnabled {
		// With icons: show yellow git icon for uncommitted, nothing for clean
		if status.HasChanges {
			indicator = styles.TextWarningStyle.Render(" " + styles.IconGit)
		}
	} else {
		// Without icons: show text indicator
		if status.HasChanges {
			indicator = styles.TextWarningStyle.Render(" • uncommitted")
		} else {
			indicator = styles.TextMutedStyle.Render(" • clean")
		}
	}

	return branch + additions + deletions + indicator
}

// renderPluginStatuses returns formatted plugin status indicators for a session.
// Uses neutral gray color for all plugin text.
func (d TreeDelegate) renderPluginStatuses(sessionID string) string {
	if len(d.PluginStatuses) == 0 {
		return ""
	}

	// Neutral gray style for all plugin text
	neutralStyle := lipgloss.NewStyle().Foreground(styles.ColorMuted)

	var parts []string
	pluginOrder := []string{pluginGitHub, pluginBeads, pluginClaude}
	for _, name := range pluginOrder {
		store, ok := d.PluginStatuses[name]
		if !ok || store == nil {
			continue
		}
		status, ok := store.Get(sessionID)
		if !ok || status.Label == "" {
			continue
		}

		var icon string
		if d.IconsEnabled {
			switch name {
			case pluginGitHub:
				icon = styles.IconGithub
			case pluginBeads:
				icon = styles.IconCheckList
			case pluginClaude:
				icon = styles.IconBrain
			default:
				icon = status.Icon
			}
		} else {
			icon = status.Icon
		}

		// Icon unstyled, only the label gets neutral color
		parts = append(parts, icon+neutralStyle.Render(status.Label))
	}

	if len(parts) == 0 {
		return ""
	}

	// Join all parts with bullet separators
	result := ""
	for i, part := range parts {
		if i == 0 {
			result = " " + neutralStyle.Render("•") + " " + part
		} else {
			result += " " + neutralStyle.Render("•") + " " + part
		}
	}
	return result
}

// renderWithMatches renders text with underlined characters at matched positions.
func (d TreeDelegate) renderWithMatches(text string, offset int, matchSet map[int]bool, baseStyle, matchStyle lipgloss.Style) string {
	if len(matchSet) == 0 {
		return baseStyle.Render(text)
	}

	runes := []rune(text)
	var result string
	for i, r := range runes {
		if matchSet[offset+i] {
			result += matchStyle.Render(string(r))
		} else {
			result += baseStyle.Render(string(r))
		}
	}
	return result
}
