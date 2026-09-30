// Package credentials stores provider secrets keyed by Ref{Provider,
// Account}. Values live in the OS keychain; the refs themselves live in a
// separate index, because keychains cannot enumerate.
//
// It is deliberately a leaf: no dependency on app, flow, or any connector, so
// a connector package can import it without a cycle — the same rule that put
// the connector vocabulary in internal/app/sources/connector.
//
// Two rules govern everything here. **Config holds refs, never tokens**:
// flows/ is dotfiles-managed, so a token embedded in a node's config is a
// token in a git repo. And **the keychain is the truth, the index is a
// cache**: a ref present in the index whose keychain entry is gone reads as
// absent and is pruned, because the alternative is a "Connected" badge over a
// credential that no longer exists.
package credentials

import (
	"errors"
	"fmt"
	"strings"
)

// ErrNotFound is returned by Get for a ref with no stored value. It is a
// distinct error rather than a zero Secret so a caller cannot mistake "no
// credential" for "an empty credential" — the first is an ordinary state that
// means "not connected", the second is corruption.
var ErrNotFound = errors.New("credentials: not found")

// Ref identifies one credential. A Value Object: immutable, compared by
// value, self-validating, with no identity of its own. A credential is
// referred to by this and never by a bare string, so "which account" cannot
// be lost in a signature that takes two strings in the wrong order.
//
// Refs compare exactly and are not case-normalised. Providers come from
// connector descriptor types, which are constants.
type Ref struct {
	// Provider names the connector family: "github", "grafana".
	Provider string
	// Account distinguishes several credentials for one provider — a login, a
	// hostname, an environment.
	Account string
}

// String is the wire and storage form, "github/octocat". It is also the
// keychain account name, which is why the separator may not appear in either
// half.
func (r Ref) String() string { return r.Provider + "/" + r.Account }

// Validate reports whether the ref is well-formed. Both halves must be
// non-empty and neither may contain the separator, or String and ParseRef
// stop being inverses.
func (r Ref) Validate() error {
	switch {
	case r.Provider == "":
		return fmt.Errorf("credentials: ref has no provider")
	case r.Account == "":
		return fmt.Errorf("credentials: ref %q has no account", r.Provider)
	case strings.Contains(r.Provider, "/"):
		return fmt.Errorf("credentials: provider %q contains %q", r.Provider, "/")
	case strings.Contains(r.Account, "/"):
		return fmt.Errorf("credentials: account %q contains %q", r.Account, "/")
	}
	return nil
}

// ParseRef reads the "provider/account" form a node's `credential:` field
// carries.
func ParseRef(s string) (Ref, error) {
	provider, account, found := strings.Cut(s, "/")
	if !found {
		return Ref{}, fmt.Errorf("credentials: %q is not provider/account", s)
	}
	ref := Ref{Provider: provider, Account: account}
	if err := ref.Validate(); err != nil {
		return Ref{}, err
	}
	return ref, nil
}

// Store persists credentials. Get returns ErrNotFound when a ref has no
// value; List enumerates the refs that have one.
//
// Values are plain strings. A redacting wrapper type earns its place when
// secrets are loaded from config and flow through structs that get marshalled
// and logged — but the rule above means config carries refs and never tokens,
// so no such path exists here. A value's whole life is keychain → connector
// factory → provider client, and none of those marshal it.
//
// A consumer-defined interface lives with its consumer, but this one is
// declared here on purpose: it has two implementations in this package
// (keychain and memory) and the narrow views its callers want — "read one
// credential" — are declared at those call sites instead.
type Store interface {
	Get(ref Ref) (string, error)
	Set(ref Ref, value string) error
	Delete(ref Ref) error
	List() ([]Ref, error)
}
