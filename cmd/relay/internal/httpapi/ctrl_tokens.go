package httpapi

import (
	"net/http"
	"strings"
	"time"

	"github.com/hay-kot/criterio"
	"github.com/hay-kot/httpkit/server"

	"github.com/colonyops/hive/cmd/relay/internal/relay"
	"github.com/colonyops/hive/internal/web/extractors"
)

const maxTokenName = 80

type tokenView struct {
	ID         string          `json:"id"`
	Kind       relay.TokenKind `json:"kind"`
	Name       string          `json:"name"`
	CreatedAt  time.Time       `json:"created_at"`
	LastUsedAt *time.Time      `json:"last_used_at"`
	// Current marks the token this request authenticated with, so a client
	// can tell before revoking itself.
	Current bool `json:"current"`
}

type tokensResponse struct {
	Tokens []tokenView `json:"tokens"`
}

type createTokenRequest struct {
	Name string `json:"name"`
}

func (b createTokenRequest) Validate() error {
	return criterio.Run("name", strings.TrimSpace(b.Name), criterio.StrBetween(1, maxTokenName))
}

type createdTokenResponse struct {
	Token     string          `json:"token"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Kind      relay.TokenKind `json:"kind"`
	CreatedAt time.Time       `json:"created_at"`
}

func (s *api) listTokens(w http.ResponseWriter, r *http.Request) error {
	toks, err := s.core.Auth.ListTokens(r.Context(), accountFrom(r.Context()).Email)
	if err != nil {
		return err
	}
	current := tokenFrom(r.Context()).ID
	out := make([]tokenView, 0, len(toks))
	for _, t := range toks {
		v := tokenView{ID: t.ID, Kind: t.Kind, Name: t.Device, CreatedAt: t.CreatedAt, Current: t.ID == current}
		if !t.LastUsedAt.IsZero() {
			v.LastUsedAt = &t.LastUsedAt
		}
		out = append(out, v)
	}
	return server.JSON(w, http.StatusOK, tokensResponse{Tokens: out})
}

func (s *api) createToken(w http.ResponseWriter, r *http.Request) error {
	body, err := extractors.Body[createTokenRequest](w, r)
	if err != nil {
		return err
	}
	plaintext, tok, err := s.core.Auth.MintToken(r.Context(), accountFrom(r.Context()).Email, body.Name, relay.TokenAPI)
	if err != nil {
		return err
	}
	return server.JSON(w, http.StatusCreated, createdTokenResponse{
		Token: plaintext, ID: tok.ID, Name: tok.Device, Kind: tok.Kind, CreatedAt: tok.CreatedAt,
	})
}

func (s *api) deleteToken(w http.ResponseWriter, r *http.Request) error {
	if err := s.core.Auth.RevokeToken(r.Context(), accountFrom(r.Context()).Email, r.PathValue("id")); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
