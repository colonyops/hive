package command

import "context"

// TmuxExecutor opens or creates a tmux session via the TmuxOpener interface.
type TmuxExecutor struct {
	opener       TmuxOpener
	name         string
	path         string
	remote       string
	targetWindow string
	background   bool

	completed bool
}

var (
	_ Executor        = (*TmuxExecutor)(nil)
	_ ResultMessenger = (*TmuxExecutor)(nil)
)

func (e *TmuxExecutor) Execute(ctx context.Context) (output <-chan string, done <-chan error, cancel context.CancelFunc) {
	ctx, cancel = context.WithCancel(ctx)
	doneCh := make(chan error, 1)

	go func() {
		defer close(doneCh)
		result, err := e.opener.OpenTmuxSession(ctx, e.name, e.path, e.remote, e.targetWindow, e.background)
		e.completed = err == nil && result.Completed
		doneCh <- err
	}()

	return nil, doneCh, cancel
}

// ResultMessage reports a launch whose commands all finished, because the user
// gets no terminal to attach to and would otherwise see nothing happen.
func (e *TmuxExecutor) ResultMessage() string {
	if e.completed {
		return "Session completed successfully."
	}
	return ""
}
