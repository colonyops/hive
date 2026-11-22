package httpapi

import (
	"net/http"

	"github.com/hay-kot/httpkit/errchain"

	"github.com/colonyops/hive/cmd/drive-server/internal/core"
	"github.com/colonyops/hive/internal/platform/driveapi"
	"github.com/colonyops/hive/internal/web/extractors"
)

type meHandlers struct {
	docs     *core.Documents
	accounts *core.Accounts
}

func (h meHandlers) get(_ http.ResponseWriter, r *http.Request, p core.Principal) (driveapi.Me, error) {
	acct, err := h.accounts.Get(r.Context(), p)
	if err != nil {
		return driveapi.Me{}, err
	}
	return h.me(r, p, acct)
}

func (h meHandlers) update(w http.ResponseWriter, r *http.Request, p core.Principal) (driveapi.Me, error) {
	in, err := extractors.Body[driveapi.UpdateMe](w, r)
	if err != nil {
		return driveapi.Me{}, err
	}
	acct, err := h.accounts.SetDisplayName(r.Context(), p, in.DisplayName)
	if err != nil {
		return driveapi.Me{}, err
	}
	return h.me(r, p, acct)
}

func (h meHandlers) me(r *http.Request, p core.Principal, acct core.Account) (driveapi.Me, error) {
	used, err := h.docs.Usage(r.Context(), p)
	if err != nil {
		return driveapi.Me{}, err
	}
	resp := driveapi.Me{
		ID:          p.UserID,
		Email:       p.Email,
		DisplayName: acct.DisplayName,
		Policy:      p.Policy.Name,
		Quota:       driveapi.Quota{Used: used},
		Features:    []string{driveapi.FeatureComments},
	}
	if !p.Policy.Quotas.Limit.Unlimited {
		limit := p.Policy.Quotas.Limit.Bytes
		resp.Quota.Limit = &limit
	}
	return resp, nil
}

func registerMeRoutes(mux router, authenticated authenticatedFunc, sessionOnly func(errchain.HandlerFunc) http.HandlerFunc, h meHandlers) {
	mux.HandleFunc("GET /api/v1/me", authenticated(core.ScopeRead, authed(http.StatusOK, h.get)))
	mux.HandleFunc("PATCH /api/v1/me", sessionOnly(authed(http.StatusOK, h.update)))
}
