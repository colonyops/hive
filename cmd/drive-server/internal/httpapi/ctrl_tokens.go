package httpapi

import (
	"net/http"

	"github.com/hay-kot/httpkit/errchain"
	"github.com/hay-kot/httpkit/server"

	"github.com/colonyops/hive/cmd/drive-server/internal/core"
	"github.com/colonyops/hive/internal/platform/driveapi"
	"github.com/colonyops/hive/internal/web/extractors"
)

func registerTokenRoutes(mux *http.ServeMux, sessionOnly func(errchain.HandlerFunc) http.HandlerFunc, tokens *core.Tokens) {
	h := tokenHandlers{tokens}
	mux.HandleFunc("POST /api/v1/tokens", sessionOnly(h.create))
	mux.HandleFunc("GET /api/v1/tokens", sessionOnly(h.list))
	mux.HandleFunc("DELETE /api/v1/tokens/{id}", sessionOnly(h.revoke))
}

type tokenHandlers struct{ tokens *core.Tokens }

func (h tokenHandlers) create(w http.ResponseWriter, r *http.Request) error {
	p, ok := principalFrom(r.Context())
	if !ok {
		return core.NewError(core.KindUnauthenticated, "not signed in")
	}
	body, err := extractors.Body[driveapi.CreateToken](w, r)
	if err != nil {
		return err
	}
	scope, err := core.ParseScope(body.Scope)
	if err != nil {
		return core.NewError(core.KindInvalid, `scope must be "read" or "push"`)
	}
	secret, tok, err := h.tokens.MintFor(r.Context(), p.UserID, body.Name, scope, body.ExpiresIn)
	if err != nil {
		return err
	}
	return server.JSON(w, http.StatusCreated, driveapi.CreatedToken{Secret: secret, Token: tokenResponse(tok)})
}

func (h tokenHandlers) list(w http.ResponseWriter, r *http.Request) error {
	p, ok := principalFrom(r.Context())
	if !ok {
		return core.NewError(core.KindUnauthenticated, "not signed in")
	}
	toks, err := h.tokens.List(r.Context(), p.UserID)
	if err != nil {
		return err
	}
	out := make([]driveapi.Token, len(toks))
	for i, t := range toks {
		out[i] = tokenResponse(t)
	}
	return server.JSON(w, http.StatusOK, out)
}

func (h tokenHandlers) revoke(w http.ResponseWriter, r *http.Request) error {
	p, ok := principalFrom(r.Context())
	if !ok {
		return core.NewError(core.KindUnauthenticated, "not signed in")
	}
	if err := h.tokens.Revoke(r.Context(), p.UserID, r.PathValue("id")); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func tokenResponse(t core.Token) driveapi.Token {
	return driveapi.Token{
		ID:         t.ID,
		Name:       t.Name,
		Scope:      string(t.Scope),
		ExpiresAt:  t.ExpiresAt,
		LastUsedAt: t.LastUsedAt,
		CreatedAt:  t.CreatedAt,
	}
}
