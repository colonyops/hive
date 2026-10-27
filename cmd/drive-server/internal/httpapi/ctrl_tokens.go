package httpapi

import (
	"net/http"

	"github.com/hay-kot/httpkit/errchain"

	"github.com/colonyops/hive/cmd/drive-server/internal/core"
	"github.com/colonyops/hive/internal/platform/driveapi"
	"github.com/colonyops/hive/internal/web/extractors"
)

func registerTokenRoutes(mux router, sessionOnly func(errchain.HandlerFunc) http.HandlerFunc, tokens *core.Tokens) {
	h := tokenHandlers{tokens}
	mux.HandleFunc("POST /api/v1/tokens", sessionOnly(authed(http.StatusCreated, h.create)))
	mux.HandleFunc("GET /api/v1/tokens", sessionOnly(authed(http.StatusOK, h.list)))
	mux.HandleFunc("DELETE /api/v1/tokens/{id}", sessionOnly(authedNoContent(h.revoke)))
}

type tokenHandlers struct{ tokens *core.Tokens }

func (h tokenHandlers) create(w http.ResponseWriter, r *http.Request, p core.Principal) (driveapi.CreatedToken, error) {
	body, err := extractors.Body[driveapi.CreateToken](w, r)
	if err != nil {
		return driveapi.CreatedToken{}, err
	}
	scope, err := core.ParseScope(body.Scope)
	if err != nil {
		return driveapi.CreatedToken{}, core.NewError(core.KindInvalid, `scope must be "read" or "push"`)
	}
	secret, tok, err := h.tokens.MintFor(r.Context(), p.UserID, body.Name, scope, body.ExpiresIn)
	if err != nil {
		return driveapi.CreatedToken{}, err
	}
	return driveapi.CreatedToken{Secret: secret, Token: tokenResponse(tok)}, nil
}

func (h tokenHandlers) list(_ http.ResponseWriter, r *http.Request, p core.Principal) ([]driveapi.Token, error) {
	toks, err := h.tokens.List(r.Context(), p.UserID)
	if err != nil {
		return nil, err
	}
	out := make([]driveapi.Token, len(toks))
	for i, t := range toks {
		out[i] = tokenResponse(t)
	}
	return out, nil
}

func (h tokenHandlers) revoke(r *http.Request, p core.Principal) error {
	return h.tokens.Revoke(r.Context(), p.UserID, r.PathValue("id"))
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
