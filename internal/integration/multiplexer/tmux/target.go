package tmux

import (
	"context"
	"fmt"
	"strings"

	"github.com/colonyops/hive/internal/core/multiplexer"
)

func renderSessionTarget(target multiplexer.Target) (string, error) {
	if err := target.ValidateSession(); err != nil {
		return "", err
	}
	return target.Session, nil
}

func renderWindowTarget(target multiplexer.Target) (string, error) {
	if err := target.ValidateWindow(); err != nil {
		return "", err
	}
	return target.Session + ":" + target.Window, nil
}

func renderPaneTarget(target multiplexer.Target) (string, error) {
	if err := target.ValidatePane(); err != nil {
		return "", err
	}
	return target.Session + ":" + target.Window + "." + target.Pane, nil
}

// ResolveTarget resolves a tmux target into a qualified pane.
func (c *Client) ResolveTarget(ctx context.Context, raw string) (multiplexer.Pane, error) {
	if raw == "" || strings.ContainsRune(raw, '\x00') {
		return multiplexer.Pane{}, fmt.Errorf("tmux target is invalid")
	}
	stdout, _, err := c.runner.Capture(ctx, "display-message", "-p", "-t", raw, paneFormat)
	if err != nil {
		return multiplexer.Pane{}, fmt.Errorf("resolve tmux target %q: %w", raw, err)
	}
	pane, err := parsePaneRow(strings.TrimSuffix(string(stdout), "\n"))
	if err != nil {
		return multiplexer.Pane{}, fmt.Errorf("resolve tmux target %q: %w", raw, err)
	}
	return pane, nil
}

// CurrentSession returns the current tmux session, or an empty target outside tmux.
func (c *Client) CurrentSession(ctx context.Context) (multiplexer.Target, error) {
	if !c.insideTmux() {
		return multiplexer.Target{}, nil
	}
	stdout, _, err := c.runner.Capture(ctx, "display-message", "-p", "#S")
	if err != nil {
		return multiplexer.Target{}, fmt.Errorf("get current tmux session: %w", err)
	}
	name := strings.TrimSpace(string(stdout))
	if name == "" {
		return multiplexer.Target{}, nil
	}
	return multiplexer.Target{Session: name}, nil
}
