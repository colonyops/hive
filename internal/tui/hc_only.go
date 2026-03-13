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
	tasksView     *tasks.View
	handler       *KeybindingResolver
	copyCmd       string
	width, height int
	quitting      bool
	helpDialog    *components.HelpDialog
	// inline confirmation state
	confirming    bool
	confirmMsg    string
	pendingAct    act.Action
	pendingItemID string
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
	return HoneycombOnlyModel{
		tasksView: tasksView,
		handler:   handler,
		copyCmd:   cfg.CopyCommand,
	}
}

// Init implements tea.Model.
func (m HoneycombOnlyModel) Init() tea.Cmd {
	return m.tasksView.Init()
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

	case tea.KeyPressMsg:
		if m.helpDialog != nil {
			switch msg.String() {
			case "ctrl+c":
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
		if m.confirming {
			switch msg.String() {
			case "y", "Y":
				m.confirming = false
				cmd := m.executeConfirmedAction()
				m.pendingAct = act.Action{}
				m.pendingItemID = ""
				m.confirmMsg = ""
				return m, cmd
			default:
				m.confirming = false
				m.pendingAct = act.Action{}
				m.pendingItemID = ""
				m.confirmMsg = ""
				return m, nil
			}
		}
		switch msg.String() {
		case "ctrl+c", "q":
			m.quitting = true
			return m, tea.Quit
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
			return m, nil
		}
		return m, func() tea.Msg { return tasks.RefreshTasksMsg{} }
	}

	cmd := m.tasksView.Update(msg)
	return m, cmd
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
			_ = m.copyToClipboard(item.ID)
		}
		return m, nil
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
		m.pendingItemID = item.ID
		m.pendingAct = a
		m.confirmMsg = fmt.Sprintf("Cancel %q? (y/n)", item.Title)
		m.confirming = true
		return m, nil
	case act.TypeTasksDelete:
		item := m.tasksView.SelectedItem()
		if item == nil {
			return m, nil
		}
		m.pendingItemID = item.ID
		m.pendingAct = a
		m.confirmMsg = fmt.Sprintf("Delete %q? This cannot be undone. (y/n)", item.Title)
		m.confirming = true
		return m, nil
	case act.TypeTasksPrune:
		m.pendingItemID = ""
		m.pendingAct = a
		m.confirmMsg = "Remove all done/cancelled items older than 24h? (y/n)"
		m.confirming = true
		return m, nil
	default:
		return m, nil
	}
}

// executeConfirmedAction executes the stored pending action after user confirmation.
func (m HoneycombOnlyModel) executeConfirmedAction() tea.Cmd {
	svc := m.tasksView.Svc()
	a := m.pendingAct
	id := m.pendingItemID
	repoKey := m.tasksView.RepoKey()

	switch a.Type {
	case act.TypeTasksSetCancelled:
		return func() tea.Msg {
			status := hc.StatusCancelled
			_, err := svc.UpdateItem(context.Background(), id, hc.ItemUpdate{Status: &status})
			return tasks.TaskActionCompleteMsg{Err: err}
		}
	case act.TypeTasksDelete:
		return func() tea.Msg {
			err := svc.DeleteItem(context.Background(), id)
			return tasks.TaskActionCompleteMsg{Err: err}
		}
	case act.TypeTasksPrune:
		return func() tea.Msg {
			_, err := svc.Prune(context.Background(), hc.PruneOpts{
				OlderThan: 24 * time.Hour,
				Statuses:  []hc.Status{hc.StatusDone, hc.StatusCancelled},
				RepoKey:   repoKey,
			})
			return tasks.TaskActionCompleteMsg{Err: err}
		}
	default:
		return nil
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
	if m.confirming {
		content += "\n" + m.confirmMsg
	}
	if m.helpDialog != nil {
		content = m.helpDialog.Overlay(content, m.width, m.height)
	}
	return tea.NewView(content)
}
