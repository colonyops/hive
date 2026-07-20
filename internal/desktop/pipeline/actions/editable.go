package actions

import (
	"fmt"
	"time"
)

// EditableAction is the explicit, non-executable editor contract exposed to
// desktop clients. Exactly one config branch must be set and it must match
// Type; callers cannot send an untyped executable blob.
type EditableAction struct {
	ID           string                 `json:"id"`
	Label        string                 `json:"label"`
	Type         string                 `json:"type"`
	ShowInDetail bool                   `json:"showInDetail"`
	AppliesTo    []string               `json:"appliesTo"`
	Launch       *EditableLaunchConfig  `json:"launch,omitempty"`
	Shell        *EditableShellConfig   `json:"shell,omitempty"`
	Message      *EditableMessageConfig `json:"message,omitempty"`
}

// EditableCatalog returns the effective last-good catalog and any error from
// parsing the latest file. It lets the settings UI remain useful while making
// a hand-edited malformed actions.yml visible to the user.
type EditableCatalog struct {
	Actions []EditableAction `json:"actions"`
	Error   string           `json:"error"`
}

type EditableLaunchConfig struct {
	PromptTemplate string `json:"promptTemplate"`
	Agent          string `json:"agent,omitempty"`
	RepoTemplate   string `json:"repoTemplate,omitempty"`
}

type EditableShellConfig struct {
	CommandTemplate string            `json:"commandTemplate"`
	Cwd             string            `json:"cwd,omitempty"`
	Timeout         string            `json:"timeout,omitempty"`
	Env             map[string]string `json:"env,omitempty"`
}

type EditableMessageConfig struct {
	Topic string `json:"topic"`
}

func editableFromAction(a Action) EditableAction {
	out := EditableAction{ID: a.ID, Label: a.Label, Type: a.Type, ShowInDetail: a.ShowInDetail, AppliesTo: append([]string(nil), a.AppliesTo...)}
	switch c := a.Config.(type) {
	case *LaunchSessionConfig:
		out.Launch = &EditableLaunchConfig{PromptTemplate: c.PromptTemplate, Agent: c.Agent, RepoTemplate: c.RepoTemplate}
	case *ShellConfig:
		timeout := ""
		if c.Timeout != 0 {
			timeout = time.Duration(c.Timeout).String()
		}
		out.Shell = &EditableShellConfig{CommandTemplate: c.CommandTemplate, Cwd: c.Cwd, Timeout: timeout, Env: cloneEnv(c.Env)}
	case *PublishEventConfig:
		out.Message = &EditableMessageConfig{Topic: c.Topic}
	}
	return out
}

func actionFromEditable(e EditableAction) (Action, error) {
	branches := 0
	if e.Launch != nil {
		branches++
	}
	if e.Shell != nil {
		branches++
	}
	if e.Message != nil {
		branches++
	}
	if branches != 1 {
		return Action{}, fmt.Errorf("action %q: exactly one matching config branch is required", e.ID)
	}
	a := Action{ID: e.ID, Label: e.Label, Type: e.Type, ShowInDetail: e.ShowInDetail, AppliesTo: append([]string(nil), e.AppliesTo...)}
	switch e.Type {
	case "launch-session":
		if e.Launch == nil {
			return Action{}, fmt.Errorf("action %q: launch config is required for launch-session", e.ID)
		}
		a.Config = &LaunchSessionConfig{PromptTemplate: e.Launch.PromptTemplate, Agent: e.Launch.Agent, RepoTemplate: e.Launch.RepoTemplate}
	case "shell":
		if e.Shell == nil {
			return Action{}, fmt.Errorf("action %q: shell config is required for shell", e.ID)
		}
		var timeout Duration
		if e.Shell.Timeout != "" {
			d, err := time.ParseDuration(e.Shell.Timeout)
			if err != nil {
				return Action{}, fmt.Errorf("action %q: timeout: %w", e.ID, err)
			}
			timeout = Duration(d)
		}
		a.Config = &ShellConfig{CommandTemplate: e.Shell.CommandTemplate, Cwd: e.Shell.Cwd, Timeout: timeout, Env: cloneEnv(e.Shell.Env)}
	case "publish-event":
		if e.Message == nil {
			return Action{}, fmt.Errorf("action %q: message config is required for publish-event", e.ID)
		}
		a.Config = &PublishEventConfig{Topic: e.Message.Topic}
	default:
		return Action{}, fmt.Errorf("action %q: unknown type %q", e.ID, e.Type)
	}
	if err := validateActions([]Action{a}); err != nil {
		return Action{}, err
	}
	return a, nil
}

func cloneEnv(env map[string]string) map[string]string {
	if env == nil {
		return nil
	}
	out := make(map[string]string, len(env))
	for k, v := range env {
		out[k] = v
	}
	return out
}
