package httpapi

import (
	"net/http"

	"github.com/hay-kot/httpkit/errchain"
	"github.com/hay-kot/httpkit/server"

	"github.com/colonyops/hive/cmd/drive-server/internal/core"
	"github.com/colonyops/hive/internal/platform/driveapi"
)

func me(docs *core.Documents) errchain.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		p, err := principal(r)
		if err != nil {
			return err
		}
		used, err := docs.Usage(r.Context(), p)
		if err != nil {
			return err
		}
		resp := driveapi.Me{Email: p.Email, Policy: p.Policy.Name, Quota: driveapi.Quota{Used: used}}
		if !p.Policy.Quotas.Limit.Unlimited {
			limit := p.Policy.Quotas.Limit.Bytes
			resp.Quota.Limit = &limit
		}
		return server.JSON(w, http.StatusOK, resp)
	}
}
