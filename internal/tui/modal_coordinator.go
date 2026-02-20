package tui

import (
	"context"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	lipgloss "charm.land/lipgloss/v2"

	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/internal/core/session"
	"github.com/colonyops/hive/internal/core/styles"
	"github.com/colonyops/hive/internal/tui/components"
	"github.com/colonyops/hive/internal/tui/components/form"
	tuinotify "github.com/colonyops/hive/internal/tui/notify"
	"github.com/colonyops/hive/internal/tui/views/review"
)

// ModalCoordinator owns all modal component references, pending action state,
// recycle streaming state, and provides overlay rendering plus lifecycle methods.
type ModalCoordinator struct {
	// Modal components
	Confirm         Modal
	Output          OutputModal
	NewSession      *NewSessionForm
	CommandPalette  *CommandPalette
	Help            *components.HelpDialog
	Notification    *NotificationModal
	FormDialog      *form.Dialog
	DocPicker       *review.DocumentPickerModal
	RenameInput     textinput.Model
	RenameSessionID string

	// Pending action state
	Pending                 Action
	PendingCreate           *PendingCreate
	PendingRecycledSessions []session.Session
	PendingFormCmd          config.UserCommand
	PendingFormName         string
	PendingFormSess         *session.Session
	PendingFormArgs         []string

	// Recycle streaming
	RecycleOutput <-chan string
	RecycleDone   <-chan error
	RecycleCancel context.CancelFunc

	// Sizing
	width, height int
}

// NewModalCoordinator creates a new ModalCoordinator with default state.
func NewModalCoordinator() *ModalCoordinator {
	return &ModalCoordinator{}
}

// SetSize updates the available dimensions for modal rendering.
func (mc *ModalCoordinator) SetSize(w, h int) {
	mc.width = w
	mc.height = h
}

// centeredOverlay composites a pre-rendered overlay string centered over bg.
func centeredOverlay(bg, overlay string, w, h int) string {
	overlayW := lipgloss.Width(overlay)
	overlayH := lipgloss.Height(overlay)
	bgLayer := lipgloss.NewLayer(bg)
	overlayLayer := lipgloss.NewLayer(overlay)
	overlayLayer.X((w - overlayW) / 2).Y((h - overlayH) / 2).Z(1)
	return lipgloss.NewCompositor(bgLayer, overlayLayer).Render()
}

// Overlay renders the appropriate modal overlay based on the current UI state.
// It returns the background string unchanged if no modal is active.
func (mc *ModalCoordinator) Overlay(state UIState, bg string, s spinner.Model, loadingMsg string) string {
	w, h := mc.width, mc.height
	if w == 0 {
		w = 80
	}
	if h == 0 {
		h = 24
	}

	switch {
	case state == stateRunningRecycle:
		return mc.Output.Overlay(bg, w, h)

	case state == stateCreatingSession && mc.NewSession != nil:
		formContent := lipgloss.JoinVertical(
			lipgloss.Left,
			styles.ModalTitleStyle.Render("New Session"),
			"",
			mc.NewSession.View(),
		)
		return centeredOverlay(bg, styles.ModalStyle.Render(formContent), w, h)

	case state == stateFormInput && mc.FormDialog != nil:
		formContent := lipgloss.JoinVertical(
			lipgloss.Left,
			styles.ModalTitleStyle.Render(mc.FormDialog.Title),
			"",
			mc.FormDialog.View(),
		)
		return centeredOverlay(bg, styles.FormModalStyle.Render(formContent), w, h)

	case state == stateLoading:
		loadingView := lipgloss.JoinHorizontal(lipgloss.Left, s.View(), " "+loadingMsg)
		return centeredOverlay(bg, styles.ModalStyle.Render(loadingView), w, h)

	case state == stateConfirming:
		return mc.Confirm.Overlay(bg, w, h)

	case state == stateCommandPalette && mc.CommandPalette != nil:
		return mc.CommandPalette.Overlay(bg, w, h)

	case state == stateShowingHelp && mc.Help != nil:
		return mc.Help.Overlay(bg, w, h)

	case state == stateShowingNotifications && mc.Notification != nil:
		return mc.Notification.Overlay(bg, w, h)

	case state == stateRenaming:
		renameContent := lipgloss.JoinVertical(
			lipgloss.Left,
			styles.ModalTitleStyle.Render("Rename Session"),
			"",
			mc.RenameInput.View(),
			"",
			styles.ModalHelpStyle.Render("enter: confirm • esc: cancel"),
		)
		return centeredOverlay(bg, styles.ModalStyle.Width(50).Render(renameContent), w, h)

	case mc.DocPicker != nil:
		return mc.DocPicker.Overlay(bg, w, h)

	default:
		return bg
	}
}

// ShowHelp creates and displays the help dialog.
func (mc *ModalCoordinator) ShowHelp(title string, sections []components.HelpDialogSection) {
	mc.Help = components.NewHelpDialog(title, sections, mc.width, mc.height)
}

// ShowNotifications creates and displays the notification modal.
func (mc *ModalCoordinator) ShowNotifications(bus *tuinotify.Bus) {
	mc.Notification = NewNotificationModal(bus, mc.width, mc.height)
}

// ShowConfirm creates and displays the confirmation modal.
func (mc *ModalCoordinator) ShowConfirm(title, message string) {
	mc.Confirm = NewModal(title, message)
}

// ShowOutputModal creates and displays the output modal.
func (mc *ModalCoordinator) ShowOutputModal(title string) {
	mc.Output = NewOutputModal(title)
}

// DismissHelp closes the help dialog.
func (mc *ModalCoordinator) DismissHelp() {
	mc.Help = nil
}

// DismissNotifications closes the notification modal.
func (mc *ModalCoordinator) DismissNotifications() {
	mc.Notification = nil
}

// DismissConfirm resets the confirm modal to zero value.
func (mc *ModalCoordinator) DismissConfirm() {
	mc.Confirm = Modal{}
}

// ClearFormState resets all form dialog state.
func (mc *ModalCoordinator) ClearFormState() {
	mc.FormDialog = nil
	mc.PendingFormCmd = config.UserCommand{}
	mc.PendingFormName = ""
	mc.PendingFormSess = nil
	mc.PendingFormArgs = nil
}

// HasEditorFocus returns true if a modal with text input is active.
func (mc *ModalCoordinator) HasEditorFocus(state UIState) bool {
	switch state { //nolint:exhaustive // only editor-bearing states return true
	case stateCommandPalette, stateCreatingSession, stateRenaming, stateFormInput:
		return true
	}
	return false
}
