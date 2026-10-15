package app_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/drive-server/internal/app"
	"github.com/colonyops/hive/cmd/drive-server/internal/config"
	"github.com/colonyops/hive/cmd/drive-server/internal/core"
)

func openServices(t *testing.T, policies ...config.Policy) *app.Services {
	t.Helper()
	svc, err := app.OpenServices(t.Context(), zerolog.Nop(), config.Config{DataDir: t.TempDir(), UserPolicies: policies})
	require.NoError(t, err)
	t.Cleanup(func() { _ = svc.Close() })
	return svc
}

func devPolicy() config.Policy {
	return config.Policy{Name: "dev", Matches: []string{"*@example.com"}}
}

func TestParseTTL(t *testing.T) {
	for in, want := range map[string]time.Duration{
		"":     0,
		"30d":  30 * 24 * time.Hour,
		"720h": 720 * time.Hour,
	} {
		got, err := app.ParseTTL(in)
		require.NoError(t, err, in)
		assert.Equal(t, want, got, in)
	}
	for _, bad := range []string{"0d", "-1d", "xd", "soon", "-5h"} {
		_, err := app.ParseTTL(bad)
		assert.Error(t, err, bad)
	}
}

func TestCreateTokenCreatesAccountAndAuthenticates(t *testing.T) {
	svc := openServices(t, devPolicy())

	secret, tok, err := svc.CreateToken(t.Context(), "Dev@example.com", core.ScopePush, "cli", 24*time.Hour)
	require.NoError(t, err)
	require.NotNil(t, tok.ExpiresAt)

	p, err := svc.Tokens.Resolve(t.Context(), secret)
	require.NoError(t, err)
	assert.Equal(t, "dev@example.com", p.Email)
}

func TestCreateTokenNeedsPolicy(t *testing.T) {
	svc := openServices(t, devPolicy())

	_, _, err := svc.CreateToken(t.Context(), "dev@other.example", core.ScopeRead, "cli", 0)
	var coreErr *core.Error
	require.ErrorAs(t, err, &coreErr)
	assert.Equal(t, core.KindForbidden, coreErr.Kind)
}

func TestWriteAccountsShowsLocked(t *testing.T) {
	svc := openServices(t, devPolicy())
	_, _, err := svc.CreateToken(t.Context(), "dev@example.com", core.ScopeRead, "cli", 0)
	require.NoError(t, err)

	var out bytes.Buffer
	require.NoError(t, svc.WriteAccounts(t.Context(), &out))
	assert.Regexp(t, `dev@example.com\s+\S+\s+\d{4}-\d{2}-\d{2}\s+dev`, out.String())

	svc.Policies.Swap(nil)
	out.Reset()
	require.NoError(t, svc.WriteAccounts(t.Context(), &out))
	assert.Contains(t, out.String(), "locked")
}
