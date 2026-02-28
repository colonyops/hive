package command

import "context"

// channelWriter adapts a string channel to an io.Writer.
type channelWriter struct {
	ch  chan<- string
	ctx context.Context
}

func (w *channelWriter) Write(p []byte) (int, error) {
	select {
	case w.ch <- string(p):
		return len(p), nil
	case <-w.ctx.Done():
		return 0, w.ctx.Err()
	}
}
