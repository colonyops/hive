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
}

var _ Executor = (*TmuxExecutor)(nil)

func (e *TmuxExecutor) Execute(ctx context.Context) (output <-chan string, done <-chan error, cancel context.CancelFunc) {
	ctx, cancel = context.WithCancel(ctx)
	doneCh := make(chan error, 1)

	go func() {
		defer close(doneCh)
		doneCh <- e.opener.OpenTmuxSession(ctx, e.name, e.path, e.remote, e.targetWindow, e.background)
	}()

	return nil, doneCh, cancel
}
