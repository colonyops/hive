package credentials

import (
	"errors"
	"os"
	"strings"
)

// Resolver returns a credential's current value, or "" when none is stored.
//
// Empty-and-no-error is deliberate: with providers configured per connector
// rather than gating the app, "not connected" is an ordinary state a caller
// reports, not an error it propagates. A consumer that needs a token maps ""
// to its own "not authenticated" error at the point it would have used one.
//
// A Resolver reads the store on every call. That is what makes connecting,
// rotating, or disconnecting a credential take effect on the next use —
// nothing captures the value, so there is no cache to invalidate.
type Resolver func() (string, error)

// EnvOverrideName is the environment variable that overrides every stored
// credential for one provider: HIVE_GITHUB_TOKEN, HIVE_GRAFANA_TOKEN.
//
// It is provider-wide rather than per-account because its purpose is headless
// operation — CI, the server build, the e2e harness — where there is one
// identity and no keychain to read. A machine with several accounts
// configured is not the case it serves.
func EnvOverrideName(provider string) string {
	var b strings.Builder
	b.WriteString("HIVE_")
	for _, r := range strings.ToUpper(provider) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			continue
		}
		b.WriteRune('_')
	}
	b.WriteString("_TOKEN")
	return b.String()
}

// HasEnvOverride reports whether a provider's environment override is set.
//
// It is what lets a caller tell "connected with no stored credential" from
// "not connected": the override is provider-wide and names no account, so a
// listing of stored refs is empty for it while every fetch still succeeds.
// Reporting that as disconnected would contradict the working feed beside it.
func HasEnvOverride(provider string) bool {
	return os.Getenv(EnvOverrideName(provider)) != ""
}

// Resolve reads one ref's value, preferring the provider's environment
// override. It returns "" and no error when nothing is stored.
func Resolve(store Store, ref Ref) (string, error) {
	if err := ref.Validate(); err != nil {
		return "", err
	}
	if value := os.Getenv(EnvOverrideName(ref.Provider)); value != "" {
		return value, nil
	}

	value, err := store.Get(ref)
	if errors.Is(err, ErrNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return value, nil
}

// Bind returns a Resolver over one ref.
func Bind(store Store, ref Ref) Resolver {
	return func() (string, error) { return Resolve(store, ref) }
}

// ListProvider returns every stored ref for one provider, sorted. It is what
// an Integrations screen enumerates and what a connector offers when a node
// picks which account to use.
func ListProvider(store Store, provider string) ([]Ref, error) {
	refs, err := store.List()
	if err != nil {
		return nil, err
	}
	out := make([]Ref, 0, len(refs))
	for _, ref := range refs {
		if ref.Provider == provider {
			out = append(out, ref)
		}
	}
	return out, nil
}
