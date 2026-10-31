package tmuxcc

import (
	"context"
	"strings"
	"sync"
)

const (
	aggressiveResize = "aggressive-resize"
	// aggressiveResizeMark holds a window's own aggressive-resize while Hive has
	// it off. It lives in tmux so that a client that ends without Close leaves the
	// way back behind, for whichever client closes cleanly next.
	aggressiveResizeMark = "@hive-aggressive-resize"
	// aggressiveResizeInherited is the mark of a window with no value of its own.
	aggressiveResizeInherited = "inherit"
)

// windowSizing is what a client knows about the windows it gave a size of their
// own. The mutex is held across gateway round trips, each bounded by
// imageRestoreTimeout, so that a resize and Close cannot interleave.
type windowSizing struct {
	mu       sync.Mutex
	prepared map[string]struct{}
	closed   bool
}

// sizeWhileNotCurrent makes tmux honour a window's own size when it is not the
// session's current window. With aggressive-resize on, tmux sizes only the
// current window, so a window beside it in a split would overflow its pane until
// something activated it. Selecting the window instead would move the session's
// focus for every client. The option has to change before the size is sent: tmux
// does not apply a size it ignored earlier.
//
// A failure here is logged and not returned, because the size is still worth
// sending: the option is commonly off, and then nothing was needed. The window
// is not recorded as prepared, so the next resize tries again.
func (c *Client) sizeWhileNotCurrent(ctx context.Context, id string) {
	c.sizing.mu.Lock()
	defer c.sizing.mu.Unlock()
	if _, prepared := c.sizing.prepared[id]; prepared || c.sizing.closed {
		return
	}
	// Not cancelled with the request: tmux can apply a change before a cancelled
	// Send returns, and the mark and the option must not part ways. Bounded
	// because a frozen server must not hold Close behind this mutex.
	prepare, cancel := context.WithTimeout(context.WithoutCancel(ctx), imageRestoreTimeout)
	defer cancel()
	if err := c.turnOffAggressiveResize(prepare, id); err != nil {
		c.log.Debug().Err(err).Str("window", id).Msg("preparing a window for its own size failed")
		return
	}
	if c.sizing.prepared == nil {
		c.sizing.prepared = map[string]struct{}{}
	}
	c.sizing.prepared[id] = struct{}{}
}

func (c *Client) turnOffAggressiveResize(ctx context.Context, id string) error {
	effective, err := c.windowOption(ctx, id, "-A", aggressiveResize)
	if err != nil || effective != "on" {
		return err
	}
	mark, err := c.windowOption(ctx, id, "-q", aggressiveResizeMark)
	if err != nil {
		return err
	}
	if mark == "" {
		own, err := c.windowOption(ctx, id, "-q", aggressiveResize)
		if err != nil {
			return err
		}
		if own == "" {
			own = aggressiveResizeInherited
		}
		// The mark goes first: a client that dies between the two leaves a mark
		// on an unchanged window, which restores to what it already is.
		if _, err := c.gw.Send(ctx, "set-option -w -t "+id+" "+aggressiveResizeMark+" "+own); err != nil {
			return err
		}
	}
	_, err = c.gw.Send(ctx, "set-option -w -t "+id+" "+aggressiveResize+" off")
	return err
}

// clearWindowSizes drops the per-window sizes this client asked for. A surface
// that re-attaches to a live client starts its votes over, and a size it no
// longer knows about would otherwise outrank the session's size for good.
func (c *Client) clearWindowSizes(ctx context.Context) {
	c.sizing.mu.Lock()
	defer c.sizing.mu.Unlock()
	for id := range c.sizing.prepared {
		if _, err := c.gw.Send(ctx, "refresh-client -C "+id+":"); err != nil {
			c.log.Debug().Err(err).Str("window", id).Msg("clearing a window's own size failed")
		}
	}
}

// restoreAggressiveResize hands the marked windows back, those a crashed client
// left included. It waits for the last control client on the session, because
// another one may still be sizing these windows.
func (c *Client) restoreAggressiveResize(ctx context.Context) {
	c.sizing.mu.Lock()
	defer c.sizing.mu.Unlock()
	c.sizing.closed = true

	restore, cancel := context.WithTimeout(context.WithoutCancel(ctx), imageRestoreTimeout)
	defer cancel()
	if c.otherControlClients(restore) {
		return
	}
	lines, err := c.gw.Send(restore, `list-windows -F "#{window_id} #{`+aggressiveResizeMark+`}"`)
	if err != nil {
		c.log.Debug().Err(err).Msg("listing windows to restore aggressive-resize failed")
		return
	}
	for _, line := range lines {
		id, mark, _ := strings.Cut(line, " ")
		if !validWindowID(id) {
			continue
		}
		var cmd string
		switch mark {
		case aggressiveResizeInherited:
			cmd = "set-option -w -t " + id + " -u " + aggressiveResize
		case "on", "off":
			cmd = "set-option -w -t " + id + " " + aggressiveResize + " " + mark
		default:
			continue
		}
		if _, err := c.gw.Send(restore, cmd); err != nil {
			c.log.Warn().Err(err).Str("window", id).Msg("restoring a window's aggressive-resize failed")
			continue
		}
		if _, err := c.gw.Send(restore, "set-option -w -t "+id+" -u "+aggressiveResizeMark); err != nil {
			c.log.Debug().Err(err).Str("window", id).Msg("clearing a window's aggressive-resize mark failed")
		}
	}
}

// otherControlClients reports another control client on this session. A failed
// read counts as none: leaving the option changed is the worse of the two.
func (c *Client) otherControlClients(ctx context.Context) bool {
	lines, err := c.gw.Send(ctx, `list-clients -F "#{client_control_mode} #{client_session}"`)
	if err != nil {
		c.log.Debug().Err(err).Msg("listing clients to restore aggressive-resize failed")
		return false
	}
	control := 0
	for _, line := range lines {
		if mode, session, _ := strings.Cut(line, " "); mode == "1" && session == c.slug {
			control++
		}
	}
	return control > 1
}
