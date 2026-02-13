package tui

import (
	"fmt"

	"github.com/hay-kot/hive/internal/core/config"
	"github.com/hay-kot/hive/internal/core/session"
	"github.com/hay-kot/hive/internal/tui/components/form"
)

// newFormDialog creates a form.Dialog from config fields, injecting runtime data.
func newFormDialog(
	title string,
	fields []config.FormField,
	sessions []session.Session,
	repos []DiscoveredRepo,
) (*form.Dialog, error) {
	components := make([]form.Field, 0, len(fields))
	variables := make([]string, 0, len(fields))

	for _, f := range fields {
		var comp form.Field
		switch {
		case f.Preset == config.FormPresetSessionSelector:
			comp = form.NewSessionSelectorField(f.Label, sessions, f.Multi)
		case f.Preset == config.FormPresetProjectSelector:
			formRepos := make([]form.Repo, len(repos))
			for i, r := range repos {
				formRepos[i] = form.Repo{Name: r.Name, Path: r.Path, Remote: r.Remote}
			}
			comp = form.NewProjectSelectorField(f.Label, formRepos, f.Multi)
		case f.Type == config.FormTypeText:
			comp = form.NewTextField(f.Label, f.Placeholder, f.Default)
		case f.Type == config.FormTypeTextArea:
			comp = form.NewTextAreaField(f.Label, f.Placeholder, f.Default)
		case f.Type == config.FormTypeSelect:
			comp = form.NewSelectFormField(f.Label, f.Options, f.Default)
		case f.Type == config.FormTypeMultiSelect:
			comp = form.NewMultiSelectFormField(f.Label, f.Options)
		default:
			return nil, fmt.Errorf("unknown form field type/preset: %s/%s", f.Type, f.Preset)
		}
		components = append(components, comp)
		variables = append(variables, f.Variable)
	}

	return form.NewDialog(title, components, variables), nil
}
