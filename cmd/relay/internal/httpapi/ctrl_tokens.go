package httpapi

import (
	"net/http"

	"github.com/hay-kot/httpkit/server"

	"github.com/colonyops/hive/cmd/relay/internal/authctx"
	"github.com/colonyops/hive/cmd/relay/internal/relay"
	"github.com/colonyops/hive/cmd/relay/internal/wire"
	"github.com/colonyops/hive/internal/web/extractors"
)

func (s *api) listTokens(w http.ResponseWriter, r *http.Request) error {
	toks, err := s.core.Auth.ListTokens(r.Context(), authctx.AccountFrom(r.Context()).Email)
	if err != nil {
		return err
	}
	return server.JSON(w, http.StatusOK, wire.TokensFrom(toks, authctx.TokenFrom(r.Context()).ID))
}

func (s *api) createToken(w http.ResponseWriter, r *http.Request) error {
	body, err := extractors.Body[wire.CreateToken](w, r)
	if err != nil {
		return err
	}
	plaintext, tok, err := s.core.Auth.MintToken(r.Context(), authctx.AccountFrom(r.Context()).Email, body.Name, relay.TokenAPI)
	if err != nil {
		return err
	}
	return server.JSON(w, http.StatusCreated, wire.CreatedTokenFrom(plaintext, tok))
}

func (s *api) deleteToken(w http.ResponseWriter, r *http.Request) error {
	if err := s.core.Auth.RevokeToken(r.Context(), authctx.AccountFrom(r.Context()).Email, r.PathValue("id")); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
