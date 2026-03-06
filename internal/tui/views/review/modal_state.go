package review

import (
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"

	"github.com/colonyops/hive/internal/core/styles"
	"github.com/colonyops/hive/internal/tui/components"
)

// ModalState coordinates modal lifecycle and rendering.
type ModalState struct {
	commentModal      *CommentModal
	confirmModal      *components.ConfirmModal
	finalizationModal *FinalizationModal
}

// NewModalState creates a new ModalState instance.
func NewModalState() ModalState {
	return ModalState{}
}

// HasActiveModal returns true if any modal is currently active.
func (ms *ModalState) HasActiveModal() bool {
	return ms.commentModal != nil ||
		ms.confirmModal != nil ||
		ms.finalizationModal != nil
}

// ActiveModal returns the current active modal interface, or nil if none.
func (ms *ModalState) ActiveModal() Modal {
	if ms.finalizationModal != nil {
		return ms.finalizationModal
	}
	if ms.commentModal != nil {
		return ms.commentModal
	}
	return nil
}

// Update routes update messages to the active modal.
func (ms ModalState) Update(msg tea.Msg) (ModalState, tea.Cmd) {
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
func (ms *ModalState) RenderOverlay(background string, width, height int) string {
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

	modalW := lipgloss.Width(modal)
	modalH := lipgloss.Height(modal)
	x := (width - modalW) / 2
	y := (height - modalH) / 2

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
