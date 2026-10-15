package app

import (
	"context"
	"fmt"

	"github.com/colonyops/hive/cmd/drive-server/internal/config"
	"github.com/colonyops/hive/cmd/drive-server/internal/core"
	"github.com/colonyops/hive/cmd/drive-server/internal/store/control"
)

// Services are the core services over one control.db. The server and the
// admin commands build the same set.
type Services struct {
	Policies *core.Policies
	Accounts *core.Accounts
	Tokens   *core.Tokens

	control *control.Store
}

func OpenServices(ctx context.Context, cfg config.Config) (*Services, error) {
	store, err := control.Open(ctx, cfg.DataDir)
	if err != nil {
		return nil, fmt.Errorf("open control store: %w", err)
	}
	policies := core.NewPolicies(cfg.UserPolicies)
	return &Services{
		Policies: policies,
		Accounts: core.NewAccounts(store.Accounts(), policies),
		Tokens:   core.NewTokens(store.Tokens(), store.Accounts(), policies),
		control:  store,
	}, nil
}

func (s *Services) Close() error { return s.control.Close() }
