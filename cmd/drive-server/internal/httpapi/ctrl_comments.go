package httpapi

import (
	"net/http"
	"time"

	"github.com/hay-kot/httpkit/errchain"
	"github.com/hay-kot/httpkit/server"

	"github.com/colonyops/hive/cmd/drive-server/internal/core"
	"github.com/colonyops/hive/cmd/drive-server/internal/guard"
	"github.com/colonyops/hive/internal/platform/driveapi"
	"github.com/colonyops/hive/internal/web/extractors"
)

const (
	limitKeys = 10_000
	limitIdle = 10 * time.Minute
)

// viewerFunc resolves the caller of a route that also serves anonymous
// readers: nil with no error for an anonymous one.
type viewerFunc func(r *http.Request) (*core.Principal, error)

type principalFn[T any] func(w http.ResponseWriter, r *http.Request, p core.Principal) (T, error)

type commentHandlers struct {
	comments *core.Comments
	writes   *guard.Limiter
}

func registerCommentRoutes(mux router, chain *errchain.ErrChain, authenticated authenticatedFunc, viewer viewerFunc, comments *core.Comments) {
	h := commentHandlers{
		comments: comments,
		writes:   guard.NewLimiter(core.CommentWritesPerUser, core.CommentLimitWindow, limitKeys, limitIdle, nil),
	}
	push := func(fn errchain.HandlerFunc) http.HandlerFunc { return authenticated(core.ScopePush, fn) }

	for _, sr := range spaceRoutes {
		route := sr.prefix + "/threads/{path...}"
		mux.HandleFunc("GET "+route, authenticated(core.ScopeRead, authed(http.StatusOK, h.ownerList(sr.space))))
		mux.HandleFunc("POST "+route, push(authed(http.StatusCreated, limited(h, h.ownerCreate(sr.space)))))
	}
	mux.HandleFunc("POST /api/v1/threads/{id}/comments", push(authed(http.StatusCreated, limited(h, h.reply))))
	mux.HandleFunc("PATCH /api/v1/threads/{id}", push(authed(http.StatusOK, limited(h, h.setResolved))))
	mux.HandleFunc("PATCH /api/v1/comments/{id}", push(authed(http.StatusOK, limited(h, h.edit))))
	mux.HandleFunc("DELETE /api/v1/comments/{id}", push(h.delete))

	mux.HandleFunc("GET /api/v1/public/{slug}/threads", chain.ToHandlerFunc(h.linkList(viewer)))
	mux.HandleFunc("POST /api/v1/links/{slug}/threads", push(authed(http.StatusCreated, limited(h, h.linkCreate))))
	mux.HandleFunc("POST /api/v1/links/{slug}/threads/{id}/comments", push(authed(http.StatusCreated, limited(h, h.reply))))
	mux.HandleFunc("PATCH /api/v1/links/{slug}/threads/{id}", push(authed(http.StatusOK, limited(h, h.setResolved))))
	mux.HandleFunc("PATCH /api/v1/links/{slug}/comments/{id}", push(authed(http.StatusOK, limited(h, h.edit))))
	mux.HandleFunc("DELETE /api/v1/links/{slug}/comments/{id}", push(h.delete))
}

func (h commentHandlers) allow(w http.ResponseWriter, p core.Principal) error {
	return allow(w, h.writes, p.UserID, "too many comment changes; wait a minute")
}

func limited[T any](h commentHandlers, fn principalFn[T]) principalFn[T] {
	return func(w http.ResponseWriter, r *http.Request, p core.Principal) (T, error) {
		if err := h.allow(w, p); err != nil {
			var zero T
			return zero, err
		}
		return fn(w, r, p)
	}
}

func (h commentHandlers) ownerList(space spaceFunc) principalFn[driveapi.ThreadList] {
	return func(_ http.ResponseWriter, r *http.Request, p core.Principal) (driveapi.ThreadList, error) {
		t, err := ownerTarget(r, space)
		if err != nil {
			return driveapi.ThreadList{}, err
		}
		return h.comments.List(r.Context(), &p, t)
	}
}

func (h commentHandlers) linkList(viewer viewerFunc) errchain.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		p, err := viewer(r)
		if err != nil {
			return err
		}
		w.Header().Set("Cache-Control", "private, no-store")
		w.Header().Set("X-Robots-Tag", "noindex")
		list, err := h.comments.List(r.Context(), p, core.Target{Slug: r.PathValue("slug")})
		if err != nil {
			return err
		}
		return server.JSON(w, http.StatusOK, list)
	}
}

func (h commentHandlers) ownerCreate(space spaceFunc) principalFn[driveapi.Thread] {
	return func(w http.ResponseWriter, r *http.Request, p core.Principal) (driveapi.Thread, error) {
		t, err := ownerTarget(r, space)
		if err != nil {
			return driveapi.Thread{}, err
		}
		in, err := extractors.Body[driveapi.CreateThread](w, r)
		if err != nil {
			return driveapi.Thread{}, err
		}
		return h.comments.CreateThread(r.Context(), p, t, in)
	}
}

func (h commentHandlers) linkCreate(w http.ResponseWriter, r *http.Request, p core.Principal) (driveapi.Thread, error) {
	in, err := extractors.Body[driveapi.CreateThread](w, r)
	if err != nil {
		return driveapi.Thread{}, err
	}
	return h.comments.CreateThread(r.Context(), p, core.Target{Slug: r.PathValue("slug")}, in)
}

// The id-addressed handlers serve both the owner routes and the link routes:
// on an owner route {slug} is empty, which means the caller's own file.

func (h commentHandlers) reply(w http.ResponseWriter, r *http.Request, p core.Principal) (driveapi.Comment, error) {
	in, err := extractors.Body[driveapi.CommentBody](w, r)
	if err != nil {
		return driveapi.Comment{}, err
	}
	return h.comments.Reply(r.Context(), p, r.PathValue("slug"), r.PathValue("id"), in.Body)
}

func (h commentHandlers) setResolved(w http.ResponseWriter, r *http.Request, p core.Principal) (driveapi.Thread, error) {
	in, err := extractors.Body[driveapi.ThreadStateUpdate](w, r)
	if err != nil {
		return driveapi.Thread{}, err
	}
	return h.comments.SetResolved(r.Context(), p, r.PathValue("slug"), r.PathValue("id"), in.Resolved)
}

func (h commentHandlers) edit(w http.ResponseWriter, r *http.Request, p core.Principal) (driveapi.Comment, error) {
	in, err := extractors.Body[driveapi.CommentBody](w, r)
	if err != nil {
		return driveapi.Comment{}, err
	}
	return h.comments.EditComment(r.Context(), p, r.PathValue("slug"), r.PathValue("id"), in.Body)
}

func (h commentHandlers) delete(w http.ResponseWriter, r *http.Request) error {
	p, err := principal(r)
	if err != nil {
		return err
	}
	if err := h.allow(w, p); err != nil {
		return err
	}
	if err := h.comments.DeleteComment(r.Context(), p, r.PathValue("slug"), r.PathValue("id")); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func ownerTarget(r *http.Request, space spaceFunc) (core.Target, error) {
	s, err := spaceOf(r, space)
	if err != nil {
		return core.Target{}, err
	}
	return core.Target{Space: &s, Path: r.PathValue("path")}, nil
}
