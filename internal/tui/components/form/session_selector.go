package form

import (
	tea "charm.land/bubbletea/v2"
	"github.com/hay-kot/hive/internal/core/session"
)

// SessionSelectorField is a preset form field for selecting sessions.
// It composes SelectFormField (single) or MultiSelectField (multi) based on
// the multi flag, delegating all Field methods except Value().
type SessionSelectorField struct {
	inner    Field
	sessions []session.Session
	label_   string
	multi    bool
}

// NewSessionSelectorField creates a session selector preset field.
// When multi is true, it uses MultiSelectField; otherwise SelectFormField.
func NewSessionSelectorField(label string, sessions []session.Session, multi bool) *SessionSelectorField {
	labels := make([]string, len(sessions))
	for i, s := range sessions {
		labels[i] = s.Name
	}

	var inner Field
	if multi {
		inner = NewMultiSelectFormField(label, labels)
	} else {
		inner = NewSelectFormField(label, labels, "")
	}

	return &SessionSelectorField{
		inner:    inner,
		sessions: sessions,
		label_:   label,
		multi:    multi,
	}
}

func (f *SessionSelectorField) Update(msg tea.Msg) (Field, tea.Cmd) {
	var cmd tea.Cmd
	f.inner, cmd = f.inner.Update(msg)
	return f, cmd
}

func (f *SessionSelectorField) View() string   { return f.inner.View() }
func (f *SessionSelectorField) Focus() tea.Cmd { return f.inner.Focus() }
func (f *SessionSelectorField) Blur()          { f.inner.Blur() }
func (f *SessionSelectorField) Focused() bool  { return f.inner.Focused() }
func (f *SessionSelectorField) Label() string  { return f.label_ }

// Value returns session.Session for single-select or []session.Session for multi-select.
func (f *SessionSelectorField) Value() any {
	if f.multi {
		selectedLabels, _ := f.inner.Value().([]string)
		labelToIdx := make(map[string]int, len(f.sessions))
		for i, s := range f.sessions {
			labelToIdx[s.Name] = i
		}
		result := make([]session.Session, 0, len(selectedLabels))
		for _, label := range selectedLabels {
			if idx, ok := labelToIdx[label]; ok {
				result = append(result, f.sessions[idx])
			}
		}
		return result
	}

	selectedLabel, _ := f.inner.Value().(string)
	for _, s := range f.sessions {
		if s.Name == selectedLabel {
			return s
		}
	}
	return session.Session{}
}
