package review

import (
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"

	"github.com/hay-kot/hive/internal/core/styles"
	"github.com/hay-kot/hive/internal/tui/components"
)

// ModalState coordinates modal lifecycle and rendering.
type ModalState struct {
	commentModal      *CommentModal
	confirmModal      *components.ConfirmModal
	finalizationModal *FinalizationModal
	pickerModal       *DocumentPickerModal
}

// NewModalState creates a new ModalState instance.
func NewModalState() ModalState {
	return ModalState{
		commentModal:      nil,
		confirmModal:      nil,
		finalizationModal: nil,
		pickerModal:       nil,
	}
}

// HasActiveModal returns true if any modal is currently active.
func (ms *ModalState) HasActiveModal() bool {
	return ms.commentModal != nil ||
		ms.confirmModal != nil ||
		ms.finalizationModal != nil ||
		ms.pickerModal != nil
}

// ActiveModal returns the current active modal interface, or nil if none.
// Priority: picker > finalization > confirm > comment
func (ms *ModalState) ActiveModal() Modal {
	if ms.pickerModal != nil {
		// Note: DocumentPickerModal doesn't implement Modal interface
		return nil
	}
	if ms.finalizationModal != nil {
		return ms.finalizationModal
	}
	if ms.confirmModal != nil {
		// Note: components.ConfirmModal doesn't implement our Modal interface
		return nil
	}
	if ms.commentModal != nil {
		return ms.commentModal
	}
	return nil
}

// Update routes update messages to the active modal.
// Returns the updated ModalState and any commands.
func (ms ModalState) Update(msg tea.Msg) (ModalState, tea.Cmd) {
	// Priority: picker > finalization > confirm > comment
	// Only update the highest-priority active modal

	if ms.pickerModal != nil {
		modal, cmd := ms.pickerModal.Update(msg)
		ms.pickerModal = modal
		return ms, cmd
	}

	if ms.finalizationModal != nil {
		modal, cmd := ms.finalizationModal.Update(msg)
		ms.finalizationModal = &modal
		return ms, cmd
	}

	if ms.confirmModal != nil {
		modal, cmd := ms.confirmModal.Update(msg)
		ms.confirmModal = &modal
		return ms, cmd
	}

	if ms.commentModal != nil {
		modal, cmd := ms.commentModal.Update(msg)
		ms.commentModal = &modal
		return ms, cmd
	}

	return ms, nil
}

// RenderOverlay overlays the active modal on the background.
// Returns the background with modal overlay, or just background if no modal is active.
func (ms *ModalState) RenderOverlay(background string, width, height int) string {
	// Priority: picker > finalization > confirm > comment

	if ms.pickerModal != nil {
		return ms.pickerModal.Overlay(background, width, height)
	}

	if ms.finalizationModal != nil {
		return ms.finalizationModal.Overlay(background)
	}

	if ms.confirmModal != nil {
		return ms.renderCenteredModal(ms.confirmModal.View(), background, width, height)
	}

	if ms.commentModal != nil {
		return ms.renderCenteredModal(ms.commentModal.View(), background, width, height)
	}

	return background
}

// renderCenteredModal renders a modal centered on the background.
func (ms *ModalState) renderCenteredModal(modalContent, background string, width, height int) string {
	modal := styles.ReviewOverlayModalStyle.Render(modalContent)

	// Center the modal
	modalW := lipgloss.Width(modal)
	modalH := lipgloss.Height(modal)
	x := (width - modalW) / 2
	y := (height - modalH) / 2

	// Use compositor to overlay modal
	bgLayer := lipgloss.NewLayer(background)
	modalLayer := lipgloss.NewLayer(modal)
	modalLayer.X(x).Y(y).Z(1)

	compositor := lipgloss.NewCompositor(bgLayer, modalLayer)
	return compositor.Render()
}

// CloseAll closes all active modals.
func (ms *ModalState) CloseAll() {
	ms.commentModal = nil
	ms.confirmModal = nil
	ms.finalizationModal = nil
	ms.pickerModal = nil
}

// ShowCommentModal sets the comment modal as active.
func (ms *ModalState) ShowCommentModal(modal *CommentModal) {
	ms.commentModal = modal
}

// ShowConfirmModal sets the confirm modal as active.
func (ms *ModalState) ShowConfirmModal(modal *components.ConfirmModal) {
	ms.confirmModal = modal
}

// ShowFinalizationModal sets the finalization modal as active.
func (ms *ModalState) ShowFinalizationModal(modal *FinalizationModal) {
	ms.finalizationModal = modal
}

// ShowPickerModal sets the picker modal as active.
func (ms *ModalState) ShowPickerModal(modal *DocumentPickerModal) {
	ms.pickerModal = modal
}

// CommentModal returns the active comment modal, or nil if not active.
func (ms *ModalState) CommentModal() *CommentModal {
	return ms.commentModal
}

// ConfirmModal returns the active confirm modal, or nil if not active.
func (ms *ModalState) ConfirmModal() *components.ConfirmModal {
	return ms.confirmModal
}

// FinalizationModal returns the active finalization modal, or nil if not active.
func (ms *ModalState) FinalizationModal() *FinalizationModal {
	return ms.finalizationModal
}

// PickerModal returns the active picker modal, or nil if not active.
func (ms *ModalState) PickerModal() *DocumentPickerModal {
	return ms.pickerModal
}
