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
// on a width change, as Pi does, sends them again. The window narrows by a
// column through this client's per-window size, then the size is cleared.
func (c *Client) redrawImageWindows(ctx context.Context) {
	windows, err := c.imageWindows(ctx)
	if err != nil {
		if ctx.Err() == nil {
			c.log.Debug().Err(err).Msg("listing image panes failed")
		}
		return
	}
	var narrowed []string
	for _, id := range windows {
		w, ok := c.ctrl.byID(id)
		if !ok || w.Width < 3 || w.Height < 1 {
			continue
		}
		if _, err := c.gw.Send(ctx, fmt.Sprintf("refresh-client -C %s:%dx%d", id, w.Width-1, w.Height)); err != nil {
			c.log.Debug().Err(err).Str("window", id).Msg("narrowing an image window failed")
			continue
		}
		narrowed = append(narrowed, id)
	}
	if len(narrowed) == 0 {
		return
	}
	select {
	case <-time.After(imageRedrawPause):
	case <-ctx.Done():
	}
	// A cancelled context fails these, which is fine: the per-window size
	// belongs to this client and goes away with it.
	for _, id := range narrowed {
		if _, err := c.gw.Send(ctx, "refresh-client -C "+id+":"); err != nil && ctx.Err() == nil {
			c.log.Warn().Err(err).Str("window", id).Msg("restoring an image window's size failed")
		}
	}
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
