package tui

import (
	"fmt"

	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/internal/core/session"
	"github.com/colonyops/hive/internal/core/terminal"
	"github.com/colonyops/hive/internal/tui/components/form"
	"github.com/colonyops/hive/pkg/kv"
)

// newFormDialog creates a form.Dialog from config fields, injecting runtime data.
// termStatuses may be nil if terminal integration is not configured.
func newFormDialog(
	title string,
	fields []config.FormField,
	sessions []session.Session,
	repos []DiscoveredRepo,
	termStatuses *kv.Store[string, TerminalStatus],
) (*form.Dialog, error) {
	components := make([]form.Field, 0, len(fields))
	variables := make([]string, 0, len(fields))

	for _, f := range fields {
		var comp form.Field
		switch {
		case f.Preset == config.FormPresetSessionSelector:
			filtered := sessions
			if f.Filter != config.FormFilterAll {
				filtered = filterActiveSessions(sessions, termStatuses)
			}
			comp = form.NewSessionSelectorField(f.Label, filtered, f.Multi)
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

// filterActiveSessions returns sessions that are active and have a non-missing
// terminal status. When termStatuses is nil (no terminal integration), falls
// back to filtering by session state only.
func filterActiveSessions(sessions []session.Session, termStatuses *kv.Store[string, TerminalStatus]) []session.Session {
	filtered := make([]session.Session, 0, len(sessions))
	for _, s := range sessions {
		if s.State != session.StateActive {
			continue
		}
		if termStatuses != nil {
			ts, ok := termStatuses.Get(s.ID)
			if !ok || ts.Status == terminal.StatusMissing {
				continue
			}
		}
		filtered = append(filtered, s)
	}
	return filtered
}
