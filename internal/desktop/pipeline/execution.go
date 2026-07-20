package pipeline

import "context"

// ActionInvocationInput contains only user-supplied action inputs. It never
// carries executable configuration or message attribution.
type ActionInvocationInput struct {
	Session *SessionInvocationInput `json:"session,omitempty"`
}

// SessionLaunchRepository is the safe presentation of a repository available
// to an interactive action. Local checkout paths stay backend-only.
type SessionLaunchRepository struct {
	Name       string `json:"name"`
	Repository string `json:"repository"`
}

// SessionLaunchOptions is the narrow DTO used by the session launch dialog.
type SessionLaunchOptions struct {
	Repositories      []SessionLaunchRepository `json:"repositories"`
	DefaultRepository string                    `json:"defaultRepository"`
	Agents            []string                  `json:"agents"`
	DefaultAgent      string                    `json:"defaultAgent"`
}

// SessionLaunchOptionsProvider supplies configured choices for interactive
// launch-session actions.
type SessionLaunchOptionsProvider interface {
	SessionLaunchOptions(context.Context) (SessionLaunchOptions, error)
}

type SessionInvocationInput struct {
	Name       string `json:"name"`
	Repository string `json:"repository"`
	Agent      string `json:"agent,omitempty"`
}

type ExecutionLog struct {
	Stdout string `json:"stdout,omitempty"`
	Stderr string `json:"stderr,omitempty"`
}

type SessionExecutionOutcome struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type MessageExecutionOutcome struct {
	Topic  string `json:"topic"`
	Sender string `json:"sender"`
}

// ExecutionOutcome is a tagged-by-presence union. Exactly one branch is set
// for successful side-effecting executors.
type ExecutionOutcome struct {
	Session *SessionExecutionOutcome `json:"session,omitempty"`
	Message *MessageExecutionOutcome `json:"message,omitempty"`
}

type ExecutionResult struct {
	// Attempted is internal execution state, not frontend data. It separates a
	// side effect that returned an error after dispatch from validation/config
	// failures before dispatch.
	Attempted bool              `json:"-"`
	Outcome   *ExecutionOutcome `json:"outcome,omitempty"`
	Log       ExecutionLog      `json:"log"`
}

type ActionRunView struct {
	CommandID int64             `json:"commandId"`
	Status    string            `json:"status"`
	Result    *ExecutionOutcome `json:"result,omitempty"`
	Error     string            `json:"error,omitempty"`
	Stdout    string            `json:"stdout,omitempty"`
	Stderr    string            `json:"stderr,omitempty"`
}
