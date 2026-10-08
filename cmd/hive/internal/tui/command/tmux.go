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
	outputCh := make(chan string)

	go func() {
		defer close(doneCh)
		defer close(outputCh)
		result, err := e.opener.OpenTmuxSession(ctx, e.name, e.path, e.remote, e.targetWindow, e.background)
		if err == nil && result.Completed {
			select {
			case outputCh <- "Session completed successfully.":
			case <-ctx.Done():
				err = ctx.Err()
			}
		}
		doneCh <- err
	}()

	return outputCh, doneCh, cancel
}
