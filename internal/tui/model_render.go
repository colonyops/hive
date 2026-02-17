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
	case m.msgView != nil && m.msgView.IsPreviewActive():
		content = m.msgView.Overlay(mainView, w, h)
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
