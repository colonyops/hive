package credentials

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The override name is derived from the provider rather than declared per
// connector, so a new provider gets one for free. github must keep producing
// HIVE_GITHUB_TOKEN exactly: CI, the server build, and the e2e harness all set
// that name today.
func TestEnvOverrideNameDerivesFromTheProvider(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"github":  "HIVE_GITHUB_TOKEN",
		"grafana": "HIVE_GRAFANA_TOKEN",
		"my-host": "HIVE_MY_HOST_TOKEN",
	}
	for provider, want := range cases {
		assert.Equalf(t, want, EnvOverrideName(provider), "provider %q", provider)
	}
}

func TestResolveReturnsEmptyWhenNothingIsStored(t *testing.T) {
	t.Parallel()

	value, err := Resolve(NewMemoryStore(), Ref{Provider: "github", Account: "octocat"})
	require.NoError(t, err, "an unconnected account is a state, not a failure")
	assert.Empty(t, value)
}

// The override exists so a headless run needs no keychain at all, which only
// works if it outranks what is stored.
func TestResolvePrefersTheEnvironmentOverride(t *testing.T) {
	t.Setenv("HIVE_GITHUB_TOKEN", "from-env")

	store := NewMemoryStore()
	ref := Ref{Provider: "github", Account: "octocat"}
	require.NoError(t, store.Set(ref, "from-keychain"))

	value, err := Resolve(store, ref)
	require.NoError(t, err)
	assert.Equal(t, "from-env", value)

	// Scoped to its own provider: another provider must not pick it up.
	other := Ref{Provider: "grafana", Account: "prod"}
	require.NoError(t, store.Set(other, "grafana-token"))
	value, err = Resolve(store, other)
	require.NoError(t, err)
	assert.Equal(t, "grafana-token", value)
}

// A Resolver reads through on every call. This is the invariant that makes a
// rotated or newly connected credential take effect on the next fetch: bind
// once at wiring time, capture the value, and a token rotation would not apply
// until the app restarted.
func TestBindReadsThroughOnEveryCall(t *testing.T) {
	t.Parallel()

	store := NewMemoryStore()
	ref := Ref{Provider: "github", Account: "octocat"}
	resolve := Bind(store, ref)

	value, err := resolve()
	require.NoError(t, err)
	assert.Empty(t, value, "bound before the account was connected")

	require.NoError(t, store.Set(ref, "first"))
	value, err = resolve()
	require.NoError(t, err)
	assert.Equal(t, "first", value)

	require.NoError(t, store.Set(ref, "rotated"))
	value, err = resolve()
	require.NoError(t, err)
	assert.Equal(t, "rotated", value, "a rotated credential must not need a restart")

	require.NoError(t, store.Delete(ref))
	value, err = resolve()
	require.NoError(t, err)
	assert.Empty(t, value, "a disconnected account must not keep fetching")
}

func TestListProviderSelectsOneProvidersAccounts(t *testing.T) {
	t.Parallel()

	store := NewMemoryStore()
	for _, ref := range []Ref{
		{Provider: "github", Account: "octocat"},
		{Provider: "github", Account: "hubot"},
		{Provider: "grafana", Account: "prod"},
	} {
		require.NoError(t, store.Set(ref, "token"))
	}

	refs, err := ListProvider(store, "github")
	require.NoError(t, err)
	assert.Equal(t, []Ref{
		{Provider: "github", Account: "hubot"},
		{Provider: "github", Account: "octocat"},
	}, refs)

	refs, err = ListProvider(store, "gitlab")
	require.NoError(t, err)
	assert.Empty(t, refs)
}
