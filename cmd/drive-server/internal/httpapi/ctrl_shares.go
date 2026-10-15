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
	"github.com/colonyops/hive/internal/platform/driveapi"
	"github.com/colonyops/hive/internal/web/extractors"
)

// rawCSP lets a shared HTML page run its own inline script and style and
// nothing else: no network, no forms, no framing by other origins.
const rawCSP = "default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; img-src data:; font-src data:; connect-src 'none'; form-action 'none'; frame-src 'none'; base-uri 'none'; frame-ancestors 'self'; sandbox allow-scripts"

func registerShareRoutes(mux *http.ServeMux, chain *errchain.ErrChain, authenticated authenticatedFunc, shares *core.Shares) {
	h := shareHandlers{shares}
	for _, sr := range spaceRoutes {
		route := sr.prefix + "/shares/{path...}"
		mux.HandleFunc("POST "+route, authenticated(core.ScopePush, h.create(sr.space)))
		mux.HandleFunc("DELETE "+route, authenticated(core.ScopePush, h.revoke(sr.space)))
	}
	mux.HandleFunc("GET /s/{slug}", chain.ToHandlerFunc(h.redirect))
	mux.HandleFunc("GET /s/{slug}/raw", chain.ToHandlerFunc(h.raw))
	mux.HandleFunc("GET /api/v1/public/{slug}", chain.ToHandlerFunc(h.public))
}

type shareHandlers struct{ shares *core.Shares }

func (h shareHandlers) create(space spaceFunc) errchain.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		p, s, err := resolve(r, space)
		if err != nil {
			return err
		}
		req, err := extractors.Body[driveapi.ShareRequest](w, r)
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		share, err := h.shares.Create(r.Context(), p, s, r.PathValue("path"), req.ExpiresIn)
		if err != nil {
			return err
		}
		return server.JSON(w, http.StatusOK, share)
	}
}

func (h shareHandlers) revoke(space spaceFunc) errchain.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		p, s, err := resolve(r, space)
		if err != nil {
			return err
		}
		if err := h.shares.Revoke(r.Context(), p, s, r.PathValue("path")); err != nil {
			return err
		}
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
}

func (h shareHandlers) redirect(w http.ResponseWriter, r *http.Request) error {
	http.Redirect(w, r, "/s/"+r.PathValue("slug")+"/raw", http.StatusFound)
	return nil
}

func (h shareHandlers) raw(w http.ResponseWriter, r *http.Request) error {
	w.Header().Set("X-Robots-Tag", "noindex")
	_, doc, body, err := h.shares.Resolve(r.Context(), r.PathValue("slug"))
	if err != nil {
		return err
	}
	return writeRaw(w, doc, body)
}

func (h shareHandlers) public(w http.ResponseWriter, r *http.Request) error {
	w.Header().Set("X-Robots-Tag", "noindex")
	w.Header().Set("Cache-Control", "private, no-store")
	owner, doc, body, err := h.shares.Resolve(r.Context(), r.PathValue("slug"))
	if err != nil {
		return err
	}
	out := driveapi.PublicDocument{OwnerEmail: owner.Email, Document: doc}
	if doc.Kind == drive.KindMarkdown {
		out.Body = string(body)
	}
	return server.JSON(w, http.StatusOK, out)
}

func contentType(k drive.Kind) string {
	if k == drive.KindHtml {
		return "text/html; charset=utf-8"
	}
	return "text/markdown; charset=utf-8"
}

// writeRaw sends a stored body under the raw rules: a browser that renders
// it gets no access to the server's origin or the network.
func writeRaw(w http.ResponseWriter, doc driveapi.Document, body []byte) error {
	hdr := w.Header()
	hdr.Set("Content-Type", contentType(doc.Kind))
	hdr.Set("Content-Length", strconv.Itoa(len(body)))
	hdr.Set("X-Content-Type-Options", "nosniff")
	hdr.Set("X-Robots-Tag", "noindex")
	hdr.Set("Referrer-Policy", "no-referrer")
	hdr.Set("Cache-Control", "private, no-store")
	hdr.Set("Content-Security-Policy", rawCSP)
	_, err := w.Write(body)
	return err
}
