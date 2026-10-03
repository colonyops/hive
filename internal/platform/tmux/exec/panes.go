package tmuxexec

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/colonyops/hive/internal/domain/multiplexer"
)

const paneDelimiter = "|||"

// q escapes delimiter characters in free-form fields. Numeric and tmux-native
// identity fields cannot contain the delimiter.
const paneFormat = "#{q:session_name}" + paneDelimiter + "#{window_index}" + paneDelimiter + "#{pane_index}" + paneDelimiter +
	"#{q:window_name}" + paneDelimiter + "#{q:pane_current_path}" + paneDelimiter + "#{window_activity}" + paneDelimiter +
	"#{pane_id}" + paneDelimiter + "#{pane_pid}" + paneDelimiter + "#{q:pane_title}" + paneDelimiter +
	"#{q:@hive-session}" + paneDelimiter + "#{pane_in_mode}"

// ListPanes returns every pane visible to the current tmux server.
func (c *Client) ListPanes(ctx context.Context) ([]multiplexer.Pane, error) {
	stdout, _, err := c.runner.Capture(ctx, "list-panes", "-a", "-F", paneFormat)
	if err != nil {
		return nil, fmt.Errorf("tmux list-panes: %w", err)
	}
	trimmed := strings.TrimSuffix(string(stdout), "\n")
	if trimmed == "" {
		return nil, nil
	}
	lines := strings.Split(trimmed, "\n")
	panes := make([]multiplexer.Pane, 0, len(lines))
	for i, line := range lines {
		pane, parseErr := parsePaneRow(line)
		if parseErr != nil {
			return nil, fmt.Errorf("parse tmux pane row %d: %w", i+1, parseErr)
		}
		panes = append(panes, pane)
	}
	return panes, nil
}

// InspectPane returns one resolved pane.
func (c *Client) InspectPane(ctx context.Context, target multiplexer.Target) (multiplexer.Pane, error) {
	rendered, err := renderPaneTarget(target)
	if err != nil {
		return multiplexer.Pane{}, err
	}
	return c.ResolveTarget(ctx, rendered)
}

// CapturePane captures pane text.
func (c *Client) CapturePane(ctx context.Context, target multiplexer.Target, opts multiplexer.CaptureOptions) (string, error) {
	rendered, err := renderPaneTarget(target)
	if err != nil {
		return "", err
	}
	args := []string{"capture-pane", "-p", "-t", rendered}
	if opts.JoinWrappedLines {
		args = append(args, "-J")
	}
	if opts.StartLine != nil {
		args = append(args, "-S", strconv.Itoa(*opts.StartLine))
	}
	if opts.EndLine != nil {
		args = append(args, "-E", strconv.Itoa(*opts.EndLine))
	}
	stdout, _, err := c.runner.Capture(ctx, args...)
	if err != nil {
		return "", fmt.Errorf("tmux capture-pane: %w", err)
	}
	return string(stdout), nil
}

func parsePaneRow(line string) (multiplexer.Pane, error) {
	parts := splitEscaped(line, paneDelimiter)
	if len(parts) != 11 {
		return multiplexer.Pane{}, fmt.Errorf("expected 11 fields, got %d", len(parts))
	}
	activity, err := strconv.ParseInt(parts[5], 10, 64)
	if err != nil {
		return multiplexer.Pane{}, fmt.Errorf("activity: %w", err)
	}
	pid, err := strconv.ParseInt(parts[7], 10, 64)
	if err != nil {
		return multiplexer.Pane{}, fmt.Errorf("pid: %w", err)
	}
	if parts[10] != "0" && parts[10] != "1" {
		return multiplexer.Pane{}, fmt.Errorf("pane mode %q is invalid", parts[10])
	}
	return multiplexer.Pane{
		Target: multiplexer.Target{
			Session: unescapeFormat(parts[0]),
			Window:  parts[1],
			Pane:    parts[2],
		},
		WindowName:       unescapeFormat(parts[3]),
		WorkingDirectory: unescapeFormat(parts[4]),
		Activity:         activity,
		NativeID:         parts[6],
		PID:              pid,
		Title:            unescapeFormat(parts[8]),
		HiveSession:      unescapeFormat(parts[9]),
		InMode:           parts[10] == "1",
	}, nil
}

func splitEscaped(value, separator string) []string {
	var parts []string
	start := 0
	for i := 0; i+len(separator) <= len(value); i++ {
		if value[i:i+len(separator)] != separator || escapedAt(value, i) {
			continue
		}
		parts = append(parts, value[start:i])
		i += len(separator) - 1
		start = i + 1
	}
	return append(parts, value[start:])
}

func escapedAt(value string, index int) bool {
	backslashes := 0
	for index > 0 && value[index-1] == '\\' {
		backslashes++
		index--
	}
	return backslashes%2 == 1
}

func unescapeFormat(value string) string {
	var b strings.Builder
	b.Grow(len(value))
	for i := 0; i < len(value); i++ {
		if value[i] == '\\' && i+1 < len(value) {
			i++
		}
		b.WriteByte(value[i])
	}
	return b.String()
}
