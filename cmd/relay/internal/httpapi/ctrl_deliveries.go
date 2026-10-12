package httpapi

import (
	"net/http"

	"github.com/hay-kot/httpkit/server"

	"github.com/colonyops/hive/cmd/relay/internal/authctx"
	"github.com/colonyops/hive/cmd/relay/internal/wire"
	"github.com/colonyops/hive/internal/web/extractors"
)

func (s *api) listDeliveries(w http.ResponseWriter, r *http.Request) error {
	q, err := extractors.Query[wire.DeliveriesQuery](r)
	if err != nil {
		return err
	}
	page, err := s.core.Feed.ListAfter(r.Context(), authctx.AccountFrom(r.Context()).Email, r.PathValue("id"), q.After, q.PageSize())
	if err != nil {
		return err
	}
	return server.JSON(w, http.StatusOK, page)
}
