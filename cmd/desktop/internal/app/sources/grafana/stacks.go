package grafana

import (
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/bindingstore"
)

// StackStore persists each connected stack's non-secret base URL, keyed by
// credential ref (see bindingstore).
type StackStore = bindingstore.Store[stackEntry]

func NewStackStore(path string) *StackStore {
	return bindingstore.New[stackEntry]("grafana stacks", path)
}

type stackEntry struct {
	URL string `json:"url"`
}
