package app

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/relay"
	"github.com/colonyops/hive/internal/platform/credentials"
)

// relayMCPProvider holds the per-workspace keys the hive-relay MCP entry
// writes into a workspace, apart from the account's own key so revoking one
// workspace never disconnects the account.
const relayMCPProvider = "relay-mcp"

const relayTokenNameMax = 80

var errNoRelayAccount = errors.New("no Hive Relay account is connected; connect one under Settings ▸ Integrations")

// relayMCPResolver resolves the hive-relay catalogue entry for a workspace:
// the first connected account's relay URL and a key minted for that
// workspace alone.
type relayMCPResolver struct {
	accounts  func(context.Context) ([]relay.Instance, error)
	clientFor func(credentials.Ref) (*relay.Client, error)
	creds     credentials.Store

	// mu keeps two opens of one workspace from each minting a key.
	mu sync.Mutex
}

func (r *relayMCPResolver) Resolve(ctx context.Context, workspaceID string) (string, map[string]string, error) {
	accounts, err := r.accounts(ctx)
	if err != nil {
		return "", nil, fmt.Errorf("listing Hive Relay accounts: %w", err)
	}
	accounts = slices.DeleteFunc(accounts, func(a relay.Instance) bool { return a.URL == "" })
	if len(accounts) == 0 {
		return "", nil, errNoRelayAccount
	}
	account := slices.MinFunc(accounts, func(a, b relay.Instance) int { return cmp.Compare(a.Account, b.Account) })

	keyRef := credentials.Ref{Provider: relayMCPProvider, Account: account.Account + "-" + workspaceID}
	if err := keyRef.Validate(); err != nil {
		return "", nil, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	key, err := r.creds.Get(keyRef)
	if errors.Is(err, credentials.ErrNotFound) {
		key, err = r.mint(ctx, account, keyRef, workspaceID)
	}
	if err != nil {
		return "", nil, err
	}
	return account.URL, map[string]string{"Authorization": "Bearer " + key}, nil
}

func (r *relayMCPResolver) mint(ctx context.Context, account relay.Instance, keyRef credentials.Ref, workspaceID string) (string, error) {
	client, err := r.clientFor(credentials.Ref{Provider: relay.Provider, Account: account.Account})
	if err != nil {
		return "", err
	}
	name := "workspace:" + workspaceID
	if runes := []rune(name); len(runes) > relayTokenNameMax {
		name = string(runes[:relayTokenNameMax])
	}
	key, id, err := client.CreateToken(ctx, name)
	if err != nil {
		return "", fmt.Errorf("minting a Hive Relay key for the workspace: %w", err)
	}
	if err := r.creds.Set(keyRef, key); err != nil {
		if revokeErr := client.RevokeToken(context.WithoutCancel(ctx), id); revokeErr != nil {
			err = errors.Join(err, fmt.Errorf("revoke the unsaved key: %w", revokeErr))
		}
		return "", fmt.Errorf("saving the workspace's Hive Relay key: %w", err)
	}
	return key, nil
}
