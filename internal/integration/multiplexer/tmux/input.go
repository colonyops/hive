package tmux

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/colonyops/hive/internal/domain/multiplexer"
	"github.com/colonyops/hive/pkg/randid"
)

// SendLiteral sends text without interpreting it as key names.
func (c *Client) SendLiteral(ctx context.Context, target multiplexer.Target, text string) error {
	rendered, err := renderPaneTarget(target)
	if err != nil {
		return err
	}
	_, _, err = c.runner.Capture(ctx, "send-keys", "-t", rendered, "-l", "--", text)
	if err != nil {
		return fmt.Errorf("tmux send literal: %w", err)
	}
	return nil
}

// SendKey sends one validated tmux named-key token.
func (c *Client) SendKey(ctx context.Context, target multiplexer.Target, key multiplexer.NamedKey) error {
	validated, err := multiplexer.NewNamedKey(string(key))
	if err != nil {
		return err
	}
	rendered, err := renderPaneTarget(target)
	if err != nil {
		return err
	}
	_, _, err = c.runner.Capture(ctx, "send-keys", "-t", rendered, "--", string(validated))
	if err != nil {
		return fmt.Errorf("tmux send key %q: %w", key, err)
	}
	return nil
}

// Paste loads bytes into a private tmux buffer and pastes it into a pane.
func (c *Client) Paste(ctx context.Context, target multiplexer.Target, text []byte, opts multiplexer.PasteOptions) (retErr error) {
	rendered, err := renderPaneTarget(target)
	if err != nil {
		return err
	}
	bufferName := "hive-" + randid.Generate(12)
	if _, _, err = c.runner.Input(ctx, bytes.NewReader(text), "load-buffer", "-b", bufferName, "-"); err != nil {
		return fmt.Errorf("tmux load buffer: %w", err)
	}
	bufferDeleted := false
	defer func() {
		if bufferDeleted {
			return
		}
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()
		if _, _, cleanupErr := c.runner.Capture(cleanupCtx, "delete-buffer", "-b", bufferName); cleanupErr != nil {
			c.log.Warn().Err(cleanupErr).Str("buffer", bufferName).Msg("failed to delete tmux paste buffer")
		}
	}()

	args := []string{"paste-buffer", "-d", "-r", "-S", "-b", bufferName, "-t", rendered}
	if opts.Bracketed {
		args = append(args, "-p")
	}
	_, stderr, err := c.runner.Capture(ctx, args...)
	if err != nil && bytes.Contains(stderr, []byte("unknown flag -S")) {
		// tmux added -S in the same change that introduced paste sanitization.
		// Older releases reject the flag but already paste bytes without filtering.
		args = slices.DeleteFunc(args, func(arg string) bool { return arg == "-S" })
		_, _, err = c.runner.Capture(ctx, args...)
	}
	if err != nil {
		return fmt.Errorf("tmux paste buffer: %w", err)
	}
	bufferDeleted = true
	return nil
}
