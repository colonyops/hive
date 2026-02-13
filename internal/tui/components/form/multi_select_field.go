package form

import (
	"io"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/hay-kot/hive/internal/core/styles"
)

// MultiSelectField is a multi-select form field with checkbox toggles.
type MultiSelectField struct {
	list    list.Model
	options []string
	checked map[int]bool
	label_  string
	focused bool
}

// multiSelectDelegate renders items with checkbox state.
type multiSelectDelegate struct {
	checked *map[int]bool
}

func (d multiSelectDelegate) Height() int                             { return 1 }
func (d multiSelectDelegate) Spacing() int                            { return 0 }
func (d multiSelectDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d multiSelectDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	item, ok := listItem.(selectItem)
	if !ok {
		return
	}

	isHighlighted := index == m.Index()

	check := "[ ] "
	if (*d.checked)[index] {
		check = "[x] "
	}

	style := styles.TextForegroundStyle
	cursor := "  "
	if isHighlighted {
		style = styles.SelectFieldItemSelectedStyle
		cursor = "> "
	}

	_, _ = io.WriteString(w, cursor+style.Render(check+item.label))
}

// NewMultiSelectFormField creates a multi-select field from static options.
func NewMultiSelectFormField(label string, options []string) *MultiSelectField {
	items := make([]list.Item, len(options))
	for i, opt := range options {
		items[i] = selectItem{label: opt, index: i}
	}

	const maxVisible = 8
	height := max(min(len(options), maxVisible), 1)

	checked := make(map[int]bool)
	delegate := multiSelectDelegate{checked: &checked}

	l := list.New(items, delegate, 40, height)
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.SetShowFilter(false)
	l.SetShowHelp(false)
	l.SetShowPagination(len(options) > maxVisible)
	l.Styles.TitleBar = lipgloss.NewStyle()

	l.FilterInput.Prompt = "/ "
	filterStyles := textinput.DefaultStyles(true)
	filterStyles.Focused.Prompt = styles.TextPrimaryStyle
	filterStyles.Cursor.Color = styles.ColorPrimary
	l.FilterInput.SetStyles(filterStyles)

	return &MultiSelectField{
		list:    l,
		options: options,
		checked: checked,
		label_:  label,
	}
}

func (f *MultiSelectField) Update(msg tea.Msg) (Field, tea.Cmd) {
	if !f.focused {
		return f, nil
	}

	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		if keyMsg.String() == "space" {
			idx := f.list.Index()
			f.checked[idx] = !f.checked[idx]
			return f, nil
		}
	}

	var cmd tea.Cmd
	f.list, cmd = f.list.Update(msg)
	return f, cmd
}

func (f *MultiSelectField) View() string {
	titleStyle := styles.TextMutedStyle
	if f.focused {
		titleStyle = styles.FormTitleStyle
	}
	title := titleStyle.Render(f.label_)

	var content string
	if f.list.SettingFilter() {
		content = lipgloss.JoinVertical(lipgloss.Left,
			title,
			f.list.FilterInput.View(),
			f.list.View(),
		)
	} else {
		content = lipgloss.JoinVertical(lipgloss.Left, title, f.list.View())
	}

	borderStyle := styles.FormFieldStyle
	if f.focused {
		borderStyle = styles.FormFieldFocusedStyle
	}

	return borderStyle.Render(content)
}

func (f *MultiSelectField) Focus() tea.Cmd {
	f.focused = true
	return nil
}

func (f *MultiSelectField) Blur() {
	f.focused = false
}

func (f *MultiSelectField) Focused() bool { return f.focused }
func (f *MultiSelectField) Label() string { return f.label_ }

// Value returns the selected options as []string.
func (f *MultiSelectField) Value() any {
	var selected []string
	for i, opt := range f.options {
		if f.checked[i] {
			selected = append(selected, opt)
		}
	}
	return selected
}

// SelectedIndices returns the indices of checked items.
func (f *MultiSelectField) SelectedIndices() []int {
	var indices []int
	for i := range f.options {
		if f.checked[i] {
			indices = append(indices, i)
		}
	}
	return indices
}

// IsFiltering returns whether the list is currently filtering.
func (f *MultiSelectField) IsFiltering() bool {
	return f.list.SettingFilter()
}
