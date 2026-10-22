package httpapi

import (
	"net/http"

	"github.com/hay-kot/httpkit/errchain"

	"github.com/colonyops/hive/cmd/drive-server/internal/core"
	"github.com/colonyops/hive/internal/platform/driveapi"
)

func me(docs *core.Documents) errchain.HandlerFunc {
	return authed(http.StatusOK, func(_ http.ResponseWriter, r *http.Request, p core.Principal) (driveapi.Me, error) {
		used, err := docs.Usage(r.Context(), p)
		if err != nil {
			return driveapi.Me{}, err
		}
		resp := driveapi.Me{Email: p.Email, Policy: p.Policy.Name, Quota: driveapi.Quota{Used: used}}
		if !p.Policy.Quotas.Limit.Unlimited {
			limit := p.Policy.Quotas.Limit.Bytes
			resp.Quota.Limit = &limit
		}
		return resp, nil
	})
}
