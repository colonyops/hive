package actions

import (
	"fmt"
	"maps"
	"time"
)

// EditableAction is the explicit, non-executable editor contract exposed to
// desktop clients. Exactly one config branch must be set and it must match
// Type; callers cannot send an untyped executable blob.
type EditableAction struct {
	ID           string `json:"id"`
	Label        string `json:"label"`
	Type         string `json:"type"`
	ShowInDetail bool   `json:"showInDetail"`
	// Targets is always explicit on the wire — an action that declares none
	// is an item action, and the editor renders a checked box rather than an
	// empty one it would have to explain.
	Targets   []string                 `json:"targets"`
	AppliesTo []string                 `json:"appliesTo"`
	Inputs    []InputSpec              `json:"inputs,omitempty"`
	Launch    *EditableLaunchConfig    `json:"launch,omitempty"`
	Shell     *EditableShellConfig     `json:"shell,omitempty"`
	Message   *EditableMessageConfig   `json:"message,omitempty"`
	Clipboard *EditableClipboardConfig `json:"clipboard,omitempty"`
}

// EditableCatalog returns the effective last-good catalog and any error from
// parsing the latest file. It lets the settings UI remain useful while making
// a hand-edited malformed actions.yml visible to the user.
//
// Launchers ride along rather than getting a read of their own: they are the
// other list in the same file, so one parse answers for both and a single
// Error describes whatever is wrong with it.
type EditableCatalog struct {
	Actions   []EditableAction `json:"actions"`
	Launchers []Launcher       `json:"launchers"`
	Error     string           `json:"error"`
}

type EditableLaunchConfig struct {
	PromptTemplate  string `json:"promptTemplate"`
	Agent           string `json:"agent,omitempty"`
	RepoTemplate    string `json:"repoTemplate,omitempty"`
	Workspace       string `json:"workspace,omitempty"`
	PostHook        string `json:"postHook,omitempty"`
	PostHookTimeout string `json:"postHookTimeout,omitempty"`
}

type EditableShellConfig struct {
	CommandTemplate string            `json:"commandTemplate"`
	Cwd             string            `json:"cwd,omitempty"`
	Timeout         string            `json:"timeout,omitempty"`
	Env             map[string]string `json:"env,omitempty"`
}

type EditableMessageConfig struct {
	MessageTemplate string `json:"messageTemplate"`
	Topic           string `json:"topic"`
}

type EditableClipboardConfig struct {
	TextTemplate string `json:"textTemplate"`
}

func editableFromAction(a Action) (EditableAction, error) {
	out := EditableAction{ID: a.ID, Label: a.Label, Type: a.Type, ShowInDetail: a.ShowInDetail, Targets: effectiveTargets(a), AppliesTo: append([]string(nil), a.AppliesTo...), Inputs: cloneInputs(a.Inputs)}
	switch c := a.Config.(type) {
	case *LaunchSessionConfig:
		postHookTimeout := ""
		if c.PostHookTimeout != 0 {
			postHookTimeout = time.Duration(c.PostHookTimeout).String()
		}
		out.Launch = &EditableLaunchConfig{PromptTemplate: c.PromptTemplate, Agent: c.Agent, RepoTemplate: c.RepoTemplate, Workspace: c.Workspace, PostHook: c.PostHook, PostHookTimeout: postHookTimeout}
	case *ShellConfig:
		timeout := ""
		if c.Timeout != 0 {
			timeout = time.Duration(c.Timeout).String()
		}
		out.Shell = &EditableShellConfig{CommandTemplate: c.CommandTemplate, Cwd: c.Cwd, Timeout: timeout, Env: cloneEnv(c.Env)}
	case *PublishMessageConfig:
		out.Message = &EditableMessageConfig{MessageTemplate: c.MessageTemplate, Topic: c.Topic}
	case *ClipboardConfig:
		out.Clipboard = &EditableClipboardConfig{TextTemplate: c.TextTemplate}
	default:
		// A registered type with no case here would otherwise return an
		// EditableAction with every config branch left nil — the frontend
		// would render an action it cannot show any fields for. Fail rather
		// than serve that silently-empty record.
		return EditableAction{}, fmt.Errorf("action %q: no editable-catalog branch for config type %T (type %q); registry and editable.go are out of sync", a.ID, a.Config, a.Type)
	}
	return out, nil
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
	if e.Clipboard != nil {
		branches++
	}
	if branches != 1 {
		return Action{}, fmt.Errorf("action %q: exactly one matching config branch is required", e.ID)
	}
	a := Action{ID: e.ID, Label: e.Label, Type: e.Type, ShowInDetail: e.ShowInDetail, Targets: normalizeTargets(e.Targets), AppliesTo: append([]string(nil), e.AppliesTo...), Inputs: normalizeInputs(cloneInputs(e.Inputs))}
	switch e.Type {
	case "launch-session":
		if e.Launch == nil {
			return Action{}, fmt.Errorf("action %q: launch config is required for launch-session", e.ID)
		}
		var postHookTimeout Duration
		if e.Launch.PostHookTimeout != "" {
			d, err := time.ParseDuration(e.Launch.PostHookTimeout)
			if err != nil {
				return Action{}, fmt.Errorf("action %q: post_hook_timeout: %w", e.ID, err)
			}
			postHookTimeout = Duration(d)
		}
		a.Config = &LaunchSessionConfig{PromptTemplate: e.Launch.PromptTemplate, Agent: e.Launch.Agent, RepoTemplate: e.Launch.RepoTemplate, Workspace: e.Launch.Workspace, PostHook: e.Launch.PostHook, PostHookTimeout: postHookTimeout}
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
	case "publish-message":
		if e.Message == nil {
			return Action{}, fmt.Errorf("action %q: message config is required for publish-message", e.ID)
		}
		a.Config = &PublishMessageConfig{MessageTemplate: e.Message.MessageTemplate, Topic: e.Message.Topic}
	case "clipboard":
		if e.Clipboard == nil {
			return Action{}, fmt.Errorf("action %q: clipboard config is required for clipboard", e.ID)
		}
		a.Config = &ClipboardConfig{TextTemplate: e.Clipboard.TextTemplate}
	default:
		return Action{}, fmt.Errorf("action %q: unknown type %q", e.ID, e.Type)
	}
	if err := validateActions([]Action{a}); err != nil {
		return Action{}, err
	}
	return a, nil
}

// effectiveTargets spells out what an empty declaration means, so the editor
// and any other reader of the catalog never has to know the default.
func effectiveTargets(a Action) []string {
	if len(a.Targets) == 0 {
		return []string{TargetItem}
	}
	return append([]string(nil), a.Targets...)
}

func cloneEnv(env map[string]string) map[string]string {
	if env == nil {
		return nil
	}
	out := make(map[string]string, len(env))
	maps.Copy(out, env)
	return out
}
