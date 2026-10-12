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

// StartEmail asks the relay to mail a sign-in code; it stores nothing.
func (s *RelayService) StartEmail(ctx context.Context, url, email string) error {
	return s.relay.StartEmail(ctx, url, email)
}

// VerifyEmail trades the mailed code for a device token and stores it.
func (s *RelayService) VerifyEmail(ctx context.Context, url, email, code string) (relay.Instance, error) {
	return s.relay.VerifyEmail(ctx, url, email, code)
}

// Login trades a password for a device token and stores it.
func (s *RelayService) Login(ctx context.Context, url, email, password string) (relay.Instance, error) {
	return s.relay.Login(ctx, url, email, password)
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

func (s *RelayService) Me(ctx context.Context, account string) (relay.Me, error) {
	return s.relay.Me(ctx, account)
}

// SetEncryption turns sealing on or off for later deliveries. Turning it on
// needs at least one registered recipient.
func (s *RelayService) SetEncryption(ctx context.Context, account string, on bool) (relay.Me, error) {
	return s.relay.SetEncryption(ctx, account, on)
}

// ListRecipients lists the devices the relay seals deliveries to.
func (s *RelayService) ListRecipients(ctx context.Context, account string) ([]relay.RecipientView, error) {
	return s.relay.ListRecipients(ctx, account)
}

func (s *RelayService) RevokeRecipient(ctx context.Context, account, id string) error {
	return s.relay.RevokeRecipient(ctx, account, id)
}
