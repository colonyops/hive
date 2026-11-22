package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"regexp"
	"time"

	"github.com/hay-kot/httpkit/errchain"
	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/drive-server/internal/core"
	"github.com/colonyops/hive/cmd/drive-server/internal/guard"
	"github.com/colonyops/hive/cmd/drive-server/internal/live"
	"github.com/colonyops/hive/internal/platform/driveapi"
	"github.com/colonyops/hive/internal/web/extractors"
)

const (
	keepaliveEvery = 25 * time.Second
	linkCheckEvery = 30 * time.Second
)

type liveHandlers struct {
	log          zerolog.Logger
	hub          *live.Hub
	comments     *core.Comments
	accounts     *core.Accounts
	trusted      []netip.Prefix
	opens        *guard.Limiter
	beatPerToken *guard.Limiter
	beatPerIP    *guard.Limiter
	linkCheck    time.Duration
}

func registerLiveRoutes(mux router, chain *errchain.ErrChain, authenticated authenticatedFunc, viewer viewerFunc, h liveHandlers) {
	h.opens = guard.NewLimiter(core.StreamOpensPerIP, core.CommentLimitWindow, limitKeys, limitIdle, nil)
	h.beatPerToken = guard.NewLimiter(core.PresencePerToken, core.CommentLimitWindow, limitKeys, limitIdle, nil)
	h.beatPerIP = guard.NewLimiter(core.PresencePerIP, core.CommentLimitWindow, limitKeys, limitIdle, nil)
	if h.linkCheck == 0 {
		h.linkCheck = linkCheckEvery
	}
	for _, sr := range spaceRoutes {
		mux.HandleFunc("GET "+sr.prefix+"/events/{path...}", authenticated(core.ScopeRead, h.ownerStream(sr.space)))
	}
	mux.HandleFunc("GET /api/v1/public/{slug}/events", chain.ToHandlerFunc(h.linkStream(viewer)))
	mux.HandleFunc("POST /api/v1/presence", chain.ToHandlerFunc(h.presence))
}

var streamPath = regexp.MustCompile(`^/api/v1/(?:public/[^/]+/events|(?:shared|workspaces/[^/]+|repos/[^/]+/[^/]+)/events/.+)$`)

// isQuiet keeps successful heartbeats and stream opens out of the request
// log: every open viewer makes them.
func isQuiet(r *http.Request) bool {
	path := r.URL.Path
	return path == "/healthz" || path == "/api/v1/presence" || (r.Method == http.MethodGet && streamPath.MatchString(path))
}

func (h liveHandlers) ownerStream(space spaceFunc) errchain.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		p, err := principal(r)
		if err != nil {
			return err
		}
		t, err := ownerTarget(r, space)
		if err != nil {
			return err
		}
		return h.stream(w, r, &p, t)
	}
}

func (h liveHandlers) linkStream(viewer viewerFunc) errchain.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		p, err := viewer(r)
		if err != nil {
			return err
		}
		w.Header().Set("X-Robots-Tag", "noindex")
		return h.stream(w, r, p, core.Target{Slug: r.PathValue("slug")})
	}
}

func (h liveHandlers) stream(w http.ResponseWriter, r *http.Request, p *core.Principal, t core.Target) error {
	ctx := r.Context()
	ip := guard.LimitKey(guard.ClientIP(r, h.trusted))
	if err := allow(w, h.opens, ip.String(), "too many streams opened; wait a minute"); err != nil {
		return err
	}
	doc, err := h.comments.Locate(ctx, p, t)
	if err != nil {
		return err
	}
	v := live.Viewer{ViaLink: t.Slug != "", IP: ip}
	if p != nil {
		acct, err := h.accounts.Get(ctx, *p)
		if err != nil {
			return err
		}
		v.Name, v.Color, v.SignedIn, v.AccountID = core.ShownName(acct), live.ColorFor(acct.ID), true, acct.ID
	}
	hello, events, cancel, err := h.hub.Subscribe(live.Key{OwnerID: doc.OwnerID, DocumentID: doc.DocumentID}, v)
	if err != nil {
		return err
	}
	defer cancel()
	// Read the version again now that the stream is subscribed: a push
	// between the first read and Subscribe sends no event to this stream.
	if doc, err = h.comments.Locate(ctx, p, t); err != nil {
		return err
	}
	hello.SHA256 = doc.SHA256

	// A stream outlives the server's read timeout; without this net/http
	// would cancel it when the deadline passes.
	rc := http.NewResponseController(w)
	if err := rc.SetReadDeadline(time.Time{}); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return fmt.Errorf("clear read deadline: %w", err)
	}
	// The server sets no write timeout, so a keep-alive connection would
	// carry the last stream write's deadline into its next request.
	defer func() { _ = rc.SetWriteDeadline(time.Time{}) }()
	hdr := w.Header()
	hdr.Set("Content-Type", "text/event-stream")
	hdr.Set("Cache-Control", "no-store")
	hdr.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	sse := sseWriter{w: w, rc: rc}
	if err := sse.event("hello", hello); err != nil {
		h.log.Debug().Err(err).Msg("stream write failed")
		return nil
	}

	keepalive := time.NewTicker(keepaliveEvery)
	defer keepalive.Stop()
	var check <-chan time.Time
	if t.Slug != "" {
		ticker := time.NewTicker(h.linkCheck)
		defer ticker.Stop()
		check = ticker.C
	}

	for {
		var err error
		select {
		case <-ctx.Done():
			return nil
		case e, ok := <-events:
			if !ok {
				return nil
			}
			err = sse.event(e.Name, e.Data)
			if e.Name == live.EventClosed {
				return nil
			}
		case <-keepalive.C:
			err = sse.comment("keepalive")
		case <-check:
			if h.linkDead(ctx, p, t) {
				_ = sse.event(live.EventClosed, struct{}{})
				return nil
			}
		}
		if err != nil {
			h.log.Debug().Err(err).Msg("stream write failed")
			return nil
		}
	}
}

// linkDead reports whether the link stopped working. A failure that is not a
// dead link, such as a busy database, keeps the stream open.
func (h liveHandlers) linkDead(ctx context.Context, p *core.Principal, t core.Target) bool {
	_, err := h.comments.Locate(ctx, p, t)
	if err != nil && !core.IsKind(err, core.KindNotFound) {
		h.log.Warn().Err(err).Msg("link stream: check link")
		return false
	}
	return err != nil
}

func (h liveHandlers) presence(w http.ResponseWriter, r *http.Request) error {
	ip := guard.LimitKey(guard.ClientIP(r, h.trusted))
	if err := allow(w, h.beatPerIP, ip.String(), "too many presence updates"); err != nil {
		return err
	}
	in, err := extractors.Body[driveapi.PresenceUpdate](w, r)
	if err != nil {
		return err
	}
	if in.Token == "" {
		return core.NewError(core.KindInvalid, "token is required")
	}
	if err := allow(w, h.beatPerToken, in.Token, "too many presence updates"); err != nil {
		return err
	}
	if err := h.hub.Heartbeat(in.Token, in.Name, in.Color, in.Selection); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// streamWriteTimeout bounds one stream write. Without it a client that
// stops reading blocks the handler in Write, where it never sees the context
// end or the hub close its stream.
const streamWriteTimeout = 10 * time.Second

type sseWriter struct {
	w  http.ResponseWriter
	rc *http.ResponseController
}

func (s sseWriter) write(format string, args ...any) error {
	if err := s.rc.SetWriteDeadline(time.Now().Add(streamWriteTimeout)); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return fmt.Errorf("set write deadline: %w", err)
	}
	if _, err := fmt.Fprintf(s.w, format, args...); err != nil {
		return err
	}
	return s.rc.Flush()
}

func (s sseWriter) event(name string, data any) error {
	b, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("encode %s event: %w", name, err)
	}
	return s.write("event: %s\ndata: %s\n\n", name, b)
}

func (s sseWriter) comment(text string) error {
	return s.write(": %s\n\n", text)
}
