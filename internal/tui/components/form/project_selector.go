package form

import tea "charm.land/bubbletea/v2"

// Repo represents a project/repository for the ProjectSelector preset.
// This is a form-package type to avoid import cycles with the tui package.
type Repo struct {
	Name   string
	Path   string
	Remote string
}

// ProjectSelectorField is a preset form field for selecting projects/repositories.
// It composes SelectFormField (single) or MultiSelectField (multi) based on
// the multi flag, delegating all Field methods except Value().
type ProjectSelectorField struct {
	inner  Field
	repos  []Repo
	label_ string
	multi  bool
}

// NewProjectSelectorField creates a project selector preset field.
// When multi is true, it uses MultiSelectField; otherwise SelectFormField.
func NewProjectSelectorField(label string, repos []Repo, multi bool) *ProjectSelectorField {
	labels := make([]string, len(repos))
	for i, r := range repos {
		labels[i] = r.Name
	}

	var inner Field
	if multi {
		inner = NewMultiSelectFormField(label, labels)
	} else {
		inner = NewSelectFormField(label, labels, "")
	}

	return &ProjectSelectorField{
		inner:  inner,
		repos:  repos,
		label_: label,
		multi:  multi,
	}
}

func (f *ProjectSelectorField) Update(msg tea.Msg) (Field, tea.Cmd) {
	var cmd tea.Cmd
	f.inner, cmd = f.inner.Update(msg)
	return f, cmd
}

func (f *ProjectSelectorField) View() string     { return f.inner.View() }
func (f *ProjectSelectorField) Focus() tea.Cmd   { return f.inner.Focus() }
func (f *ProjectSelectorField) Blur()            { f.inner.Blur() }
func (f *ProjectSelectorField) Focused() bool    { return f.inner.Focused() }
func (f *ProjectSelectorField) Label() string    { return f.label_ }
func (f *ProjectSelectorField) Validate() string { return f.inner.Validate() }

// Value returns Repo for single-select or []Repo for multi-select.
func (f *ProjectSelectorField) Value() any {
	if f.multi {
		selectedLabels, _ := f.inner.Value().([]string)
		labelToIdx := make(map[string]int, len(f.repos))
		for i, r := range f.repos {
			labelToIdx[r.Name] = i
		}
		result := make([]Repo, 0, len(selectedLabels))
		for _, label := range selectedLabels {
			if idx, ok := labelToIdx[label]; ok {
				result = append(result, f.repos[idx])
			}
		}
		return result
	}

	selectedLabel, _ := f.inner.Value().(string)
	for _, r := range f.repos {
		if r.Name == selectedLabel {
			return r
		}
	}
	return Repo{}
}
