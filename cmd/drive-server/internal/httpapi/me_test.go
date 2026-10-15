package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/drive-server/internal/app"
	"github.com/colonyops/hive/cmd/drive-server/internal/config"
	"github.com/colonyops/hive/cmd/drive-server/internal/core"
	"github.com/colonyops/hive/cmd/drive-server/internal/httpapi"
	"github.com/colonyops/hive/pkg/buildinfo"
)

func newMeServer(t *testing.T, policy config.Policy) (http.Handler, *app.Services) {
	t.Helper()
	svc, err := app.OpenServices(t.Context(), config.Config{DataDir: t.TempDir(), UserPolicies: []config.Policy{policy}})
	require.NoError(t, err)
	t.Cleanup(func() { _ = svc.Close() })
	return httpapi.New(zerolog.Nop(), buildinfo.Info{}, httpapi.Services{Tokens: svc.Tokens}), svc
}

func getMe(h http.Handler, authorization string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func devPolicy(limit config.Size) config.Policy {
	return config.Policy{Name: "dev", Matches: []string{"*@example.com"}, Quotas: config.Quotas{Limit: limit}}
}

func TestMeRequiresValidToken(t *testing.T) {
	h, _ := newMeServer(t, devPolicy(config.Size{Bytes: 1000}))

	for name, header := range map[string]string{
		"no header":    "",
		"wrong scheme": "Basic abc",
		"bad token":    "Bearer hds_nope",
		"not a token":  "Bearer nope",
	} {
		t.Run(name, func(t *testing.T) {
			rec := getMe(h, header)
			assert.Equal(t, http.StatusUnauthorized, rec.Code)
			var body map[string]string
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			assert.Equal(t, "unauthenticated", body["kind"])
		})
	}
}

func TestMeReturnsPrincipal(t *testing.T) {
	h, svc := newMeServer(t, devPolicy(config.Size{Bytes: 1000}))
	secret, _, err := svc.CreateToken(t.Context(), "dev@example.com", core.ScopeRead, "cli", 0)
	require.NoError(t, err)

	rec := getMe(h, "Bearer "+secret)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"email":"dev@example.com","policy":"dev","quota":{"used":0,"limit":1000}}`, rec.Body.String())
}

func TestMeUnlimitedQuotaIsNull(t *testing.T) {
	h, svc := newMeServer(t, devPolicy(config.Size{Unlimited: true}))
	secret, _, err := svc.CreateToken(t.Context(), "dev@example.com", core.ScopePush, "cli", 0)
	require.NoError(t, err)

	rec := getMe(h, "bearer "+secret)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"email":"dev@example.com","policy":"dev","quota":{"used":0,"limit":null}}`, rec.Body.String())
}

func TestMeRejectsLockedAccount(t *testing.T) {
	h, svc := newMeServer(t, devPolicy(config.Size{Bytes: 1000}))
	secret, _, err := svc.CreateToken(t.Context(), "dev@example.com", core.ScopeRead, "cli", 0)
	require.NoError(t, err)

	svc.Policies.Swap(nil)
	assert.Equal(t, http.StatusUnauthorized, getMe(h, "Bearer "+secret).Code)
}
