package wailsui

import (
	"github.com/colonyops/hive/cmd/desktop/internal/app/dispatch"
)

// ActionInvocationInput carries only user-supplied action inputs. It never
// carries executable configuration or message attribution.
type ActionInvocationInput struct {
	Session *SessionInvocationInput `json:"session,omitempty"`
	// Inputs are the values collected for the action's declared inputs, keyed
	// by input name. They are validated against the catalog's declaration on
	// every invocation, so a name the action does not declare is refused.
	Inputs map[string]string `json:"inputs,omitempty"`
	Rerun  bool              `json:"rerun,omitempty"`
}

// SessionInvocationInput is the session an interactive launch-session action
// asks for.
type SessionInvocationInput struct {
	Name       string `json:"name"`
	Repository string `json:"repository,omitempty"`
	Workspace  string `json:"workspace,omitempty"`
	Agent      string `json:"agent,omitempty"`
}

func (in ActionInvocationInput) core() dispatch.ActionInvocationInput {
	out := dispatch.ActionInvocationInput{Inputs: in.Inputs, Rerun: in.Rerun}
	if s := in.Session; s != nil {
		out.Session = &dispatch.SessionInvocationInput{Name: s.Name, Repository: s.Repository, Workspace: s.Workspace, Agent: s.Agent}
	}
	return out
}

// ActionRunView is one action run's state and, once it finished, its outcome.
type ActionRunView struct {
	CommandID            int64             `json:"commandId"`
	Status               string            `json:"status"`
	Result               *ExecutionOutcome `json:"result,omitempty"`
	Error                string            `json:"error,omitempty"`
	Stdout               string            `json:"stdout,omitempty"`
	Stderr               string            `json:"stderr,omitempty"`
	ConfirmationRequired bool              `json:"confirmationRequired,omitempty"`
}

// ExecutionOutcome is a tagged-by-presence union. Exactly one branch is set
// for successful side-effecting executors.
type ExecutionOutcome struct {
	Session   *SessionExecutionOutcome   `json:"session,omitempty"`
	Message   *MessageExecutionOutcome   `json:"message,omitempty"`
	Clipboard *ClipboardExecutionOutcome `json:"clipboard,omitempty"`
}

// SessionExecutionOutcome is the session a launch-session action started.
type SessionExecutionOutcome struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Slug names the tmux session; Path is the checkout a post hook runs in.
	Slug string `json:"slug,omitempty"`
	Path string `json:"path,omitempty"`
}

// MessageExecutionOutcome is where a publish-message action published.
type MessageExecutionOutcome struct {
	Topic  string `json:"topic"`
	Sender string `json:"sender"`
}

// ClipboardExecutionOutcome carries the text a clipboard action rendered. The
// frontend copies it; the core never touches the clipboard.
type ClipboardExecutionOutcome struct {
	Text string `json:"text"`
}

func actionRunViewOf(v dispatch.ActionRunView) ActionRunView {
	return ActionRunView{
		CommandID:            v.CommandID,
		Status:               v.Status,
		Result:               executionOutcomeOf(v.Result),
		Error:                v.Error,
		Stdout:               v.Stdout,
		Stderr:               v.Stderr,
		ConfirmationRequired: v.ConfirmationRequired,
	}
}

func executionOutcomeOf(o *dispatch.ExecutionOutcome) *ExecutionOutcome {
	if o == nil {
		return nil
	}
	out := &ExecutionOutcome{}
	if s := o.Session; s != nil {
		out.Session = &SessionExecutionOutcome{ID: s.ID, Name: s.Name, Slug: s.Slug, Path: s.Path}
	}
	if m := o.Message; m != nil {
		out.Message = &MessageExecutionOutcome{Topic: m.Topic, Sender: m.Sender}
	}
	if c := o.Clipboard; c != nil {
		out.Clipboard = &ClipboardExecutionOutcome{Text: c.Text}
	}
	return out
}
