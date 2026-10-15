package tmuxcc

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"
)

// imagePaneOption marks a pane whose output has carried a Kitty graphics
// command. tmux keeps pane options for the pane's life, so the mark survives
// this app restarting and tells a later attach which windows to redraw.
const imagePaneOption = "@hive-kitty-images"

// imageRedrawPause is how long a window stays one column narrower. The program
// has to read its size while it is still the narrow one: a restore that lands
// first reads as no change at all.
const imageRedrawPause = 150 * time.Millisecond

const imageRestoreTimeout = 2 * time.Second

var kittyCommandStart = []byte("\x1b_G")

// imageDetector reports the first Kitty graphics command in each pane's
// output, including one whose opening bytes straddle two %output lines. Only
// the reader goroutine uses it.
type imageDetector struct {
	tail map[string][]byte
	seen map[string]bool
}

func newImageDetector() *imageDetector {
	return &imageDetector{tail: map[string][]byte{}, seen: map[string]bool{}}
}

func (d *imageDetector) first(pane string, data []byte) bool {
	if d.seen[pane] {
		return false
	}
	keep := len(kittyCommandStart) - 1
	edge := append(append([]byte(nil), d.tail[pane]...), data[:min(len(data), keep)]...)
	if bytes.Contains(edge, kittyCommandStart) || bytes.Contains(data, kittyCommandStart) {
		d.seen[pane] = true
		delete(d.tail, pane)
		return true
	}
	if len(data) >= keep {
		edge = append(edge[:0], data[len(data)-keep:]...)
	} else if len(edge) > keep {
		edge = edge[len(edge)-keep:]
	}
	d.tail[pane] = edge
	return false
}

// noteImages queues the pane mark for the worker, since the reader must never
// wait on a command. A full queue drops the mark; the next attach then skips
// that pane's redraw, which only costs its images until the program draws
// again.
func (c *Client) noteImages(pane string, data []byte) {
	if !c.images.first(pane, data) {
		return
	}
	select {
	case c.imagePanes <- pane:
	default:
		c.log.Debug().Str("pane", pane).Msg("image pane mark dropped")
	}
}

func (c *Client) markImagePane(ctx context.Context, pane string) {
	if _, err := c.gw.Send(ctx, "set-option -p -t "+pane+" "+imagePaneOption+" 1"); err != nil && ctx.Err() == nil {
		c.log.Debug().Err(err).Str("pane", pane).Msg("marking an image pane failed")
	}
}

// redrawImageWindows asks the programs in every window that has drawn a Kitty
// image to draw again. Images live only in the renderer, so a first paint
// rebuilt from capture-pane carries none; a program that redraws everything
// on a width change, as Pi does, sends them again.
//
// The window narrows by a column with resize-window, which sets its
// window-size option to manual, and the option then goes back to what it was.
// A per-window client size would be gentler, but tmux honours it only from
// the window's latest client, and another app attached to the same session
// usually is that client. A window whose size is manual anyway keeps the width
// resize-window stored, so that width is put back explicitly.
func (c *Client) redrawImageWindows(ctx context.Context) {
	windows, err := c.imageWindows(ctx)
	if err != nil {
		if ctx.Err() == nil {
			c.log.Debug().Err(err).Msg("listing image panes failed")
		}
		return
	}
	type narrowedWindow struct {
		id, size string
		width    int
		manual   bool
	}
	var narrowed []narrowedWindow
	for _, id := range windows {
		w, ok := c.ctrl.byID(id)
		if !ok || w.Width < 3 {
			continue
		}
		local, err := c.windowSizeOption(ctx, id, "-q")
		if err != nil {
			c.log.Debug().Err(err).Str("window", id).Msg("reading an image window's size option failed")
			continue
		}
		effective, err := c.windowSizeOption(ctx, id, "-A")
		if err != nil {
			c.log.Debug().Err(err).Str("window", id).Msg("reading an image window's size option failed")
			continue
		}
		if _, err := c.gw.Send(ctx, fmt.Sprintf("resize-window -t %s -x %d", id, w.Width-1)); err != nil {
			c.log.Debug().Err(err).Str("window", id).Msg("narrowing an image window failed")
			continue
		}
		narrowed = append(narrowed, narrowedWindow{id: id, size: local, width: w.Width, manual: effective == "manual"})
	}
	if len(narrowed) == 0 {
		return
	}
	select {
	case <-time.After(imageRedrawPause):
	case <-ctx.Done():
	}
	// Restored even after cancellation: a window left on window-size manual
	// keeps the narrow size for every client. Close waits for this before it
	// detaches, so the gateway can still carry it.
	restore, cancel := context.WithTimeout(context.WithoutCancel(ctx), imageRestoreTimeout)
	defer cancel()
	for _, w := range narrowed {
		if w.manual {
			if _, err := c.gw.Send(restore, fmt.Sprintf("resize-window -t %s -x %d", w.id, w.width)); err != nil {
				c.log.Warn().Err(err).Str("window", w.id).Msg("restoring an image window's width failed")
			}
		}
		cmd := "set-option -w -t " + w.id + " -u window-size"
		if w.size != "" {
			cmd = "set-option -w -t " + w.id + " window-size " + w.size
		}
		if _, err := c.gw.Send(restore, cmd); err != nil {
			c.log.Warn().Err(err).Str("window", w.id).Msg("restoring an image window's size failed")
		}
	}
}

// windowSizeOption reads a window's window-size: its own value with -q, or the
// value in effect, inherited included, with -A.
func (c *Client) windowSizeOption(ctx context.Context, id, flag string) (string, error) {
	lines, err := c.gw.Send(ctx, "show-options -w -v "+flag+" -t "+id+" window-size")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(strings.Join(lines, "")), nil
}

func (c *Client) imageWindows(ctx context.Context) ([]string, error) {
	lines, err := c.gw.Send(ctx, `list-panes -s -F "#{window_id} #{`+imagePaneOption+`}"`)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var windows []string
	for _, line := range lines {
		id, mark, _ := strings.Cut(line, " ")
		if mark == "1" && validWindowID(id) && !seen[id] {
			seen[id] = true
			windows = append(windows, id)
		}
	}
	return windows, nil
}
