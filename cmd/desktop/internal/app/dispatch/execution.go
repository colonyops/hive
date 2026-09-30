package dispatch

// ActionInvocationInput contains only user-supplied action inputs. It never
// carries executable configuration or message attribution.
type ActionInvocationInput struct {
	Session *SessionInvocationInput `json:"session,omitempty"`
	// Inputs are the values collected for the action's declared inputs, keyed
	// by input name. They are validated against the catalog's declaration on
	// every invocation, so a name the action does not declare is refused.
	Inputs map[string]string `json:"inputs,omitempty"`
	Rerun  bool              `json:"rerun,omitempty"`
}

// SessionLaunchRepository is the safe presentation of a repository available
// to an interactive action. Local checkout paths stay backend-only.
type SessionLaunchRepository struct {
	Name       string `json:"name"`
	Repository string `json:"repository"`
}

// SessionLaunchWorkspace identifies a configured workspace by its stable
// directory name.
type SessionLaunchWorkspace struct {
	Dir            string `json:"dir"`
	Name           string `json:"name"`
	SupportsPrompt bool   `json:"supportsPrompt"`
}

// SessionLaunchOptions is the narrow DTO used by the session launch dialog.
type SessionLaunchOptions struct {
	Repositories      []SessionLaunchRepository `json:"repositories"`
	DefaultRepository string                    `json:"defaultRepository"`
	Workspaces        []SessionLaunchWorkspace  `json:"workspaces"`
	Agents            []string                  `json:"agents"`
	DefaultAgent      string                    `json:"defaultAgent"`
}

type SessionInvocationInput struct {
	Name       string `json:"name"`
	Repository string `json:"repository,omitempty"`
	Workspace  string `json:"workspace,omitempty"`
	Agent      string `json:"agent,omitempty"`
}

type ExecutionLog struct {
	Stdout string `json:"stdout,omitempty"`
	Stderr string `json:"stderr,omitempty"`
}

type SessionExecutionOutcome struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Slug names the tmux session; Path is the checkout a post hook runs in.
	Slug string `json:"slug,omitempty"`
	Path string `json:"path,omitempty"`
}

type MessageExecutionOutcome struct {
	Topic  string `json:"topic"`
	Sender string `json:"sender"`
}

// ClipboardExecutionOutcome carries the text a clipboard action rendered. The
// clipboard write itself is the desktop adapter's: the core produces the text
// and the frontend copies it (see the detail-pane render path), so this is the
// executor's whole result.
type ClipboardExecutionOutcome struct {
	Text string `json:"text"`
}

// ExecutionOutcome is a tagged-by-presence union. Exactly one branch is set
// for successful side-effecting executors.
type ExecutionOutcome struct {
	Session   *SessionExecutionOutcome   `json:"session,omitempty"`
	Message   *MessageExecutionOutcome   `json:"message,omitempty"`
	Clipboard *ClipboardExecutionOutcome `json:"clipboard,omitempty"`
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
	CommandID            int64             `json:"commandId"`
	Status               string            `json:"status"`
	Result               *ExecutionOutcome `json:"result,omitempty"`
	Error                string            `json:"error,omitempty"`
	Stdout               string            `json:"stdout,omitempty"`
	Stderr               string            `json:"stderr,omitempty"`
	ConfirmationRequired bool              `json:"confirmationRequired,omitempty"`
}
