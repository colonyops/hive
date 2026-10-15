package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/drive-server/internal/config"
	"github.com/colonyops/hive/cmd/drive-server/internal/core"
	"github.com/colonyops/hive/cmd/drive-server/internal/store/control"
	"github.com/colonyops/hive/cmd/drive-server/internal/store/userdb"
	"github.com/colonyops/hive/pkg/logutils"
)

const (
	maxOpenUserDBs = 128
	userDBIdle     = 5 * time.Minute
)

// Services are the core services over one control.db. The server and the
// admin commands build the same set.
type Services struct {
	Policies  *core.Policies
	Accounts  *core.Accounts
	Tokens    *core.Tokens
	Documents *core.Documents

	control *control.Store
	users   *userdb.Pool
}

func OpenServices(ctx context.Context, log zerolog.Logger, cfg config.Config) (*Services, error) {
	store, err := control.Open(ctx, cfg.DataDir)
	if err != nil {
		return nil, fmt.Errorf("open control store: %w", err)
	}
	users := userdb.NewPool(logutils.Component(log, "userdb"), filepath.Join(cfg.DataDir, "users"), maxOpenUserDBs, userDBIdle)
	policies := core.NewPolicies(cfg.UserPolicies)
	return &Services{
		Policies:  policies,
		Accounts:  core.NewAccounts(store.Accounts(), users.Stores(), policies),
		Tokens:    core.NewTokens(store.Tokens(), store.Accounts(), policies),
		Documents: core.NewDocuments(users.Stores(), cfg.MaxDocumentSize.Bytes),
		control:   store,
		users:     users,
	}, nil
}

func (s *Services) Close() error { return errors.Join(s.users.Close(), s.control.Close()) }
