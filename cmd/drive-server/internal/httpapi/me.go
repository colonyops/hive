package httpapi

import (
	"net/http"

	"github.com/hay-kot/httpkit/server"

	"github.com/colonyops/hive/cmd/drive-server/internal/core"
)

type meResponse struct {
	Email  string `json:"email"`
	Policy string `json:"policy"`
	Quota  quota  `json:"quota"`
}

// quota.Limit is null for an unlimited policy.
type quota struct {
	Used  int64  `json:"used"`
	Limit *int64 `json:"limit"`
}

func me(w http.ResponseWriter, r *http.Request) error {
	p, ok := principalFrom(r.Context())
	if !ok {
		return core.NewError(core.KindUnauthenticated, "not signed in")
	}
	resp := meResponse{Email: p.Email, Policy: p.Policy.Name}
	if !p.Policy.Quotas.Limit.Unlimited {
		limit := p.Policy.Quotas.Limit.Bytes
		resp.Quota.Limit = &limit
	}
	return server.JSON(w, http.StatusOK, resp)
}
