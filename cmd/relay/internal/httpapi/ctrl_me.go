package httpapi

import (
	"fmt"
	"net/http"

	"github.com/hay-kot/httpkit/server"

	"github.com/colonyops/hive/cmd/relay/internal/authctx"
	"github.com/colonyops/hive/cmd/relay/internal/wire"
)

func (s *api) me(w http.ResponseWriter, r *http.Request) error {
	acct := authctx.AccountFrom(r.Context())
	n, err := s.core.Store.Sources().CountByEmail(r.Context(), acct.Email)
	if err != nil {
		return fmt.Errorf("count sources: %w", err)
	}
	return server.JSON(w, http.StatusOK, wire.MeFrom(acct, n, s.core.Limits, s.core.SignIn.Methods(), s.core.Version))
}
