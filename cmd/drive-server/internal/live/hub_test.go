package live

import (
	"context"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/drive-server/internal/core"
	"github.com/colonyops/hive/internal/platform/driveapi"
)

var (
	doc   = Key{OwnerID: "u1", DocumentID: "d1"}
	other = Key{OwnerID: "u1", DocumentID: "d2"}
	ipA   = netip.MustParseAddr("192.0.2.1")
)

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func newHub(caps Caps) (*Hub, *clock) {
	c := &clock{t: time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)}
	return NewHub(c.now, caps), c
}

func subscribe(t *testing.T, h *Hub, key Key, v Viewer) (driveapi.StreamHello, <-chan Event) {
	t.Helper()
	hello, events, cancel, err := h.Subscribe(key, v)
	require.NoError(t, err)
	t.Cleanup(cancel)
	return hello, events
}

func subscribeErr(h *Hub, key Key, v Viewer) error {
	_, _, cancel, err := h.Subscribe(key, v)
	if cancel != nil {
		cancel()
	}
	return err
}

func drain(events <-chan Event) []Event {
	var out []Event
	for {
		select {
		case e, ok := <-events:
			if !ok {
				return out
			}
			out = append(out, e)
		default:
			return out
		}
	}
}

func lastPresence(t *testing.T, events <-chan Event) []driveapi.PresenceViewer {
	t.Helper()
	var snap *driveapi.PresenceSnapshot
	for _, e := range drain(events) {
		if e.Name == EventPresence {
			s := e.Data.(driveapi.PresenceSnapshot)
			snap = &s
		}
	}
	require.NotNil(t, snap, "no presence event")
	return snap.Viewers
}

func closed(events <-chan Event) bool {
	_, ok := ended(events)
	return ok
}

// ended reports whether events is closed, and the name of its last event.
func ended(events <-chan Event) (string, bool) {
	last := ""
	for {
		select {
		case e, ok := <-events:
			if !ok {
				return last, true
			}
			last = e.Name
		default:
			return "", false
		}
	}
}

func TestSubscribeSendsPresenceWithSelf(t *testing.T) {
	h, _ := newHub(DefaultCaps)
	helloA, a := subscribe(t, h, doc, Viewer{Name: "Ann", SignedIn: true, IP: ipA})
	helloB, b := subscribe(t, h, doc, Viewer{IP: ipA})

	assert.NotEqual(t, helloA.Token, helloB.Token)
	assert.True(t, strings.HasPrefix(helloA.ID, "p_"))

	viewers := lastPresence(t, a)
	require.Len(t, viewers, 2)
	assert.Equal(t, helloA.ID, viewers[0].ID)
	assert.True(t, viewers[0].Self)
	assert.False(t, viewers[1].Self)
	assert.NotEmpty(t, viewers[1].Name, "an anonymous viewer gets a generated name")
	assert.False(t, viewers[1].SignedIn)

	viewers = lastPresence(t, b)
	assert.False(t, viewers[0].Self)
	assert.True(t, viewers[1].Self)
}

func TestHeartbeatNeedsTheStreamsToken(t *testing.T) {
	h, _ := newHub(DefaultCaps)
	helloA, a := subscribe(t, h, doc, Viewer{IP: ipA})
	subscribe(t, h, other, Viewer{IP: ipA})

	err := h.Heartbeat("made-up", "x", 0, nil)
	assert.True(t, core.IsKind(err, core.KindNotFound))

	drain(a)
	require.NoError(t, h.Heartbeat(helloA.Token, "Ann", 3, &driveapi.LineRange{StartLine: 2, EndLine: 4}))
	viewers := lastPresence(t, a)
	require.Len(t, viewers, 1, "a heartbeat reaches only its own document")
	assert.Equal(t, "Ann", viewers[0].Name)
	assert.Equal(t, 3, viewers[0].Color)
	assert.Equal(t, &driveapi.LineRange{StartLine: 2, EndLine: 4}, viewers[0].Selection)
}

func TestHeartbeatKeepsASignedInName(t *testing.T) {
	h, _ := newHub(DefaultCaps)
	hello, events := subscribe(t, h, doc, Viewer{Name: "Ann", Color: 2, SignedIn: true, IP: ipA})
	require.NoError(t, h.Heartbeat(hello.Token, "Mallory", 5, &driveapi.LineRange{StartLine: 1, EndLine: 1}))
	v := lastPresence(t, events)[0]
	assert.Equal(t, "Ann", v.Name)
	assert.Equal(t, 2, v.Color)
}

func TestHeartbeatCleansAnonymousInput(t *testing.T) {
	h, _ := newHub(DefaultCaps)
	hello, events := subscribe(t, h, doc, Viewer{IP: ipA})
	long := strings.Repeat("é", 60)
	require.NoError(t, h.Heartbeat(hello.Token, "a\x00b\u202e"+long, 99, &driveapi.LineRange{StartLine: 5, EndLine: 2}))
	v := lastPresence(t, events)[0]
	assert.Equal(t, "ab"+strings.Repeat("é", 38), v.Name)
	assert.Equal(t, 0, v.Color)
	assert.Nil(t, v.Selection, "a backwards range is dropped")
}

func TestHeartbeatWithNoChangeSendsNothing(t *testing.T) {
	h, _ := newHub(DefaultCaps)
	hello, events := subscribe(t, h, doc, Viewer{Name: "Ann", SignedIn: true, IP: ipA})
	sel := &driveapi.LineRange{StartLine: 1, EndLine: 2}
	require.NoError(t, h.Heartbeat(hello.Token, "", 0, sel))
	drain(events)
	require.NoError(t, h.Heartbeat(hello.Token, "", 0, sel))
	assert.Empty(t, drain(events))
}

func TestSweepClearsStaleSelections(t *testing.T) {
	h, c := newHub(DefaultCaps)
	hello, events := subscribe(t, h, doc, Viewer{Name: "Ann", SignedIn: true, IP: ipA})
	require.NoError(t, h.Heartbeat(hello.Token, "", 0, &driveapi.LineRange{StartLine: 1, EndLine: 1}))
	drain(events)

	h.Sweep(c.t.Add(SelectionTTL))
	assert.Empty(t, drain(events), "a selection lives for the whole TTL")

	h.Sweep(c.t.Add(SelectionTTL + time.Second))
	viewers := lastPresence(t, events)
	require.Len(t, viewers, 1, "the viewer stays while its stream is open")
	assert.Nil(t, viewers[0].Selection)
}

func TestOneAccountIsOneViewer(t *testing.T) {
	h, c := newHub(DefaultCaps)
	ann := Viewer{Name: "Ann", SignedIn: true, AccountID: "u_ann", IP: ipA}
	helloA, a := subscribe(t, h, doc, ann)
	helloB, b, cancelB, err := h.Subscribe(doc, ann)
	require.NoError(t, err)
	_, bob := subscribe(t, h, doc, Viewer{Name: "Bob", SignedIn: true, AccountID: "u_bob", IP: ipA})

	viewers := lastPresence(t, a)
	require.Len(t, viewers, 2, "two tabs of one account show once")
	assert.Equal(t, helloA.ID, viewers[0].ID, "the oldest stream names the entry")
	assert.True(t, viewers[0].Self)
	assert.True(t, lastPresence(t, b)[0].Self, "either tab sees the entry as itself")
	assert.False(t, lastPresence(t, bob)[0].Self)

	require.NoError(t, h.Heartbeat(helloA.Token, "", 0, &driveapi.LineRange{StartLine: 1, EndLine: 1}))
	c.t = c.t.Add(time.Second)
	require.NoError(t, h.Heartbeat(helloB.Token, "", 0, &driveapi.LineRange{StartLine: 7, EndLine: 9}))
	assert.Equal(t, &driveapi.LineRange{StartLine: 7, EndLine: 9}, lastPresence(t, bob)[0].Selection, "the latest selection shows")

	cancelB()
	viewers = lastPresence(t, bob)
	require.Len(t, viewers, 2, "the account stays while one tab is open")
	assert.Equal(t, &driveapi.LineRange{StartLine: 1, EndLine: 1}, viewers[0].Selection)
}

func TestCancelRemovesTheViewer(t *testing.T) {
	h, _ := newHub(DefaultCaps)
	_, a := subscribe(t, h, doc, Viewer{IP: ipA})
	helloB, _, cancelB, err := h.Subscribe(doc, Viewer{IP: ipA})
	require.NoError(t, err)

	cancelB()
	cancelB()
	viewers := lastPresence(t, a)
	assert.Len(t, viewers, 1)
	assert.Error(t, h.Heartbeat(helloB.Token, "", 0, nil))
}

func TestCaps(t *testing.T) {
	caps := Caps{PerDocument: 3, AnonymousPerDocument: 2, PerIP: 4, Total: 5}
	ip := func(n byte) netip.Addr { return netip.AddrFrom4([4]byte{192, 0, 2, n}) }

	t.Run("anonymous cap leaves room for signed-in viewers", func(t *testing.T) {
		h, _ := newHub(caps)
		subscribe(t, h, doc, Viewer{IP: ip(1)})
		subscribe(t, h, doc, Viewer{IP: ip(2)})
		err := subscribeErr(h, doc, Viewer{IP: ip(3)})
		require.ErrorIs(t, err, ErrFull)
		subscribe(t, h, doc, Viewer{SignedIn: true, IP: ip(3)})
		err = subscribeErr(h, doc, Viewer{SignedIn: true, IP: ip(4)})
		assert.ErrorIs(t, err, ErrFull, "per document")
	})

	t.Run("per IP", func(t *testing.T) {
		h, _ := newHub(caps)
		for _, k := range []Key{doc, other, {"u2", "d1"}, {"u2", "d2"}} {
			subscribe(t, h, k, Viewer{SignedIn: true, IP: ipA})
		}
		err := subscribeErr(h, Key{"u3", "d1"}, Viewer{SignedIn: true, IP: ipA})
		assert.ErrorIs(t, err, ErrFull)
	})

	t.Run("total", func(t *testing.T) {
		h, _ := newHub(caps)
		for i := range byte(5) {
			subscribe(t, h, Key{"u", string('a' + i)}, Viewer{SignedIn: true, IP: ip(i)})
		}
		err := subscribeErr(h, Key{"u", "z"}, Viewer{SignedIn: true, IP: ip(9)})
		require.ErrorIs(t, err, ErrFull)
		assert.True(t, core.IsKind(err, core.KindTooLarge))
	})

	t.Run("a closed stream frees its slot", func(t *testing.T) {
		h, _ := newHub(caps)
		subscribe(t, h, doc, Viewer{IP: ip(1)})
		_, _, cancel, err := h.Subscribe(doc, Viewer{IP: ip(2)})
		require.NoError(t, err)
		cancel()
		subscribe(t, h, doc, Viewer{IP: ip(2)})
	})
}

func TestSignalsReachOnlyTheirDocument(t *testing.T) {
	h, _ := newHub(DefaultCaps)
	_, a := subscribe(t, h, doc, Viewer{IP: ipA})
	_, b := subscribe(t, h, other, Viewer{IP: ipA})
	drain(a)
	drain(b)

	h.ThreadsChanged(doc.OwnerID, doc.DocumentID)
	h.DocumentChanged(doc.OwnerID, doc.DocumentID, "abc")
	got := drain(a)
	require.Len(t, got, 2)
	assert.Equal(t, EventThreads, got[0].Name)
	assert.Equal(t, Event{Name: EventDocument, Data: driveapi.DocumentEvent{SHA256: "abc"}}, got[1])
	assert.Empty(t, drain(b))
}

func TestASlowSubscriberDoesNotBlock(t *testing.T) {
	h, _ := newHub(DefaultCaps)
	_, events := subscribe(t, h, doc, Viewer{IP: ipA})

	done := make(chan struct{})
	go func() {
		for range 10 * streamBuffer {
			h.ThreadsChanged(doc.OwnerID, doc.DocumentID)
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("ThreadsChanged blocked on a full stream")
	}
	assert.Len(t, drain(events), streamBuffer)
}

func TestDocumentClosedEndsItsStreams(t *testing.T) {
	h, _ := newHub(DefaultCaps)
	hello, a := subscribe(t, h, doc, Viewer{IP: ipA})
	_, b := subscribe(t, h, other, Viewer{IP: ipA})

	for range 2 * streamBuffer {
		h.ThreadsChanged(doc.OwnerID, doc.DocumentID)
	}
	h.DocumentClosed(doc.OwnerID, doc.DocumentID)
	last, done := ended(a)
	assert.True(t, done)
	assert.Equal(t, EventClosed, last, "a full buffer still ends with closed")
	assert.False(t, closed(b))
	assert.Error(t, h.Heartbeat(hello.Token, "", 0, nil))
}

func TestLinkClosedEndsOnlyLinkStreams(t *testing.T) {
	h, _ := newHub(DefaultCaps)
	_, owner := subscribe(t, h, doc, Viewer{SignedIn: true, IP: ipA})
	_, link := subscribe(t, h, doc, Viewer{ViaLink: true, IP: ipA})

	h.LinkClosed(doc.OwnerID, doc.DocumentID)
	assert.True(t, closed(link))
	assert.Len(t, lastPresence(t, owner), 1)
	assert.False(t, closed(owner))
}

func TestRunClosesEverythingWhenDone(t *testing.T) {
	h, _ := newHub(DefaultCaps)
	_, a := subscribe(t, h, doc, Viewer{IP: ipA})
	_, b := subscribe(t, h, other, Viewer{IP: ipA})

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error)
	go func() { done <- h.Run(ctx, time.Hour) }()
	cancel()
	require.NoError(t, <-done)

	last, ok := ended(a)
	assert.True(t, ok)
	assert.NotEqual(t, EventClosed, last, "shutdown is not a close")
	assert.True(t, closed(b))
	err := subscribeErr(h, doc, Viewer{IP: ipA})
	assert.ErrorIs(t, err, ErrStopped)
}

func TestGeneratedNameIsStable(t *testing.T) {
	name := generatedName("p_abc")
	assert.Equal(t, name, generatedName("p_abc"))
	assert.NotEqual(t, name, generatedName("p_abd"))
	assert.Regexp(t, `^[A-Z][a-z]+ [A-Z][a-z]+$`, generatedName("p_abc"))
}
