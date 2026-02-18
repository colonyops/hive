package tui

import (
	"testing"

	"charm.land/bubbles/v2/spinner"
	"github.com/colonyops/hive/internal/tui/components"
	tuinotify "github.com/colonyops/hive/internal/tui/notify"
	"github.com/stretchr/testify/assert"
)

const testBackground = "background"

func noSpinner() spinner.Model { return spinner.New() }

func TestModalCoordinator_Overlay_NoModal(t *testing.T) {
	mc := NewModalCoordinator()
	bg := "background content"
	got := mc.Overlay(stateNormal, bg, noSpinner(), "")
	assert.Equal(t, bg, got, "no modal active should return background unchanged")
}

func TestModalCoordinator_Overlay_ConfirmModal(t *testing.T) {
	mc := NewModalCoordinator()
	mc.Confirm = NewModal("Confirm", "Are you sure?")
	bg := testBackground
	got := mc.Overlay(stateConfirming, bg, noSpinner(), "")
	assert.NotEqual(t, bg, got, "confirm modal should modify background")
	assert.Contains(t, got, "Are you sure?", "confirm modal should render confirmation message")
}

func TestModalCoordinator_Overlay_OutputModal(t *testing.T) {
	mc := NewModalCoordinator()
	mc.Output = NewOutputModal("Processing")
	bg := testBackground
	got := mc.Overlay(stateRunningRecycle, bg, noSpinner(), "")
	assert.NotEqual(t, bg, got, "output modal should modify background")
}

func TestModalCoordinator_Overlay_HelpDialog(t *testing.T) {
	mc := NewModalCoordinator()
	mc.Help = components.NewHelpDialog("Keyboard Shortcuts", []components.HelpDialogSection{
		{
			Title:   "Navigation",
			Entries: []components.HelpEntry{{Key: "q", Desc: "quit"}},
		},
	}, 80, 24)
	bg := testBackground
	got := mc.Overlay(stateShowingHelp, bg, noSpinner(), "")
	assert.NotEqual(t, bg, got, "help dialog should modify background")
}

func TestModalCoordinator_Overlay_NotificationModal(t *testing.T) {
	mc := NewModalCoordinator()
	bus := tuinotify.NewBus(nil)
	mc.Notification = NewNotificationModal(bus, 80, 24)
	bg := testBackground
	got := mc.Overlay(stateShowingNotifications, bg, noSpinner(), "")
	assert.NotEqual(t, bg, got, "notification modal should modify background")
}

func TestModalCoordinator_Overlay_NilHelp(t *testing.T) {
	mc := NewModalCoordinator()
	// Help is nil but state says showingHelp — should return bg unchanged
	bg := testBackground
	got := mc.Overlay(stateShowingHelp, bg, noSpinner(), "")
	assert.Equal(t, bg, got, "nil Help with stateShowingHelp should return background unchanged")
}

func TestModalCoordinator_Overlay_NilNotification(t *testing.T) {
	mc := NewModalCoordinator()
	bg := testBackground
	got := mc.Overlay(stateShowingNotifications, bg, noSpinner(), "")
	assert.Equal(t, bg, got, "nil Notification should return background unchanged")
}

func TestModalCoordinator_Overlay_LoadingState(t *testing.T) {
	mc := NewModalCoordinator()
	bg := testBackground
	got := mc.Overlay(stateLoading, bg, noSpinner(), "Processing...")
	assert.Contains(t, got, "Processing...", "loading state should render loading message")
}
