package app

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"path/filepath"
	"time"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/drive-server/internal/config"
	"github.com/colonyops/hive/cmd/drive-server/internal/core"
	"github.com/colonyops/hive/cmd/drive-server/internal/httpapi"
	"github.com/colonyops/hive/cmd/drive-server/internal/store/control"
	"github.com/colonyops/hive/cmd/drive-server/internal/store/userdb"
	"github.com/colonyops/hive/pkg/logutils"
)

const (
	maxOpenUserDBs = 128
	userDBIdle     = 5 * time.Minute
)

// Services are the core services over one control.db.
type Services struct {
	Policies  *core.Policies
	Accounts  *core.Accounts
	Tokens    *core.Tokens
	Documents *core.Documents
	Shares    *core.Shares
	Sessions  *core.Sessions
	SignIn    *core.SignIn
	Sweeper   *core.Sweeper
	Outbox    core.OutboxStore

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
	accounts := core.NewAccounts(store.Accounts(), users.Stores(), policies)
	sessions := core.NewSessions(store.Sessions(), store.Accounts(), policies)
	return &Services{
		Policies:  policies,
		Accounts:  accounts,
		Tokens:    core.NewTokens(store.Tokens(), store.Accounts(), policies),
		Documents: core.NewDocuments(users.Stores(), store.Shares(), cfg.MaxDocumentSize.Bytes, core.WithBaseURL(cfg.BaseURL)),
		Shares:    core.NewShares(store.Shares(), store.Accounts(), users.Stores(), policies, core.WithBaseURL(cfg.BaseURL)),
		Sessions:  sessions,
		SignIn:    core.NewSignIn(log, store.Codes(), store.Accounts(), store.Outbox(), policies, accounts, sessions, core.WithBaseURL(cfg.BaseURL)),
		Sweeper:   core.NewSweeper(log, store.Shares(), store.Codes(), store.Sessions(), users.Stores()),
		Outbox:    store.Outbox(),
		control:   store,
		users:     users,
	}, nil
}

func (s *Services) Close() error { return errors.Join(s.users.Close(), s.control.Close()) }

func (s *Services) HTTP(baseURL string, trustedProxies []netip.Prefix) httpapi.Services {
	return httpapi.Services{
		Tokens:         s.Tokens,
		Sessions:       s.Sessions,
		SignIn:         s.SignIn,
		Documents:      s.Documents,
		Shares:         s.Shares,
		BaseURL:        baseURL,
		TrustedProxies: trustedProxies,
	}
}
