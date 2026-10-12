package httpapi

import (
	"net/http"

	"github.com/hay-kot/criterio"
	"github.com/hay-kot/httpkit/server"

	"github.com/colonyops/hive/internal/web/extractors"
)

const (
	defaultPageSize = 200
	maxPageSize     = 500
)

type deliveriesQuery struct {
	After string `query:"after"`
	// Limit of 0 takes defaultPageSize.
	Limit int `query:"limit"`
}

func (q deliveriesQuery) Validate() error {
	return criterio.ValidateStruct(
		criterio.Run("after", q.After, criterio.When(q.After != "", criterio.StrUUID)),
		criterio.Run("limit", q.Limit, criterio.When(q.Limit != 0, criterio.Between(1, maxPageSize))),
	)
}

func (s *api) listDeliveries(w http.ResponseWriter, r *http.Request) error {
	q, err := extractors.Query[deliveriesQuery](r)
	if err != nil {
		return err
	}
	limit := q.Limit
	if limit == 0 {
		limit = defaultPageSize
	}
	page, err := s.core.Feed.ListAfter(r.Context(), accountFrom(r.Context()).Email, r.PathValue("id"), q.After, limit)
	if err != nil {
		return err
	}
	return server.JSON(w, http.StatusOK, page)
}
