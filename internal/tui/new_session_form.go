package tui

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/colonyops/hive/internal/core/session"
	"github.com/colonyops/hive/internal/core/styles"
	"github.com/colonyops/hive/internal/core/workspace"
)

// agentPicker is a compact inline selector for a small set of agent names.
// It renders as a single line of options: "claude  ·  opencode  ·  pi"
// with the selected option highlighted. Left/right (or h/l) cycle the selection.
type agentPicker struct {
	keys     []string
	selected int
	focused  bool
}

func (p *agentPicker) focus() { p.focused = true }
func (p *agentPicker) blur()  { p.focused = false }

func (p *agentPicker) update(msg tea.Msg) {
	if !p.focused {
		return
	}
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return
	}
	switch key.String() {
	case "left", "h":
		if p.selected > 0 {
			p.selected--
		} else {
			p.selected = len(p.keys) - 1
		}
	case "right", "l":
		p.selected = (p.selected + 1) % len(p.keys)
	}
}

func (p agentPicker) view() string {
	sep := styles.TextMutedStyle.Render("  ·  ")
	parts := make([]string, len(p.keys))
	for i, k := range p.keys {
		if i == p.selected {
			parts[i] = styles.TextPrimaryBoldStyle.Render(k)
		} else {
			parts[i] = styles.TextMutedStyle.Render(k)
		}
	}
	return strings.Join(parts, sep)
}

// Field index constants when the agent selector is active.
// Agent sits at the top (0) but is not in the forward tab cycle.
// Initial focus is on repo (1).
const (
	agentSelectorField = 0
	repoFieldWithAgent = 1
	nameFieldWithAgent = 2
)

// NewSessionForm manages the new session creation form.
type NewSessionForm struct {
	repos         []workspace.DiscoveredRepo
	existingNames map[string]bool

	repoSelect SelectField
	nameInput  textinput.Model

	hasAgentSelector bool
	agent            agentPicker

	// focusedField tracks which field has focus.
	// Without agent selector: 0=repo, 1=name
	// With agent selector:    0=agent, 1=repo, 2=name  (initial focus: 1)
	focusedField int
	submitted    bool
	cancelled    bool
	nameError    string
}

// NewSessionFormResult contains the form submission result.
type NewSessionFormResult struct {
	Repo        workspace.DiscoveredRepo
	SessionName string
	AgentKey    string // empty when agent selector is disabled
}

// NewNewSessionForm creates a new session form with the given repos.
// If preselectedRemote is non-empty, the matching repo will be pre-selected.
// existingNames is used to validate that the session name is unique.
// agentKeys, when non-empty, adds a compact agent selector at the top of the form.
// The default agent (index 0 in agentKeys) is pre-selected; it is skipped in the
// forward tab cycle and reachable via shift+tab from the repo field.
func NewNewSessionForm(repos []workspace.DiscoveredRepo, preselectedRemote string, existingNames map[string]bool, agentKeys []string) *NewSessionForm {
	selectedIdx := 0
	for i, r := range repos {
		if r.Remote == preselectedRemote {
			selectedIdx = i
			break
		}
	}

	items := make([]SelectItem, len(repos))
	for i, r := range repos {
		items[i] = SelectItem{label: r.Name, value: i}
	}

	repoSelect := NewSelectField("Repository", items, selectedIdx)
	repoSelect.Focus()

	nameInput := textinput.New()
	nameInput.Placeholder = "my-feature-branch"
	nameInput.CharLimit = 64
	nameInput.Prompt = ""
	nameInput.SetWidth(40)
	nameInput.KeyMap.Paste.SetEnabled(true)

	inputStyles := textinput.DefaultStyles(true)
	inputStyles.Cursor.Color = styles.ColorPrimary
	inputStyles.Focused.Placeholder = lipgloss.NewStyle().Foreground(styles.ColorMuted)
	inputStyles.Blurred.Placeholder = lipgloss.NewStyle().Foreground(styles.ColorMuted)
	nameInput.SetStyles(inputStyles)

	f := &NewSessionForm{
		repos:         repos,
		existingNames: existingNames,
		repoSelect:    repoSelect,
		nameInput:     nameInput,
		focusedField:  0, // repo is 0 without agent, 1 with agent
	}

	if len(agentKeys) > 0 {
		f.hasAgentSelector = true
		f.agent = agentPicker{keys: agentKeys}
		f.focusedField = repoFieldWithAgent // start on repo, not agent
	}

	return f
}

// repoIdx returns the focusedField value for the repo field.
func (f *NewSessionForm) selectAgent(key string) {
	if !f.hasAgentSelector || key == "" {
		return
	}
	for i, agentKey := range f.agent.keys {
		if agentKey == key {
			f.agent.selected = i
			return
		}
	}
}

func (f *NewSessionForm) repoIdx() int {
	if f.hasAgentSelector {
		return repoFieldWithAgent
	}
	return 0
}

// nameIdx returns the focusedField value for the name input.
func (f *NewSessionForm) nameIdx() int {
	if f.hasAgentSelector {
		return nameFieldWithAgent
	}
	return 1
}

// Init returns the initial command for the form.
func (f *NewSessionForm) Init() tea.Cmd { return nil }

// Update handles messages for the form.
func (f *NewSessionForm) Update(msg tea.Msg) (NewSessionForm, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		return f.handleKey(keyMsg)
	}
	return f.updateFocusedField(msg)
}

// handleKey processes key events for the form.
func (f *NewSessionForm) handleKey(msg tea.KeyPressMsg) (NewSessionForm, tea.Cmd) {
	key := msg.String()

	switch key {
	case "tab":
		return f.tabForward()

	case "shift+tab":
		return f.tabBackward()

	case "enter":
		if f.focusedField == f.nameIdx() {
			return f.validateAndSubmit()
		}
		// Advance to next field in forward order (same as tab, but enter on
		// agent goes to repo rather than wrapping).
		return f.tabForward()

	case "esc":
		if f.focusedField == f.repoIdx() && f.repoSelect.IsFiltering() {
			return f.updateFocusedField(msg)
		}
		f.cancelled = true
		return *f, nil
	}

	return f.updateFocusedField(msg)
}

// tabForward advances focus to the next field (wraps around).
// With agent selector: agent(0) → repo(1) → name(2) → agent(0)
// Without:            repo(0)  → name(1) → repo(0)
func (f *NewSessionForm) tabForward() (NewSessionForm, tea.Cmd) {
	next := f.focusedField + 1
	if next > f.nameIdx() {
		next = 0
	}
	return f.moveFocus(next)
}

// tabBackward moves focus to the previous field (wraps around).
func (f *NewSessionForm) tabBackward() (NewSessionForm, tea.Cmd) {
	prev := f.focusedField - 1
	if prev < 0 {
		prev = f.nameIdx()
	}
	return f.moveFocus(prev)
}

// moveFocus transitions focus to the given field index.
func (f *NewSessionForm) moveFocus(idx int) (NewSessionForm, tea.Cmd) {
	f.repoSelect.Blur()
	f.nameInput.Blur()
	f.agent.blur()
	f.focusedField = idx

	if f.hasAgentSelector && idx == agentSelectorField {
		f.agent.focus()
		return *f, nil
	}
	if idx == f.repoIdx() {
		cmd := f.repoSelect.Focus()
		return *f, cmd
	}
	cmd := f.nameInput.Focus()
	return *f, cmd
}

// updateFocusedField routes messages to the currently focused field.
func (f *NewSessionForm) updateFocusedField(msg tea.Msg) (NewSessionForm, tea.Cmd) {
	var cmd tea.Cmd
	if f.hasAgentSelector && f.focusedField == agentSelectorField {
		f.agent.update(msg)
		return *f, nil
	}
	if f.focusedField == f.repoIdx() {
		f.repoSelect, cmd = f.repoSelect.Update(msg)
		return *f, cmd
	}
	// name field
	prev := f.nameInput.Value()
	f.nameInput, cmd = f.nameInput.Update(msg)
	if f.nameInput.Value() != prev {
		f.nameError = ""
	}
	return *f, cmd
}

// validateAndSubmit validates the name field and marks the form submitted.
func (f *NewSessionForm) validateAndSubmit() (NewSessionForm, tea.Cmd) {
	name := f.nameInput.Value()
	if name == "" {
		f.nameError = "Session name is required"
		return *f, nil
	}
	if err := session.ValidateName(name); err != nil {
		f.nameError = err.Error()
		return *f, nil
	}
	if f.existingNames[name] {
		f.nameError = "Session name already exists"
		return *f, nil
	}
	f.submitted = true
	return *f, nil
}

// Submitted returns true if the form was submitted.
func (f *NewSessionForm) Submitted() bool { return f.submitted }

// Cancelled returns true if the form was cancelled.
func (f *NewSessionForm) Cancelled() bool { return f.cancelled }

// Result returns the form result. Only valid if Submitted() is true.
func (f *NewSessionForm) Result() NewSessionFormResult {
	if len(f.repos) == 0 {
		return NewSessionFormResult{}
	}
	idx := f.repoSelect.SelectedIndex()
	if idx < 0 || idx >= len(f.repos) {
		idx = 0
	}

	agentKey := ""
	if f.hasAgentSelector && f.agent.selected >= 0 && f.agent.selected < len(f.agent.keys) {
		agentKey = f.agent.keys[f.agent.selected]
	}

	return NewSessionFormResult{
		Repo:        f.repos[idx],
		SessionName: f.nameInput.Value(),
		AgentKey:    agentKey,
	}
}

// View renders the form.
func (f *NewSessionForm) View() string {
	var sections []string

	// Agent picker — title line then options line
	if f.hasAgentSelector {
		agentFocused := f.focusedField == agentSelectorField
		titleStyle := styles.TextMutedStyle
		if agentFocused {
			titleStyle = styles.FormTitleStyle
		}
		title := titleStyle.Render("Agent")
		agentSection := styles.FormFieldStyle.Render(
			lipgloss.JoinVertical(lipgloss.Left, title, f.agent.view()),
		)
		sections = append(sections, agentSection, "")
	}

	// Repository selector
	sections = append(sections, f.repoSelect.View())

	// Session name input
	nameFocused := f.focusedField == f.nameIdx()
	nameTitleStyle := styles.TextMutedStyle
	if nameFocused {
		nameTitleStyle = styles.FormTitleStyle
	}
	nameTitle := nameTitleStyle.Render("Session Name")
	nameContent := lipgloss.JoinVertical(lipgloss.Left, nameTitle, f.nameInput.View())

	errText := f.nameError
	if errText == "" {
		errText = " "
	}
	errorView := styles.TextErrorStyle.Width(f.nameInput.Width()).Render(errText)
	nameContent = lipgloss.JoinVertical(lipgloss.Left, nameContent, errorView)

	inputBorderStyle := styles.FormFieldStyle
	if nameFocused {
		inputBorderStyle = styles.FormFieldFocusedStyle
	}
	sections = append(sections, "", inputBorderStyle.Render(nameContent))

	helpText := styles.TextMutedStyle.Render("tab: switch fields • enter: submit • esc: cancel")
	sections = append(sections, "", helpText)

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}
