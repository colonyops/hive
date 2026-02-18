package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"

	"github.com/colonyops/hive/internal/core/styles"
	"github.com/colonyops/hive/internal/tui/components"
)

// View renders the TUI.
func (m Model) View() tea.View {
	if m.quitting {
		return tea.NewView("")
	}

	mainView := m.renderTabView()

	content := m.modals.Overlay(m.state, mainView, m.spinner, m.loadingMessage)

	// Messages preview overlay (not managed by ModalCoordinator)
	if content == mainView && m.msgView != nil && m.msgView.IsPreviewActive() {
		w, h := m.width, m.height
		if w == 0 {
			w = 80
		}
		if h == 0 {
			h = 24
		}
		content = m.msgView.Overlay(mainView, w, h)
	}

	if m.toastController.HasToasts() {
		w, h := m.width, m.height
		if w == 0 {
			w = 80
		}
		if h == 0 {
			h = 24
		}
		content = m.toastView.Overlay(content, w, h)
	}

	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

// renderTabView renders the tab-based view layout.
func (m Model) renderTabView() string {
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
	if statusFilter := m.sessionsView.StatusFilter(); statusFilter != "" {
		filterLabel := string(statusFilter)
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
		content = m.sessionsView.View()
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
