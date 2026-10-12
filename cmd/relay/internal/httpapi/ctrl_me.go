package httpapi

import (
	"fmt"
	"net/http"

	"github.com/hay-kot/httpkit/server"

	"github.com/colonyops/hive/cmd/relay/internal/authctx"
	"github.com/colonyops/hive/cmd/relay/internal/relay"
	"github.com/colonyops/hive/cmd/relay/internal/wire"
	"github.com/colonyops/hive/internal/web/extractors"
)

func (s *api) me(w http.ResponseWriter, r *http.Request) error {
	return s.writeMe(w, r, authctx.AccountFrom(r.Context()))
}

func (s *api) updateMe(w http.ResponseWriter, r *http.Request) error {
	body, err := extractors.Body[wire.UpdateMe](w, r)
	if err != nil {
		return err
	}
	acct, err := s.core.Accounts.SetEncryption(r.Context(), authctx.AccountFrom(r.Context()).Email, body.On())
	if err != nil {
		return err
	}
	return s.writeMe(w, r, acct)
}

func (s *api) writeMe(w http.ResponseWriter, r *http.Request, acct relay.Account) error {
	n, err := s.core.Store.Sources().CountByEmail(r.Context(), acct.Email)
	if err != nil {
		return fmt.Errorf("count sources: %w", err)
	}
	return server.JSON(w, http.StatusOK, wire.MeFrom(acct, n, s.core.Limits, s.core.SignIn.Methods(), s.core.Version))
}
