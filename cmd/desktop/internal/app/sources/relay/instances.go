package relay

import "github.com/colonyops/hive/cmd/desktop/internal/app/sources/bindingstore"

// InstanceStore persists each connected account's relay base URL, keyed by
// credential ref, so a node can never pair an account's key with another host.
type InstanceStore = bindingstore.Store[Binding]

func NewInstanceStore(path string) *InstanceStore {
	return bindingstore.New[Binding]("relay instances", path)
}

type Binding struct {
	URL   string `json:"url"`
	Email string `json:"email"`
}
