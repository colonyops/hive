package core_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/drive-server/internal/config"
	"github.com/colonyops/hive/cmd/drive-server/internal/core"
)

func devPolicy() config.Policy {
	return config.Policy{Name: "dev", Matches: []string{"*@example.com"}, Quotas: config.Quotas{Limit: config.Size{Bytes: 1 << 30}}}
}

func mint(t *testing.T, e *env, email string, scope core.Scope, expiresAt *time.Time) (string, core.Account) {
	t.Helper()
	acct, err := e.accounts.Ensure(t.Context(), email)
	require.NoError(t, err)
	secret, _, err := e.tokens.Mint(t.Context(), acct.ID, "cli", scope, expiresAt)
	require.NoError(t, err)
	return secret, acct
}

func requireKind(t *testing.T, err error, kind core.Kind) {
	t.Helper()
	var coreErr *core.Error
	require.ErrorAs(t, err, &coreErr)
	assert.Equal(t, kind, coreErr.Kind)
}

func TestTokenSecretIsNotStored(t *testing.T) {
	e := newEnv(t, devPolicy())
	secret, _ := mint(t, e, "dev@example.com", core.ScopePush, nil)
	require.True(t, strings.HasPrefix(secret, "hds_"))

	var stored string
	require.NoError(t, e.rawDB(t).QueryRowContext(t.Context(), `SELECT secret_hash FROM api_tokens`).Scan(&stored))
	assert.NotEqual(t, secret, stored)
	assert.NotContains(t, stored, strings.TrimPrefix(secret, "hds_"))
	assert.Len(t, stored, 64)

	p, err := e.tokens.Resolve(t.Context(), secret)
	require.NoError(t, err)
	assert.Equal(t, "dev@example.com", p.Email)
	assert.Equal(t, "dev", p.Policy.Name)
	assert.False(t, p.ViaSession)
}

func TestTokenScopeRoundTrip(t *testing.T) {
	e := newEnv(t, devPolicy())
	readSecret, _ := mint(t, e, "dev@example.com", core.ScopeRead, nil)
	pushSecret, _ := mint(t, e, "dev@example.com", core.ScopePush, nil)

	read, err := e.tokens.Resolve(t.Context(), readSecret)
	require.NoError(t, err)
	assert.Equal(t, core.ScopeRead, read.Scope)

	push, err := e.tokens.Resolve(t.Context(), pushSecret)
	require.NoError(t, err)
	assert.Equal(t, core.ScopePush, push.Scope)
	assert.Equal(t, read.UserID, push.UserID)
}

func TestTokenRejectsUnknownSecrets(t *testing.T) {
	e := newEnv(t, devPolicy())
	secret, _ := mint(t, e, "dev@example.com", core.ScopeRead, nil)

	for _, bad := range []string{"", "nope", "hds_nope", secret + "x"} {
		_, err := e.tokens.Resolve(t.Context(), bad)
		requireKind(t, err, core.KindUnauthenticated)
	}
}

func TestTokenExpiry(t *testing.T) {
	e := newEnv(t, devPolicy())
	expires := e.clock.Now().Add(time.Hour)
	secret, _ := mint(t, e, "dev@example.com", core.ScopeRead, &expires)

	_, err := e.tokens.Resolve(t.Context(), secret)
	require.NoError(t, err)

	e.clock.Advance(time.Hour)
	_, err = e.tokens.Resolve(t.Context(), secret)
	requireKind(t, err, core.KindUnauthenticated)
}

func TestTokenMintValidatesInput(t *testing.T) {
	e := newEnv(t, devPolicy())
	acct, err := e.accounts.Ensure(t.Context(), "dev@example.com")
	require.NoError(t, err)

	past := e.clock.Now().Add(-time.Minute)
	for name, call := range map[string]func() error{
		"empty name": func() error { _, _, err := e.tokens.Mint(t.Context(), acct.ID, " ", core.ScopeRead, nil); return err },
		"bad scope": func() error {
			_, _, err := e.tokens.Mint(t.Context(), acct.ID, "cli", core.Scope("admin"), nil)
			return err
		},
		"past expiry": func() error {
			_, _, err := e.tokens.Mint(t.Context(), acct.ID, "cli", core.ScopeRead, &past)
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			requireKind(t, call(), core.KindInvalid)
		})
	}
}

func TestTokenRevoke(t *testing.T) {
	e := newEnv(t, devPolicy())
	secret, acct := mint(t, e, "dev@example.com", core.ScopeRead, nil)

	toks, err := e.tokens.List(t.Context(), acct.ID)
	require.NoError(t, err)
	require.Len(t, toks, 1)

	requireKind(t, e.tokens.Revoke(t.Context(), "someone-else", toks[0].ID), core.KindNotFound)
	require.NoError(t, e.tokens.Revoke(t.Context(), acct.ID, toks[0].ID))

	_, err = e.tokens.Resolve(t.Context(), secret)
	requireKind(t, err, core.KindUnauthenticated)
	requireKind(t, e.tokens.Revoke(t.Context(), acct.ID, toks[0].ID), core.KindNotFound)
}

func TestTokenOfLockedAccountIsUnauthenticated(t *testing.T) {
	e := newEnv(t, devPolicy())
	secret, _ := mint(t, e, "dev@example.com", core.ScopePush, nil)

	e.policies.Swap(nil)
	_, err := e.tokens.Resolve(t.Context(), secret)
	requireKind(t, err, core.KindUnauthenticated)

	e.policies.Swap([]config.Policy{devPolicy()})
	_, err = e.tokens.Resolve(t.Context(), secret)
	require.NoError(t, err)
}

func TestTokenLastUsedIsThrottled(t *testing.T) {
	e := newEnv(t, devPolicy())
	secret, acct := mint(t, e, "dev@example.com", core.ScopeRead, nil)
	first := e.clock.Now()

	lastUsed := func() *time.Time {
		toks, err := e.tokens.List(t.Context(), acct.ID)
		require.NoError(t, err)
		require.Len(t, toks, 1)
		return toks[0].LastUsedAt
	}
	require.Nil(t, lastUsed())

	_, err := e.tokens.Resolve(t.Context(), secret)
	require.NoError(t, err)
	require.NotNil(t, lastUsed())
	assert.True(t, first.Equal(*lastUsed()))

	e.clock.Advance(59 * time.Minute)
	_, err = e.tokens.Resolve(t.Context(), secret)
	require.NoError(t, err)
	assert.True(t, first.Equal(*lastUsed()), "a use inside the hour must not write")

	e.clock.Advance(time.Minute)
	_, err = e.tokens.Resolve(t.Context(), secret)
	require.NoError(t, err)
	assert.True(t, first.Add(time.Hour).Equal(*lastUsed()))
}

func TestAccountsEnsure(t *testing.T) {
	e := newEnv(t, devPolicy())

	a, err := e.accounts.Ensure(t.Context(), "  Dev@Example.com ")
	require.NoError(t, err)
	assert.Equal(t, "dev@example.com", a.Email)
	assert.NotEmpty(t, a.ID)

	again, err := e.accounts.Ensure(t.Context(), "dev@example.com")
	require.NoError(t, err)
	assert.Equal(t, a.ID, again.ID)

	_, err = e.accounts.Ensure(t.Context(), "dev@other.example")
	requireKind(t, err, core.KindForbidden)
	_, err = e.accounts.Ensure(t.Context(), " ")
	requireKind(t, err, core.KindInvalid)
}

func TestAccountsDelete(t *testing.T) {
	e := newEnv(t, devPolicy())
	secret, _ := mint(t, e, "dev@example.com", core.ScopeRead, nil)

	require.NoError(t, e.accounts.Delete(t.Context(), "DEV@example.com"))

	_, err := e.tokens.Resolve(t.Context(), secret)
	requireKind(t, err, core.KindUnauthenticated)
	accts, err := e.accounts.List(t.Context())
	require.NoError(t, err)
	assert.Empty(t, accts)
	requireKind(t, e.accounts.Delete(t.Context(), "dev@example.com"), core.KindNotFound)
}
