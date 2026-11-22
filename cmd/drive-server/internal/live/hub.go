// Package live holds the in-memory hub behind document streams: who is
// viewing each document, and the signals that tell a viewer to re-read.
// It is per process, so a second server replica would need a shared bus.
package live

import (
	"context"
	"crypto/rand"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/colonyops/hive/cmd/drive-server/internal/core"
	"github.com/colonyops/hive/internal/platform/driveapi"
)

const (
	EventPresence = "presence"
	EventThreads  = "threads"
	EventDocument = "document"
	// EventClosed is the last event of a stream whose document was deleted
	// or whose link was revoked. A stream that ends without it, at shutdown,
	// should reconnect.
	EventClosed = "closed"

	// SelectionTTL is how long a selection outlives its last heartbeat.
	SelectionTTL = 40 * time.Second

	maxNameRunes = 40
	streamBuffer = 16
)

type Event struct {
	Name string
	Data any
}

type Key struct{ OwnerID, DocumentID string }

// Viewer is the identity a stream opens with. An anonymous viewer's Name may
// be empty; the hub then makes one up.
type Viewer struct {
	Name     string
	Color    int
	SignedIn bool
	// AccountID groups a signed-in person's streams into one presence entry,
	// so two tabs show one viewer. It never leaves the hub.
	AccountID string
	// ViaLink marks a stream opened through a share link, which LinkClosed
	// ends.
	ViaLink bool
	IP      netip.Addr
}

type Caps struct {
	// PerDocument counts signed-in and anonymous streams together.
	PerDocument          int
	AnonymousPerDocument int
	PerIP                int
	Total                int
}

var DefaultCaps = Caps{PerDocument: 200, AnonymousPerDocument: 100, PerIP: 20, Total: 2000}

var (
	ErrFull    = core.NewError(core.KindTooLarge, "too many viewers")
	ErrStopped = core.NewError(core.KindInvalid, "the server is shutting down")
	errNoToken = core.NewError(core.KindNotFound, "no open stream has this token")
)

type stream struct {
	key      Key
	id       string
	token    string
	viewer   Viewer
	sel      *driveapi.LineRange
	selAt    time.Time
	events   chan Event
	isClosed bool
}

// Hub is safe for concurrent use. It implements core.Notifier.
type Hub struct {
	now  func() time.Time
	caps Caps

	mu      sync.Mutex
	stopped bool
	byKey   map[Key][]*stream
	byToken map[string]*stream
	perIP   map[netip.Addr]int
}

var _ core.Notifier = (*Hub)(nil)

// NewHub returns a hub. A nil now means time.Now.
func NewHub(now func() time.Time, caps Caps) *Hub {
	if now == nil {
		now = time.Now
	}
	return &Hub{
		now:     now,
		caps:    caps,
		byKey:   map[Key][]*stream{},
		byToken: map[string]*stream{},
		perIP:   map[netip.Addr]int{},
	}
}

// Subscribe opens a stream and its viewer. The channel is buffered, and a
// full buffer drops the event: every event means "re-read", and a presence
// snapshot is sent again on the next change. The hub ends a stream by closing
// its channel, after an EventClosed when the document went away. cancel is
// safe to call more than once.
func (h *Hub) Subscribe(key Key, v Viewer) (hello driveapi.StreamHello, events <-chan Event, cancel func(), err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.stopped {
		return driveapi.StreamHello{}, nil, nil, ErrStopped
	}
	if err := h.admit(key, v); err != nil {
		return driveapi.StreamHello{}, nil, nil, err
	}

	s := &stream{
		key:    key,
		id:     "p_" + rand.Text(),
		token:  rand.Text(),
		viewer: v,
		events: make(chan Event, streamBuffer),
	}
	s.viewer.Color = clampColor(v.Color)
	if v.SignedIn {
		s.viewer.Name = strings.TrimSpace(v.Name)
	} else {
		s.viewer.Name = cleanName(v.Name, s.id)
	}
	h.byKey[key] = append(h.byKey[key], s)
	h.byToken[s.token] = s
	h.perIP[v.IP]++
	h.broadcastPresence(key)

	return driveapi.StreamHello{ID: s.id, Token: s.token}, s.events, func() { h.unsubscribe(s) }, nil
}

func (h *Hub) admit(key Key, v Viewer) error {
	if len(h.byToken) >= h.caps.Total || h.perIP[v.IP] >= h.caps.PerIP {
		return ErrFull
	}
	streams := h.byKey[key]
	if len(streams) >= h.caps.PerDocument {
		return ErrFull
	}
	if !v.SignedIn {
		anon := 0
		for _, s := range streams {
			if !s.viewer.SignedIn {
				anon++
			}
		}
		if anon >= h.caps.AnonymousPerDocument {
			return ErrFull
		}
	}
	return nil
}

func (h *Hub) unsubscribe(s *stream) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if s.isClosed {
		return
	}
	h.remove(s, false)
	h.broadcastPresence(s.key)
}

// remove drops s from every index and closes its channel, after an
// EventClosed when final is set. h.mu must be held.
func (h *Hub) remove(s *stream, final bool) {
	s.isClosed = true
	if final {
		// Pending events no longer matter, and dropping them makes room, so
		// the reader always sees why its stream ended.
		for len(s.events) > 0 {
			select {
			case <-s.events:
			default:
			}
		}
		s.offer(Event{Name: EventClosed, Data: struct{}{}})
	}
	close(s.events)
	delete(h.byToken, s.token)
	if h.perIP[s.viewer.IP]--; h.perIP[s.viewer.IP] <= 0 {
		delete(h.perIP, s.viewer.IP)
	}
	rest := slices.DeleteFunc(h.byKey[s.key], func(o *stream) bool { return o == s })
	if len(rest) == 0 {
		delete(h.byKey, s.key)
	} else {
		h.byKey[s.key] = rest
	}
}

// Heartbeat updates the viewer that owns token. name and color apply only to
// an anonymous viewer. A nil or malformed sel clears the selection.
func (h *Hub) Heartbeat(token, name string, color int, sel *driveapi.LineRange) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	s, ok := h.byToken[token]
	if !ok {
		return errNoToken
	}
	before := s.state()
	if !s.viewer.SignedIn {
		s.viewer.Name = cleanName(name, s.id)
		s.viewer.Color = clampColor(color)
	}
	if sel != nil && sel.StartLine >= 1 && sel.EndLine >= sel.StartLine {
		r := *sel
		s.sel, s.selAt = &r, h.now()
	} else {
		s.sel = nil
	}
	if s.state() != before {
		h.broadcastPresence(s.key)
	}
	return nil
}

// Sweep clears selections whose last heartbeat is older than SelectionTTL.
func (h *Hub) Sweep(now time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for key, streams := range h.byKey {
		changed := false
		for _, s := range streams {
			if s.sel != nil && now.Sub(s.selAt) > SelectionTTL {
				s.sel = nil
				changed = true
			}
		}
		if changed {
			h.broadcastPresence(key)
		}
	}
}

// Run sweeps every interval until ctx is done, then closes every stream and
// refuses new ones.
func (h *Hub) Run(ctx context.Context, every time.Duration) error {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			h.stop()
			return nil
		case <-t.C:
			h.Sweep(h.now())
		}
	}
}

func (h *Hub) stop() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.stopped = true
	for _, s := range h.byToken {
		h.remove(s, false)
	}
}

func (h *Hub) DocumentChanged(ownerID, documentID, sha256 string) {
	h.send(Key{ownerID, documentID}, Event{Name: EventDocument, Data: driveapi.DocumentEvent{SHA256: sha256}})
}

func (h *Hub) ThreadsChanged(ownerID, documentID string) {
	h.send(Key{ownerID, documentID}, Event{Name: EventThreads, Data: struct{}{}})
}

// DocumentClosed ends every stream on the document.
func (h *Hub) DocumentClosed(ownerID, documentID string) {
	h.close(Key{ownerID, documentID}, func(*stream) bool { return true })
}

// LinkClosed ends the document's streams that came through its share link.
func (h *Hub) LinkClosed(ownerID, documentID string) {
	h.close(Key{ownerID, documentID}, func(s *stream) bool { return s.viewer.ViaLink })
}

func (h *Hub) close(key Key, match func(*stream) bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	removed := false
	for _, s := range slices.Clone(h.byKey[key]) {
		if match(s) {
			h.remove(s, true)
			removed = true
		}
	}
	if removed {
		h.broadcastPresence(key)
	}
}

func (h *Hub) send(key Key, e Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, s := range h.byKey[key] {
		s.offer(e)
	}
}

// broadcastPresence sends each stream on key the viewer list, with Self set
// on its own entry. A signed-in account's streams share one entry, which
// shows the most recent selection among them. h.mu must be held.
func (h *Hub) broadcastPresence(key Key) {
	streams := h.byKey[key]
	groups := make([][]*stream, 0, len(streams))
	byAccount := map[string]int{}
	for _, s := range streams {
		if id := s.viewer.AccountID; s.viewer.SignedIn && id != "" {
			if i, ok := byAccount[id]; ok {
				groups[i] = append(groups[i], s)
				continue
			}
			byAccount[id] = len(groups)
		}
		groups = append(groups, []*stream{s})
	}
	for _, to := range streams {
		viewers := make([]driveapi.PresenceViewer, 0, len(groups))
		for _, g := range groups {
			viewers = append(viewers, snapshot(g, to))
		}
		to.offer(Event{Name: EventPresence, Data: driveapi.PresenceSnapshot{Viewers: viewers}})
	}
}

func (s *stream) offer(e Event) {
	select {
	case s.events <- e:
	default:
	}
}

// viewerState is comparable, so Heartbeat can tell whether anything changed.
type viewerState struct {
	name       string
	color      int
	start, end int
}

func (s *stream) state() viewerState {
	st := viewerState{name: s.viewer.Name, color: s.viewer.Color}
	if s.sel != nil {
		st.start, st.end = s.sel.StartLine, s.sel.EndLine
	}
	return st
}

// snapshot is one presence entry for a group of streams, keyed by its oldest
// stream so the id stays put while that stream lives.
func snapshot(group []*stream, to *stream) driveapi.PresenceViewer {
	first := group[0]
	v := driveapi.PresenceViewer{ID: first.id, Name: first.viewer.Name, Color: first.viewer.Color, SignedIn: first.viewer.SignedIn}
	var latest *stream
	for _, s := range group {
		if s == to {
			v.Self = true
		}
		if s.sel != nil && (latest == nil || s.selAt.After(latest.selAt)) {
			latest = s
		}
	}
	if latest != nil {
		r := *latest.sel
		v.Selection = &r
	}
	return v
}

func clampColor(c int) int {
	if c < 0 || c >= driveapi.PresenceColors {
		return 0
	}
	return c
}

// cleanName drops control characters and cuts name to maxNameRunes. An empty
// result becomes a name made from seed.
func cleanName(name, seed string) string {
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return -1
		}
		return r
	}, name)
	name = strings.TrimSpace(name)
	if r := []rune(name); len(r) > maxNameRunes {
		name = strings.TrimSpace(string(r[:maxNameRunes]))
	}
	if name == "" {
		return generatedName(seed)
	}
	return name
}
