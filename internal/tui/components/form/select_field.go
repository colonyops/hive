package form

import (
	"io"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/colonyops/hive/internal/core/styles"
)

// SelectFormField is a single-select form field wrapping list.Model.
type SelectFormField struct {
	list    list.Model
	options []string
	label_  string
	focused bool
}

// selectDelegate renders items in a single-select list.
type selectDelegate struct{}

func (d selectDelegate) Height() int                             { return 1 }
func (d selectDelegate) Spacing() int                            { return 0 }
func (d selectDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d selectDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	item, ok := listItem.(selectItem)
	if !ok {
		return
	}

	isSelected := index == m.Index()

	style := styles.TextForegroundStyle
	cursor := "  "
	if isSelected {
		style = styles.SelectFieldItemSelectedStyle
		cursor = "> "
	}

	_, _ = io.WriteString(w, cursor)
	_, _ = io.WriteString(w, style.Render(item.label))
}

// NewSelectFormField creates a single-select field from static options.
// defaultVal pre-selects the matching option if found.
func NewSelectFormField(label string, options []string, defaultVal string) *SelectFormField {
	items := make([]list.Item, len(options))
	selected := -1
	for i, opt := range options {
		items[i] = selectItem{label: opt, index: i}
		if opt == defaultVal {
			selected = i
		}
	}

	const maxVisible = 8
	height := max(min(len(options), maxVisible), 1)

	delegate := selectDelegate{}
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

	if selected >= 0 && selected < len(options) {
		l.Select(selected)
	}

	return &SelectFormField{
		list:    l,
		options: options,
		label_:  label,
	}
}

func (f *SelectFormField) Update(msg tea.Msg) (Field, tea.Cmd) {
	if !f.focused {
		return f, nil
	}

	var cmd tea.Cmd
	f.list, cmd = f.list.Update(msg)
	return f, cmd
}

func (f *SelectFormField) View() string {
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

func (f *SelectFormField) Focus() tea.Cmd {
	f.focused = true
	return nil
}

func (f *SelectFormField) Blur() {
	f.focused = false
}

func (f *SelectFormField) Focused() bool { return f.focused }

func (f *SelectFormField) Value() any {
	item := f.list.SelectedItem()
	if item == nil {
		return ""
	}
	if si, ok := item.(selectItem); ok && si.index >= 0 && si.index < len(f.options) {
		return f.options[si.index]
	}
	return ""
}

func (f *SelectFormField) Label() string { return f.label_ }

// IsFiltering returns whether the list is currently filtering.
func (f *SelectFormField) IsFiltering() bool {
	return f.list.SettingFilter()
}
