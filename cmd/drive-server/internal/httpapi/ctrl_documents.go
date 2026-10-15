package httpapi

import (
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/hay-kot/httpkit/errchain"
	"github.com/hay-kot/httpkit/server"

	"github.com/colonyops/hive/cmd/drive-server/internal/core"
	"github.com/colonyops/hive/internal/domain/drive"
)

type authenticatedFunc func(scope core.Scope, h errchain.HandlerFunc) http.HandlerFunc

// spaceRoute is one route prefix and how it names a space. Space keys hold a
// slash, so each kind gets a fixed number of path segments.
type spaceRoute struct {
	prefix string
	space  spaceFunc
}

var spaceRoutes = []spaceRoute{
	{"/api/v1/shared", func(*http.Request) (drive.Space, error) { return drive.Shared, nil }},
	{"/api/v1/workspaces/{name}", func(r *http.Request) (drive.Space, error) {
		return drive.WorkspaceSpace(r.PathValue("name"))
	}},
	{"/api/v1/repos/{owner}/{repo}", func(r *http.Request) (drive.Space, error) {
		return drive.RepositorySpace(r.PathValue("owner"), r.PathValue("repo"))
	}},
}

func registerDocumentRoutes(mux *http.ServeMux, authenticated authenticatedFunc, docs *core.Documents) {
	h := documentHandlers{docs}
	mux.HandleFunc("GET /api/v1/spaces", authenticated(core.ScopeRead, h.spaces))
	for _, sr := range spaceRoutes {
		doc := sr.prefix + "/docs/{path...}"
		mux.HandleFunc("GET "+sr.prefix, authenticated(core.ScopeRead, h.list(sr.space)))
		mux.HandleFunc("GET "+doc, authenticated(core.ScopeRead, h.get(sr.space)))
		mux.HandleFunc("PUT "+doc, authenticated(core.ScopePush, h.put(sr.space)))
		mux.HandleFunc("DELETE "+doc, authenticated(core.ScopePush, h.delete(sr.space)))
	}
}

type documentHandlers struct{ docs *core.Documents }

type spaceFunc func(r *http.Request) (drive.Space, error)

// resolve returns the caller and the requested space. A name that is not a
// valid space is a 404, not a 422: no such space can exist.
func resolve(r *http.Request, space spaceFunc) (core.Principal, drive.Space, error) {
	p, ok := principalFrom(r.Context())
	if !ok {
		return core.Principal{}, drive.Space{}, core.NewError(core.KindUnauthenticated, "not signed in")
	}
	s, err := space(r)
	if err != nil {
		return core.Principal{}, drive.Space{}, core.NewError(core.KindNotFound, "space not found")
	}
	return p, s, nil
}

func (h documentHandlers) spaces(w http.ResponseWriter, r *http.Request) error {
	p, ok := principalFrom(r.Context())
	if !ok {
		return core.NewError(core.KindUnauthenticated, "not signed in")
	}
	spaces, err := h.docs.Spaces(r.Context(), p)
	if err != nil {
		return err
	}
	return server.JSON(w, http.StatusOK, spaces)
}

func (h documentHandlers) list(space spaceFunc) errchain.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		p, s, err := resolve(r, space)
		if err != nil {
			return err
		}
		docs, err := h.docs.List(r.Context(), p, s)
		if err != nil {
			return err
		}
		return server.JSON(w, http.StatusOK, docs)
	}
}

func (h documentHandlers) get(space spaceFunc) errchain.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		p, s, err := resolve(r, space)
		if err != nil {
			return err
		}
		doc, body, err := h.docs.Get(r.Context(), p, s, r.PathValue("path"))
		if err != nil {
			return err
		}
		contentType := "text/markdown; charset=utf-8"
		if doc.Kind == drive.KindHtml {
			contentType = "text/html; charset=utf-8"
		}
		hdr := w.Header()
		hdr.Set("Content-Type", contentType)
		hdr.Set("ETag", `"`+doc.SHA256+`"`)
		hdr.Set("Content-Length", strconv.Itoa(len(body)))
		// A stored HTML document must not run in this origin if a browser
		// navigates here.
		hdr.Set("X-Content-Type-Options", "nosniff")
		hdr.Set("Content-Security-Policy", "sandbox")
		_, err = w.Write(body)
		return err
	}
}

func (h documentHandlers) put(space spaceFunc) errchain.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		p, s, err := resolve(r, space)
		if err != nil {
			return err
		}
		body := r.Body
		if limit := h.docs.MaxDocumentSize(); limit > 0 {
			body = http.MaxBytesReader(w, r.Body, limit+1)
		}
		data, err := io.ReadAll(body)
		if err != nil {
			if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
				return err
			}
			return core.NewError(core.KindInvalid, "could not read the request body")
		}
		doc, err := h.docs.Put(r.Context(), p, s, r.PathValue("path"), data)
		if err != nil {
			return err
		}
		return server.JSON(w, http.StatusOK, doc)
	}
}

func (h documentHandlers) delete(space spaceFunc) errchain.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		p, s, err := resolve(r, space)
		if err != nil {
			return err
		}
		if err := h.docs.Delete(r.Context(), p, s, r.PathValue("path")); err != nil {
			return err
		}
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
}
