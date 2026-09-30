package tmuxcc

import (
	"slices"
	"sync"
)

// Window is the app-side model of one tmux window.
type Window struct {
	ID         string // tmux window id, e.g. "@275"
	Name       string
	Active     bool
	ActivePane string // tmux pane id, e.g. "%512"
	// Width and Height are tmux's own size for this window — whichever attached
	// client tmux's window-size option picked, not necessarily ours. A renderer
	// that draws at any other size mangles the pane's cursor-addressed output.
	// 0 means not known yet.
	Width  int
	Height int
	// Layout is the pane tree, at the window's unzoomed geometry; the zero
	// value means tmux has not reported one yet. Zoomed says the active pane is
	// drawn over the whole window while Layout still records where it goes
	// back to.
	Layout Layout
	Zoomed bool
}

// Panes lists the window's pane ids in layout order. A window whose layout
// has not been read yet offers only its active pane.
func (w Window) Panes() []string {
	if panes := w.Layout.Panes(); len(panes) > 0 {
		return panes
	}
	if w.ActivePane != "" {
		return []string{w.ActivePane}
	}
	return nil
}

// A missing layout is consistent until reconciliation supplies its tree.
func (w Window) consistent() bool {
	panes := w.Layout.Panes()
	return len(panes) == 0 || slices.Contains(panes, w.ActivePane)
}

type controller struct {
	mu      sync.Mutex
	order   []string
	windows map[string]Window
	panes   map[string]string // pane id -> window id
	// held names windows whose stored state has not been published. tmux
	// announces a split or a closed pane as a %layout-change and a
	// %window-pane-changed, in either order, and between the two the window's
	// active pane is outside its layout. A consumer reads every window event
	// as a whole snapshot, so the window goes out once both have landed.
	held map[string]bool
	// generation increments on every add the reader folds in. addedAt records
	// the generation a window entered the set under, so a merge can tell a
	// window its snapshot could not have seen from one the snapshot dropped.
	generation uint64
	addedAt    map[string]uint64
}

func newController() *controller {
	return &controller{
		windows: map[string]Window{},
		panes:   map[string]string{},
		held:    map[string]bool{},
		addedAt: map[string]uint64{},
	}
}

// mark reports the generation a caller is about to take a snapshot at. A
// reconcile of that snapshot passes it back so windows added during the round
// trip -- after the mark, so the snapshot cannot hold them -- are not mistaken
// for windows the snapshot legitimately omits because tmux closed them (#278).
func (c *controller) mark() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.generation
}

// apply folds one notification into the window set and returns the events it
// produces. Output is routed by the client instead: it is buffered by pane
// until first-paint completes, at which point the window set is known.
// apply is called on the reader goroutine and never blocks.
func (c *controller) apply(n Notification) []Event {
	c.mu.Lock()
	defer c.mu.Unlock()

	switch v := n.(type) {
	case WindowAddNotification:
		if _, ok := c.windows[v.Window]; ok {
			return nil
		}
		c.generation++
		c.addedAt[v.Window] = c.generation
		w := Window{ID: v.Window}
		c.windows[v.Window] = w
		c.order = append(c.order, v.Window)
		return []Event{WindowChanged{Kind: WindowAdded, Window: w}}

	case WindowCloseNotification:
		w, ok := c.windows[v.Window]
		if !ok {
			return nil
		}
		c.removeLocked(v.Window)
		return []Event{WindowChanged{Kind: WindowClosed, Window: w}}

	case WindowRenamedNotification:
		w, ok := c.windows[v.Window]
		if !ok {
			return nil
		}
		w.Name = v.Name
		return c.settleLocked(w, WindowRenamed)

	case WindowPaneChanged:
		w, ok := c.windows[v.Window]
		if !ok {
			return nil
		}
		w.ActivePane = v.Pane
		c.panes[v.Pane] = v.Window
		return c.settleLocked(w, WindowActiveChanged)

	case SessionWindowChanged:
		if _, ok := c.windows[v.Window]; !ok {
			return nil
		}
		c.setActiveLocked(v.Window)
		return []Event{WindowChanged{Kind: WindowActiveChanged, Window: c.windows[v.Window]}}

	case LayoutChanged:
		w, ok := c.windows[v.Window]
		if !ok || v.Layout.Width == 0 || v.Layout.Height == 0 {
			return nil
		}
		if w.Layout.Equal(v.Layout) && w.Zoomed == v.Zoomed {
			return nil
		}
		w.Layout, w.Zoomed = v.Layout, v.Zoomed
		w.Width, w.Height = v.Layout.Width, v.Layout.Height
		return c.settleLocked(w, WindowLayoutChanged)

	case PauseNotification:
		return []Event{LifecycleChanged{Kind: LifecyclePaused, WindowID: c.panes[v.Pane]}}

	case ContinueNotification:
		return []Event{LifecycleChanged{Kind: LifecycleResumed, WindowID: c.panes[v.Pane]}}

	default:
		return nil
	}
}

func (c *controller) settleLocked(w Window, kind WindowEventKind) []Event {
	c.storeLocked(w)
	if !w.consistent() {
		c.held[w.ID] = true
		return nil
	}
	if c.held[w.ID] {
		delete(c.held, w.ID)
		kind = WindowLayoutChanged
	}
	return []Event{WindowChanged{Kind: kind, Window: w}}
}

// reconcile merges an authoritative list-windows snapshot taken at generation
// since and reports what changed. One window yields at most one event: every
// window event carries the whole window, so a consumer reads the current
// layout off whichever kind it gets.
//
// since is the value mark returned right before the snapshot's round trip
// started. A window added afterward cannot be in the snapshot, so its absence
// is not evidence tmux closed it -- the removal pass leaves it alone (#278).
func (c *controller) reconcile(next []Window, since uint64) []Event {
	c.mu.Lock()
	defer c.mu.Unlock()

	var events []Event
	seen := make(map[string]bool, len(next))
	for _, w := range next {
		seen[w.ID] = true
		prev, existed := c.windows[w.ID]
		c.storeLocked(w)
		// The snapshot has now seen this window, so it is ordinary again and a
		// later snapshot may legitimately report it closed.
		delete(c.addedAt, w.ID)
		// A snapshot row is consistent by construction, so it lifts a hold.
		held := c.held[w.ID]
		delete(c.held, w.ID)
		switch {
		case !existed:
			events = append(events, WindowChanged{Kind: WindowAdded, Window: w})
		case prev.Name != w.Name:
			events = append(events, WindowChanged{Kind: WindowRenamed, Window: w})
		case prev.Active != w.Active || prev.ActivePane != w.ActivePane:
			events = append(events, WindowChanged{Kind: WindowActiveChanged, Window: w})
		case held || prev.Width != w.Width || prev.Height != w.Height || prev.Zoomed != w.Zoomed || !prev.Layout.Equal(w.Layout):
			events = append(events, WindowChanged{Kind: WindowLayoutChanged, Window: w})
		}
	}

	oldOrder := append([]string(nil), c.order...)
	tooNew := make(map[string]bool)
	for _, id := range oldOrder {
		if seen[id] {
			continue
		}
		if c.addedAt[id] > since {
			tooNew[id] = true
			continue
		}
		w := c.windows[id]
		c.removeLocked(id)
		events = append(events, WindowChanged{Kind: WindowClosed, Window: w})
	}

	// tmux's order is authoritative for every window the snapshot saw. A window
	// the snapshot is too old to hold keeps its place at the end, which is where
	// %window-add put it; the next snapshot that does see it settles its order.
	c.order = c.order[:0]
	for _, w := range next {
		c.order = append(c.order, w.ID)
	}
	for _, id := range oldOrder {
		if tooNew[id] {
			c.order = append(c.order, id)
		}
	}

	return events
}

// set installs the initial window set without deriving events — Attach returns
// the snapshot to its caller directly. It is authoritative, the same as a
// reconcile with nothing held back: no window survives it as a placeholder.
func (c *controller) set(next []Window) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, w := range next {
		c.storeLocked(w)
		delete(c.addedAt, w.ID)
		delete(c.held, w.ID)
	}
}

// Windows returns a snapshot of the current window set in tmux index order.
func (c *controller) Windows() []Window {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]Window, 0, len(c.order))
	for _, id := range c.order {
		out = append(out, c.windows[id])
	}
	return out
}

func (c *controller) windowForPane(pane string) (Window, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	w, ok := c.windows[c.panes[pane]]
	return w, ok
}

func (c *controller) byID(id string) (Window, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	w, ok := c.windows[id]
	return w, ok
}

// storeLocked keeps the pane index additive: a pane that leaves the layout
// still resolves to its window, so its last bytes are streamed and counted
// rather than dropped as unroutable. tmux never reuses a pane id.
func (c *controller) storeLocked(w Window) {
	if _, existed := c.windows[w.ID]; !existed {
		c.order = append(c.order, w.ID)
	}
	c.windows[w.ID] = w
	for _, pane := range w.Panes() {
		c.panes[pane] = w.ID
	}
}

func (c *controller) removeLocked(id string) {
	for pane, owner := range c.panes {
		if owner == id {
			delete(c.panes, pane)
		}
	}
	delete(c.windows, id)
	delete(c.addedAt, id)
	delete(c.held, id)
	for i, existing := range c.order {
		if existing == id {
			c.order = append(c.order[:i], c.order[i+1:]...)
			break
		}
	}
}

func (c *controller) setActiveLocked(id string) {
	for wid, w := range c.windows {
		active := wid == id
		if w.Active != active {
			w.Active = active
			c.windows[wid] = w
		}
	}
}
