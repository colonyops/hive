package httpapi

import (
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/hay-kot/httpkit/errchain"

	"github.com/colonyops/hive/cmd/drive-server/internal/core"
	"github.com/colonyops/hive/internal/domain/drive"
	"github.com/colonyops/hive/internal/platform/driveapi"
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

func registerDocumentRoutes(mux router, authenticated authenticatedFunc, docs *core.Documents) {
	h := documentHandlers{docs}
	mux.HandleFunc("GET /api/v1/spaces", authenticated(core.ScopeRead, authed(http.StatusOK, h.spaces)))
	mux.HandleFunc("GET /api/v1/search", authenticated(core.ScopeRead, authed(http.StatusOK, h.search)))
	mux.HandleFunc("GET /api/v1/recent", authenticated(core.ScopeRead, authed(http.StatusOK, h.recent)))
	mux.HandleFunc("GET /api/v1/shared-links", authenticated(core.ScopeRead, authed(http.StatusOK, h.sharedLinks)))
	for _, sr := range spaceRoutes {
		doc := sr.prefix + "/docs/{path...}"
		mux.HandleFunc("GET "+sr.prefix, authenticated(core.ScopeRead, authed(http.StatusOK, h.list(sr.space))))
		mux.HandleFunc("GET "+doc, authenticated(core.ScopeRead, h.get(sr.space)))
		mux.HandleFunc("PUT "+doc, authenticated(core.ScopePush, authed(http.StatusOK, h.put(sr.space))))
		mux.HandleFunc("DELETE "+doc, authenticated(core.ScopePush, authedNoContent(h.delete(sr.space))))
	}
}

type documentHandlers struct{ docs *core.Documents }

type spaceFunc func(r *http.Request) (drive.Space, error)

func (h documentHandlers) spaces(_ http.ResponseWriter, r *http.Request, p core.Principal) ([]driveapi.SpaceSummary, error) {
	return h.docs.Spaces(r.Context(), p)
}

const (
	defaultSearchLimit = 50
	maxSearchLimit     = 200
)

func parseLimit(raw string, def, maxLimit int) (int, error) {
	if raw == "" {
		return def, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return 0, core.NewError(core.KindInvalid, "limit must be a positive number")
	}
	return min(n, maxLimit), nil
}

func (h documentHandlers) search(_ http.ResponseWriter, r *http.Request, p core.Principal) (driveapi.SearchResults, error) {
	query := r.URL.Query()

	var space *drive.Space
	if key := query.Get("space"); key != "" {
		s, err := drive.ParseSpaceKey(key)
		if err != nil {
			return driveapi.SearchResults{}, core.NewError(core.KindInvalid, "invalid space")
		}
		space = &s
	}
	limit, err := parseLimit(query.Get("limit"), defaultSearchLimit, maxSearchLimit)
	if err != nil {
		return driveapi.SearchResults{}, err
	}

	hits, err := h.docs.Search(r.Context(), p, query.Get("q"), space, limit)
	if err != nil {
		return driveapi.SearchResults{}, err
	}
	return driveapi.SearchResults{Hits: hits}, nil
}

func (h documentHandlers) recent(_ http.ResponseWriter, r *http.Request, p core.Principal) ([]driveapi.Document, error) {
	limit, err := parseLimit(r.URL.Query().Get("limit"), core.DefaultRecentLimit, core.MaxRecentLimit)
	if err != nil {
		return nil, err
	}
	return h.docs.Recent(r.Context(), p, limit)
}

func (h documentHandlers) sharedLinks(_ http.ResponseWriter, r *http.Request, p core.Principal) ([]driveapi.Document, error) {
	return h.docs.SharedLinks(r.Context(), p)
}

func (h documentHandlers) list(space spaceFunc) func(http.ResponseWriter, *http.Request, core.Principal) ([]driveapi.Document, error) {
	return func(_ http.ResponseWriter, r *http.Request, p core.Principal) ([]driveapi.Document, error) {
		s, err := spaceOf(r, space)
		if err != nil {
			return nil, err
		}
		return h.docs.List(r.Context(), p, s)
	}
}

func (h documentHandlers) get(space spaceFunc) errchain.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		p, err := principal(r)
		if err != nil {
			return err
		}
		s, err := spaceOf(r, space)
		if err != nil {
			return err
		}
		doc, body, err := h.docs.Get(r.Context(), p, s, r.PathValue("path"))
		if err != nil {
			return err
		}
		if r.URL.Query().Get("raw") == "1" {
			return writeRaw(w, r, doc, body)
		}
		hdr := w.Header()
		hdr.Set("Content-Type", contentType(doc.Kind))
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

func (h documentHandlers) put(space spaceFunc) func(http.ResponseWriter, *http.Request, core.Principal) (driveapi.Document, error) {
	return func(w http.ResponseWriter, r *http.Request, p core.Principal) (driveapi.Document, error) {
		s, err := spaceOf(r, space)
		if err != nil {
			return driveapi.Document{}, err
		}
		body := r.Body
		if limit := h.docs.MaxDocumentSize(); limit > 0 {
			body = http.MaxBytesReader(w, r.Body, limit+1)
		}
		data, err := io.ReadAll(body)
		if err != nil {
			if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
				return driveapi.Document{}, err
			}
			return driveapi.Document{}, core.NewError(core.KindInvalid, "could not read the request body")
		}
		return h.docs.Put(r.Context(), p, s, r.PathValue("path"), data)
	}
}

func (h documentHandlers) delete(space spaceFunc) func(*http.Request, core.Principal) error {
	return func(r *http.Request, p core.Principal) error {
		s, err := spaceOf(r, space)
		if err != nil {
			return err
		}
		return h.docs.Delete(r.Context(), p, s, r.PathValue("path"))
	}
}
