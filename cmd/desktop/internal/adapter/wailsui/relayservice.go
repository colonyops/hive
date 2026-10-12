package wailsui

import (
	"context"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/relay"
)

// RelayService exposes Hive Relay accounts and sources to the frontend. Like
// every connector's service it gates nothing in the app.
type RelayService struct {
	relay *app.RelayService
}

func NewRelayService(r *app.RelayService) *RelayService { return &RelayService{relay: r} }

// ConnectWithKey validates the relay URL and API key, then stores the
// credential. It returns the account a source node names.
func (s *RelayService) ConnectWithKey(ctx context.Context, url, key string) (relay.Instance, error) {
	return s.relay.ConnectWithKey(ctx, url, key)
}

func (s *RelayService) Disconnect(ctx context.Context, account string) error {
	return s.relay.Disconnect(ctx, account)
}

// Methods reports the sign-in methods a relay offers; it needs no key.
func (s *RelayService) Methods(ctx context.Context, url string) (relay.Methods, error) {
	return s.relay.Methods(ctx, url)
}

func (s *RelayService) Accounts(ctx context.Context) ([]relay.Instance, error) {
	return s.relay.Accounts(ctx)
}

func (s *RelayService) ListSources(ctx context.Context, account string) ([]relay.SourceView, error) {
	return s.relay.ListSources(ctx, account)
}

// CreateSource returns the new source with its ingress URL, which the relay
// shows only this once.
func (s *RelayService) CreateSource(ctx context.Context, account, name, secret string) (relay.SourceView, error) {
	return s.relay.CreateSource(ctx, account, name, secret)
}

// RotateSource returns the new ingress URL; the old one stops accepting
// deliveries.
func (s *RelayService) RotateSource(ctx context.Context, account, id string) (string, error) {
	return s.relay.RotateSource(ctx, account, id)
}

func (s *RelayService) DeleteSource(ctx context.Context, account, id string) error {
	return s.relay.DeleteSource(ctx, account, id)
}

func (s *RelayService) RecentDeliveries(ctx context.Context, account, sourceID string, limit int) ([]relay.RecentDelivery, error) {
	return s.relay.RecentDeliveries(ctx, account, sourceID, limit)
}
