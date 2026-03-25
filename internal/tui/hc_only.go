package tui

import (
	"context"
	"fmt"
	"maps"
	"os/exec"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	act "github.com/colonyops/hive/internal/core/action"
	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/internal/core/hc"
	corekv "github.com/colonyops/hive/internal/core/kv"
	"github.com/colonyops/hive/internal/core/notify"
	"github.com/colonyops/hive/internal/core/styles"
	"github.com/colonyops/hive/internal/hive"
	"github.com/colonyops/hive/internal/tui/components"
	"github.com/colonyops/hive/internal/tui/views/tasks"
	"github.com/colonyops/hive/pkg/tmpl"
)

// HoneycombOnlyOptions configures the honeycomb-only TUI.
type HoneycombOnlyOptions struct {
	Honeycomb *hive.HoneycombService
	RepoKey   string
	Config    *config.Config
	KVStore   corekv.KV
	Renderer  *tmpl.Renderer
}

// hcHeaderLines is the number of lines occupied by the header (rule + title + rule).
const hcHeaderLines = 3

// HoneycombOnlyModel is a minimal TUI for browsing and mutating honeycomb tasks.
type HoneycombOnlyModel struct {
	tasksView       *tasks.View
	handler         *KeybindingResolver
	mergedCmds      map[string]config.UserCommand
	copyCmd         string
	width, height   int
	quitting        bool
	helpDialog      *components.HelpDialog
	confirmModal    *components.ConfirmModal
	commandPalette  *CommandPalette
	pendingCmd      tea.Cmd
	toastController *ToastController
	toastView       *ToastView
}

// NewHoneycombOnly creates a new honeycomb-only TUI model.
func NewHoneycombOnly(opts HoneycombOnlyOptions) HoneycombOnlyModel {
	cfg := opts.Config
	viewKBs := map[string]map[string]config.Keybinding{
		"global": cfg.Views.Global.Keybindings,
		"tasks":  cfg.Views.Tasks.Keybindings,
	}
	mergedCmds := config.DefaultUserCommands()
	maps.Copy(mergedCmds, cfg.UserCommands)
	handler := NewKeybindingResolver(viewKBs, mergedCmds, opts.Renderer)
	handler.SetActiveView(ViewTasks)

	tasksView := tasks.New(opts.Honeycomb, opts.RepoKey, handler, opts.KVStore, cfg.Views.Tasks.SplitRatio)
	toastCtrl := NewToastController()
	return HoneycombOnlyModel{
		tasksView:       tasksView,
		handler:         handler,
		mergedCmds:      mergedCmds,
		copyCmd:         cfg.CopyCommand,
		toastController: toastCtrl,
		toastView:       NewToastView(toastCtrl),
	}
}

// Init implements tea.Model.
func (m HoneycombOnlyModel) Init() tea.Cmd {
	return tea.Batch(m.tasksView.Init(), scheduleToastTick())
}

// Update implements tea.Model.
func (m HoneycombOnlyModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.tasksView.SetSize(msg.Width, max(msg.Height-hcHeaderLines, 1))
		m.tasksView.SetActive(true)
		return m, nil

	case toastTickMsg:
		m.toastController.Tick()
		return m, scheduleToastTick()

	case tasks.CommandPaletteRequestMsg:
		m.commandPalette = NewCommandPalette(m.taskCommands(), nil, m.width, m.height, ViewTasks)
		return m, nil

	case tea.KeyPressMsg:
		if m.commandPalette != nil {
			if msg.String() == keyCtrlC {
				m.quitting = true
				return m, tea.Quit
			}
			var cmd tea.Cmd
			m.commandPalette, cmd = m.commandPalette.Update(msg)
			if entry, _, ok := m.commandPalette.SelectedCommand(); ok {
				m.commandPalette = nil
				if entry.Command.Action != "" {
					return m.handleTaskAction(act.Action{Type: entry.Command.Action})
				}
				// Shell commands require session context not available in standalone mode.
				return m, nil
			}
			if m.commandPalette.Cancelled() {
				m.commandPalette = nil
				return m, nil
			}
			return m, cmd
		}
		if m.helpDialog != nil {
			switch msg.String() {
			case keyCtrlC:
				m.quitting = true
				return m, tea.Quit
			case "esc", "?", "q":
				m.helpDialog = nil
			case "j", "down":
				m.helpDialog.ScrollDown()
			case "k", "up":
				m.helpDialog.ScrollUp()
			}
			return m, nil
		}
		if m.confirmModal != nil {
			// Allow quitting even while a confirmation is pending.
			if msg.String() == keyCtrlC || msg.String() == "q" {
				m.quitting = true
				return m, tea.Quit
			}
			modal, cmd := m.confirmModal.Update(msg)
			m.confirmModal = &modal
			if m.confirmModal.Confirmed() {
				pendingCmd := m.pendingCmd
				m.confirmModal = nil
				m.pendingCmd = nil
				return m, tea.Batch(pendingCmd, cmd)
			}
			if m.confirmModal.Cancelled() {
				m.confirmModal = nil
				m.pendingCmd = nil
			}
			return m, cmd
		}
		switch msg.String() {
		case keyCtrlC, "q":
			m.quitting = true
			return m, tea.Quit
		case "esc":
			if m.toastController.HasToasts() {
				m.toastController.Dismiss()
			}
			return m, nil
		case "?":
			sections := m.tasksView.HelpSections()
			sections = append(sections, components.HelpDialogSection{
				Title: "General",
				Entries: []components.HelpEntry{
					{Key: "?", Desc: "toggle help"},
					{Key: "q", Desc: "quit"},
				},
			})
			m.helpDialog = components.NewHelpDialog("Keyboard Shortcuts", sections, m.width, m.height)
			return m, nil
		}

	case tasks.ActionRequestMsg:
		return m.handleTaskAction(msg.Action)

	case tasks.TaskActionCompleteMsg:
		if msg.Err != nil {
			m.toastController.Push(notify.Notification{
				Level:   notify.LevelError,
				Message: msg.Err.Error(),
			})
			return m, scheduleToastTick()
		}
		return m, func() tea.Msg { return tasks.RefreshTasksMsg{} }
	}

	cmd := m.tasksView.Update(msg)
	return m, cmd
}

// taskCommands returns commands that are valid in the standalone honeycomb TUI.
// Non-task action commands (e.g. HiveDoctor, ThemePreview) are excluded since
// they require infrastructure only present in the full TUI.
func (m HoneycombOnlyModel) taskCommands() map[string]config.UserCommand {
	filtered := make(map[string]config.UserCommand, len(m.mergedCmds))
	for name, cmd := range m.mergedCmds {
		if cmd.Action != "" && !isTaskAction(cmd.Action) {
			continue
		}
		filtered[name] = cmd
	}
	return filtered
}

// handleTaskAction dispatches based on action type.
func (m HoneycombOnlyModel) handleTaskAction(a act.Action) (tea.Model, tea.Cmd) {
	if a.Err != nil {
		return m, nil
	}

	switch a.Type {
	case act.TypeTasksRefresh:
		return m, func() tea.Msg { return tasks.RefreshTasksMsg{} }
	case act.TypeTasksFilter:
		return m, m.tasksView.CycleFilter()
	case act.TypeTasksCopyID:
		if item := m.tasksView.SelectedItem(); item != nil {
			if err := m.copyToClipboard(item.ID); err != nil {
				m.toastController.Push(notify.Notification{
					Level:   notify.LevelError,
					Message: fmt.Sprintf("copy failed: %v", err),
				})
			} else {
				m.toastController.Push(notify.Notification{
					Level:   notify.LevelInfo,
					Message: fmt.Sprintf("Copied %s", item.ID),
				})
			}
		}
		return m, scheduleToastTick()
	case act.TypeTasksTogglePreview:
		return m, m.tasksView.TogglePreview()
	case act.TypeTasksSelectRepo:
		// Not supported in standalone mode.
		return m, nil
	case act.TypeTasksSetOpen:
		return m, m.taskUpdateStatus(hc.StatusOpen)
	case act.TypeTasksSetInProgress:
		return m, m.taskUpdateStatus(hc.StatusInProgress)
	case act.TypeTasksSetDone:
		return m, m.taskUpdateStatus(hc.StatusDone)
	case act.TypeTasksSetCancelled:
		item := m.tasksView.SelectedItem()
		if item == nil {
			return m, nil
		}
		svc := m.tasksView.Svc()
		if svc == nil {
			return m, nil
		}
		id, title := item.ID, item.Title
		modal := components.NewConfirmModal(fmt.Sprintf("Cancel %q?", title))
		m.confirmModal = &modal
		m.pendingCmd = func() tea.Msg {
			status := hc.StatusCancelled
			_, err := svc.UpdateItem(context.Background(), id, hc.ItemUpdate{Status: &status})
			return tasks.TaskActionCompleteMsg{Err: err}
		}
		return m, nil
	case act.TypeTasksDelete:
		item := m.tasksView.SelectedItem()
		if item == nil {
			return m, nil
		}
		svc := m.tasksView.Svc()
		if svc == nil {
			return m, nil
		}
		id, title := item.ID, item.Title
		modal := components.NewConfirmModal(fmt.Sprintf("Delete %q? This cannot be undone.", title))
		m.confirmModal = &modal
		m.pendingCmd = func() tea.Msg {
			err := svc.DeleteItem(context.Background(), id)
			return tasks.TaskActionCompleteMsg{Err: err}
		}
		return m, nil
	case act.TypeTasksPrune:
		svc := m.tasksView.Svc()
		if svc == nil {
			return m, nil
		}
		repoKey := m.tasksView.RepoKey()
		modal := components.NewConfirmModal("Remove all done/cancelled items older than 24h?")
		m.confirmModal = &modal
		m.pendingCmd = func() tea.Msg {
			_, err := svc.Prune(context.Background(), hc.PruneOpts{
				OlderThan: 24 * time.Hour,
				Statuses:  []hc.Status{hc.StatusDone, hc.StatusCancelled},
				RepoKey:   repoKey,
			})
			return tasks.TaskActionCompleteMsg{Err: err}
		}
		return m, nil
	default:
		return m, nil
	}
}

// taskUpdateStatus returns a command that updates the selected task's status.
func (m HoneycombOnlyModel) taskUpdateStatus(status hc.Status) tea.Cmd {
	item := m.tasksView.SelectedItem()
	if item == nil {
		return nil
	}
	svc := m.tasksView.Svc()
	if svc == nil {
		return nil
	}
	id := item.ID
	return func() tea.Msg {
		_, err := svc.UpdateItem(context.Background(), id, hc.ItemUpdate{Status: &status})
		return tasks.TaskActionCompleteMsg{Err: err}
	}
}

// copyToClipboard copies text using the configured copy command.
func (m HoneycombOnlyModel) copyToClipboard(text string) error {
	if m.copyCmd == "" {
		return nil
	}
	parts := strings.Fields(m.copyCmd)
	if len(parts) == 0 {
		return nil
	}
	cmd := exec.Command(parts[0], parts[1:]...)
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}

// renderHeader builds the 3-line header: rule + title bar + rule.
func (m HoneycombOnlyModel) renderHeader() string {
	w := m.width
	if w <= 0 {
		w = 80
	}
	rule := styles.TextMutedStyle.Render(strings.Repeat("─", w))

	tab := styles.ViewSelectedStyle.Render("Tasks")
	branding := styles.TabBrandingStyle.Render(styles.IconHive + " Honeycomb")
	tabWidth := lipgloss.Width(tab)
	brandingWidth := lipgloss.Width(branding)
	spacerWidth := max(w-tabWidth-brandingWidth-2, 1) // 2 = left+right margins
	title := components.Pad(1) + tab + components.Pad(spacerWidth) + branding + components.Pad(1)

	return rule + "\n" + title + "\n" + rule
}

// View implements tea.Model.
func (m HoneycombOnlyModel) View() tea.View {
	if m.quitting {
		return tea.NewView("")
	}
	content := m.renderHeader() + "\n" + m.tasksView.View()
	if m.confirmModal != nil {
		content += "\n" + m.confirmModal.View()
	}
	if m.commandPalette != nil {
		content = m.commandPalette.Overlay(content, m.width, m.height)
	}
	content = m.toastView.Overlay(content, m.width, m.height)
	if m.helpDialog != nil {
		content = m.helpDialog.Overlay(content, m.width, m.height)
	}
	return tea.NewView(content)
}
