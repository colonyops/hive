package mcpcatalog

import (
	"context"
	"sync"
)

// AccountResolver returns the base URL and request headers an AccountURL
// server is reached with for one workspace. It may create a credential the
// first time it is asked, so it runs when a workspace is generated, never
// when the catalogue is only listed. An error means no account can serve the
// workspace; the caller reports it as a Problem.
type AccountResolver func(ctx context.Context, workspaceID string) (baseURL string, headers map[string]string, err error)

// AccountResolvers holds one resolver per server Type. The resolvers are
// registered by whoever owns the account, after the catalogue's consumer is
// built.
type AccountResolvers struct {
	mu sync.RWMutex
	m  map[string]AccountResolver
}

func (r *AccountResolvers) Register(mcpType string, resolve AccountResolver) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.m == nil {
		r.m = map[string]AccountResolver{}
	}
	r.m[mcpType] = resolve
}

func (r *AccountResolvers) Lookup(mcpType string) (AccountResolver, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	resolve, ok := r.m[mcpType]
	return resolve, ok
}
