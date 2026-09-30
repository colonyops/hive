package gitea

import (
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/bindingstore"
)

// InstanceStore persists each connected account's non-secret binding — the
// instance base URL — keyed by credential ref (see bindingstore).
type InstanceStore = bindingstore.Store[Binding]

func NewInstanceStore(path string) *InstanceStore {
	return bindingstore.New[Binding]("gitea instances", path)
}

// Binding is what a poll needs to address one connected account.
type Binding struct {
	URL string `json:"url"`
	// Login is the authenticated user the token belongs to, kept for display;
	// the account half of the ref is host-qualified and is not it.
	Login string `json:"login,omitempty"`
	// Version is the instance version recorded at connect time. Informational:
	// nothing branches on it, but it is what a support question asks for.
	Version string `json:"version,omitempty"`
}
