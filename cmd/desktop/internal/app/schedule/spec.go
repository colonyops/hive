package schedule

import (
	"fmt"
	"regexp"
	"strings"
)

// OnMissed is what a schedule does with an occurrence that came due while the
// app was not running.
//
// ENUM(run, skip)
type OnMissed string

// Reason is why a run happened.
//
// ENUM(due, catch_up, manual)
type Reason string

// Status is a run's outcome.
//
// ENUM(launched, failed, skipped)
type Status string

// idPattern is the shape of a schedule id: lowercase, digits and hyphens. It
// is narrow because the id is a stable key in both the manifest and the
// cursor table, and a user retypes it when they edit the file by hand.
var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// Spec is one schedules: entry in an agent-workspace.yaml. Workspace is not in
// the file: the loader stamps it on, because a Spec travels to the scheduler
// alone.
type Spec struct {
	Workspace string   `json:"workspace" yaml:"-"`
	ID        string   `json:"id"        yaml:"id"`
	Name      string   `json:"name"      yaml:"name,omitempty"`
	Cron      string   `json:"cron"      yaml:"cron"`
	Prompt    string   `json:"prompt"    yaml:"prompt"`
	Disabled  bool     `json:"disabled"  yaml:"disabled,omitempty"`
	OnMissed  OnMissed `json:"onMissed"  yaml:"on_missed,omitempty"`
}

func (s Spec) DisplayName() string {
	if s.Name != "" {
		return s.Name
	}
	return s.ID
}

// Validate leaves Workspace alone: it is the loader's to set, so validating an
// unsaved edit must not depend on it.
func (s Spec) Validate() error {
	if s.ID == "" {
		return fmt.Errorf("schedule: id is required")
	}
	if !idPattern.MatchString(s.ID) {
		return fmt.Errorf("schedule %q: id must be lowercase letters, digits and hyphens", s.ID)
	}
	if _, err := ParseCron(s.Cron); err != nil {
		return fmt.Errorf("schedule %q: %w", s.ID, err)
	}
	if strings.TrimSpace(s.Prompt) == "" {
		return fmt.Errorf("schedule %q: prompt is required", s.ID)
	}
	if err := ValidatePrompt(s.Prompt); err != nil {
		return fmt.Errorf("schedule %q: %w", s.ID, err)
	}
	if s.OnMissed != "" && !s.OnMissed.IsValid() {
		return fmt.Errorf("schedule %q: on_missed %q is not valid (expected %s)", s.ID, s.OnMissed, strings.Join(OnMissedNames(), ", "))
	}
	return nil
}
