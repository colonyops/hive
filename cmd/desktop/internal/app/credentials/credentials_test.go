package credentials

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// String and ParseRef are the two directions of one encoding: a ref is
// written into a node's `credential:` field and read back out. If they are
// not inverses, a credential saved through the UI resolves to nothing at the
// next tick.
func TestRefStringAndParseAreInverses(t *testing.T) {
	t.Parallel()

	for _, ref := range []Ref{
		{Provider: "github", Account: "octocat"},
		{Provider: "grafana", Account: "prod"},
		{Provider: "grafana", Account: "stack.example.com:3000"},
	} {
		parsed, err := ParseRef(ref.String())
		require.NoErrorf(t, err, "round-tripping %s", ref)
		assert.Equal(t, ref, parsed)
	}
}

func TestParseRefRejectsMalformedInput(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"no separator":        "github",
		"empty":               "",
		"no provider":         "/octocat",
		"no account":          "github/",
		"account has a slash": "github/hay/den",
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := ParseRef(input)
			assert.Errorf(t, err, "ParseRef(%q) should have failed", input)
		})
	}
}

// The store contract is shared, so both implementations are held to it. A
// memory store that diverges from the keychain store makes every mock-mode
// test a test of something the app does not do in production.
func TestStoreContract(t *testing.T) {
	for name, newStore := range map[string]func(t *testing.T) Store{
		"memory":   func(*testing.T) Store { return NewMemoryStore() },
		"keychain": func(t *testing.T) Store { return newTestKeychainStore(t) },
	} {
		t.Run(name, func(t *testing.T) {
			ref := Ref{Provider: "github", Account: "octocat"}

			t.Run("get is not found before set", func(t *testing.T) {
				store := newStore(t)
				_, err := store.Get(ref)
				require.ErrorIs(t, err, ErrNotFound)
			})

			t.Run("round trip", func(t *testing.T) {
				store := newStore(t)
				require.NoError(t, store.Set(ref, "token-value"))

				got, err := store.Get(ref)
				require.NoError(t, err)
				assert.Equal(t, "token-value", got)

				refs, err := store.List()
				require.NoError(t, err)
				assert.Equal(t, []Ref{ref}, refs)
			})

			t.Run("set overwrites", func(t *testing.T) {
				store := newStore(t)
				require.NoError(t, store.Set(ref, "first"))
				require.NoError(t, store.Set(ref, "second"))

				got, err := store.Get(ref)
				require.NoError(t, err)
				assert.Equal(t, "second", got)

				refs, err := store.List()
				require.NoError(t, err)
				assert.Len(t, refs, 1, "re-setting a credential must not duplicate its ref")
			})

			t.Run("delete removes value and ref", func(t *testing.T) {
				store := newStore(t)
				require.NoError(t, store.Set(ref, "token-value"))
				require.NoError(t, store.Delete(ref))

				_, err := store.Get(ref)
				require.ErrorIs(t, err, ErrNotFound)

				refs, err := store.List()
				require.NoError(t, err)
				assert.Empty(t, refs)
			})

			t.Run("deleting an absent ref is not an error", func(t *testing.T) {
				store := newStore(t)
				require.NoError(t, store.Delete(ref))
			})

			// An empty value is indistinguishable from "no credential" at
			// every later read, so it is rejected at the door rather than
			// stored and misread as a sign-out.
			t.Run("empty values are rejected", func(t *testing.T) {
				store := newStore(t)
				require.Error(t, store.Set(ref, ""))
			})

			t.Run("malformed refs are rejected", func(t *testing.T) {
				store := newStore(t)
				bad := Ref{Provider: "github"}
				require.Error(t, store.Set(bad, "token-value"))
				_, err := store.Get(bad)
				require.Error(t, err)
			})

			t.Run("list sorts and separates accounts", func(t *testing.T) {
				store := newStore(t)
				second := Ref{Provider: "github", Account: "alt"}
				other := Ref{Provider: "grafana", Account: "prod"}
				require.NoError(t, store.Set(ref, "one"))
				require.NoError(t, store.Set(other, "two"))
				require.NoError(t, store.Set(second, "three"))

				refs, err := store.List()
				require.NoError(t, err)
				assert.Equal(t, []Ref{second, ref, other}, refs)

				// Two accounts of one provider are genuinely separate
				// credentials — the dimension the vendored single-slot token
				// store could not express.
				got, err := store.Get(second)
				require.NoError(t, err)
				assert.Equal(t, "three", got)
			})
		})
	}
}
