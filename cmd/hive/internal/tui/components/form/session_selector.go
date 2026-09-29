package form

import (
	tea "charm.land/bubbletea/v2"
	"github.com/colonyops/hive/internal/core/git"
	"github.com/colonyops/hive/internal/core/session"
)

// SessionSelectorField is a preset form field for selecting sessions.
// It composes SelectFormField (single) or MultiSelectField (multi) based on
// the multi flag, delegating all Field methods except Value().
type SessionSelectorField struct {
	inner    Field
	sessions []session.Session
	labels   []string // display labels (may include repo prefix)
	label_   string
	multi    bool
}

// NewSessionSelectorField creates a session selector preset field.
// When multi is true, it uses MultiSelectField; otherwise SelectFormField.
// Labels are formatted as "repo/session-name" when sessions span multiple remotes.
func NewSessionSelectorField(label string, sessions []session.Session, multi bool) *SessionSelectorField {
	labels := sessionLabels(sessions)

	var inner Field
	if multi {
		inner = NewMultiSelectFormField(label, labels)
	} else {
		inner = NewSelectFormField(label, labels, "")
	}

	return &SessionSelectorField{
		inner:    inner,
		sessions: sessions,
		labels:   labels,
		label_:   label,
		multi:    multi,
	}
}

// sessionLabels builds display labels for sessions. If all sessions share the
// same remote, labels are just session names. Otherwise they are prefixed with
// the repo name extracted from the remote (e.g. "hive/my-session").
func sessionLabels(sessions []session.Session) []string {
	labels := make([]string, len(sessions))
	if len(sessions) == 0 {
		return labels
	}

	multiRemote := false
	first := sessions[0].Remote
	for _, s := range sessions[1:] {
		if s.Remote != first {
			multiRemote = true
			break
		}
	}

	for i, s := range sessions {
		if multiRemote {
			repo := git.ExtractRepoName(s.Remote)
			labels[i] = repo + "/" + s.Name
		} else {
			labels[i] = s.Name
		}
	}
	return labels
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
	labelToIdx := make(map[string]int, len(f.labels))
	for i, l := range f.labels {
		labelToIdx[l] = i
	}

	if f.multi {
		selectedLabels, _ := f.inner.Value().([]string)
		result := make([]session.Session, 0, len(selectedLabels))
		for _, label := range selectedLabels {
			if idx, ok := labelToIdx[label]; ok {
				result = append(result, f.sessions[idx])
			}
		}
		return result
	}

	selectedLabel, _ := f.inner.Value().(string)
	if idx, ok := labelToIdx[selectedLabel]; ok {
		return f.sessions[idx]
	}
	return session.Session{}
}
