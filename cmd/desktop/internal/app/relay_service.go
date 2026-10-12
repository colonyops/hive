package app

import (
	"context"
	"errors"

	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/relay"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/sourcehttp"
	"github.com/colonyops/hive/internal/platform/credentials"
)

// RelayService connects Hive Relay accounts and manages their sources. An
// account is the credentials.Ref account half, as Accounts reports it.
type RelayService struct {
	auth     *relay.Authenticator
	fetchers *relay.Fetchers
}

func newRelayService(auth *relay.Authenticator, fetchers *relay.Fetchers) *RelayService {
	return &RelayService{auth: auth, fetchers: fetchers}
}

func (s *RelayService) ConnectWithKey(ctx context.Context, url, key string) (relay.Instance, error) {
	instance, err := s.auth.ConnectWithKey(ctx, url, key)
	return instance, Wrap(err, KindUnauthenticated, "connecting to Hive Relay")
}

func (s *RelayService) Disconnect(ctx context.Context, account string) error {
	return Wrap(s.auth.Disconnect(ctx, account), KindInternal, "disconnecting Hive Relay")
}

func (s *RelayService) Methods(ctx context.Context, url string) (relay.Methods, error) {
	methods, err := s.auth.Methods(ctx, url)
	return methods, Wrap(err, relayErrorKind(err), "reading Hive Relay sign-in methods")
}

func (s *RelayService) Accounts(ctx context.Context) ([]relay.Instance, error) {
	accounts, err := s.auth.Accounts(ctx)
	return accounts, Wrap(err, KindInternal, "listing Hive Relay accounts")
}

func (s *RelayService) ListSources(ctx context.Context, account string) ([]relay.SourceView, error) {
	client, err := s.client(account)
	if err != nil {
		return nil, err
	}
	sources, err := client.ListSources(ctx)
	return sources, Wrap(err, relayErrorKind(err), "listing relay sources")
}

func (s *RelayService) CreateSource(ctx context.Context, account, name, secret string) (relay.SourceView, error) {
	client, err := s.client(account)
	if err != nil {
		return relay.SourceView{}, err
	}
	source, err := client.CreateSource(ctx, name, secret)
	return source, Wrap(err, relayErrorKind(err), "creating relay source")
}

func (s *RelayService) RotateSource(ctx context.Context, account, id string) (string, error) {
	client, err := s.client(account)
	if err != nil {
		return "", err
	}
	ingressURL, err := client.RotateSource(ctx, id)
	return ingressURL, Wrap(err, relayErrorKind(err), "rotating relay source")
}

func (s *RelayService) DeleteSource(ctx context.Context, account, id string) error {
	client, err := s.client(account)
	if err != nil {
		return err
	}
	err = client.DeleteSource(ctx, id)
	return Wrap(err, relayErrorKind(err), "deleting relay source")
}

// RecentDeliveries is the node editor's preview: the newest limit deliveries,
// each parsed under the auto dialect.
func (s *RelayService) RecentDeliveries(ctx context.Context, account, sourceID string, limit int) ([]relay.RecentDelivery, error) {
	client, err := s.client(account)
	if err != nil {
		return nil, err
	}
	recent, err := client.RecentDeliveries(ctx, sourceID, limit)
	return recent, Wrap(err, relayErrorKind(err), "reading relay deliveries")
}

func (s *RelayService) client(account string) (*relay.Client, error) {
	ref := credentials.Ref{Provider: relay.Provider, Account: account}
	if err := ref.Validate(); err != nil {
		return nil, Wrap(err, KindInvalid, "relay account %q", account)
	}
	client, err := s.fetchers.For(ref)
	return client, Wrap(err, relayErrorKind(err), "relay account %q", account)
}

func relayErrorKind(err error) Kind {
	switch {
	case errors.Is(err, relay.ErrNotConnected), errors.Is(err, sourcehttp.ErrUnauthorized):
		return KindUnauthenticated
	case errors.Is(err, sourcehttp.ErrRateLimited), errors.Is(err, sourcehttp.ErrUnreachable):
		return KindUnavailable
	default:
		return KindInternal
	}
}
