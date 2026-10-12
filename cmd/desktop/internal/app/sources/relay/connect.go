package relay

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"

	"filippo.io/age"
	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/sourcehttp"
	"github.com/colonyops/hive/internal/platform/credentials"
)

// Instance is one connected relay account, as the Integrations screen renders
// it.
type Instance struct {
	Account string `json:"account"`
	URL     string `json:"url"`
	Email   string `json:"email"`
}

// clientFactory is a field so a test can point the authenticator anywhere.
type clientFactory func(base string, token func() string) *Client

// Authenticator connects and disconnects relay accounts, signing in with an
// API key, an emailed code, or a password.
type Authenticator struct {
	creds     credentials.Store
	instances *InstanceStore
	newClient clientFactory
	onChange  func(credentials.Ref)
	logger    zerolog.Logger
}

func NewAuthenticator(creds credentials.Store, instances *InstanceStore, version string, logger zerolog.Logger, onChange func(credentials.Ref)) *Authenticator {
	return &Authenticator{
		creds:     creds,
		instances: instances,
		newClient: func(base string, token func() string) *Client {
			return NewClient(base, token, version, logger)
		},
		onChange: onChange,
		logger:   logger,
	}
}

// ConnectWithKey validates the key against /v1/me before persisting anything,
// so a rejected paste leaves nothing behind.
func (a *Authenticator) ConnectWithKey(ctx context.Context, rawURL, key string) (Instance, error) {
	base, host, err := normalizeRelayURL(rawURL)
	if err != nil {
		return Instance{}, err
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return Instance{}, errors.New("relay: API key is empty")
	}

	me, err := a.newClient(base, func() string { return key }).Me(ctx)
	if errors.Is(err, sourcehttp.ErrUnauthorized) {
		return Instance{}, errors.New("the relay rejected the API key")
	}
	if err != nil {
		return Instance{}, fmt.Errorf("%s does not answer as a Hive Relay: %w", base, err)
	}
	if me.Email == "" {
		return Instance{}, errors.New("relay: the API key authenticates no account")
	}

	return a.store(ctx, base, host, me.Email, key)
}

// StartEmail asks the relay to mail a sign-in code. It stores nothing; the
// code comes back through VerifyEmail.
func (a *Authenticator) StartEmail(ctx context.Context, rawURL, email string) error {
	base, _, err := normalizeRelayURL(rawURL)
	if err != nil {
		return err
	}
	email, err = requireEmail(email)
	if err != nil {
		return err
	}
	return a.newClient(base, nil).StartEmail(ctx, email)
}

// VerifyEmail trades an emailed code for a device token and connects the
// account it names.
func (a *Authenticator) VerifyEmail(ctx context.Context, rawURL, email, code string) (Instance, error) {
	base, host, err := normalizeRelayURL(rawURL)
	if err != nil {
		return Instance{}, err
	}
	email, err = requireEmail(email)
	if err != nil {
		return Instance{}, err
	}
	signed, err := a.newClient(base, nil).VerifyEmail(ctx, email, strings.TrimSpace(code), deviceName())
	if err != nil {
		return Instance{}, err
	}
	return a.storeSignedIn(ctx, base, host, signed)
}

// Login trades a password for a device token and connects the account.
func (a *Authenticator) Login(ctx context.Context, rawURL, email, password string) (Instance, error) {
	base, host, err := normalizeRelayURL(rawURL)
	if err != nil {
		return Instance{}, err
	}
	email, err = requireEmail(email)
	if err != nil {
		return Instance{}, err
	}
	if password == "" {
		return Instance{}, errors.New("relay: password is empty")
	}
	signed, err := a.newClient(base, nil).Login(ctx, email, password, deviceName())
	if err != nil {
		return Instance{}, err
	}
	return a.storeSignedIn(ctx, base, host, signed)
}

func (a *Authenticator) storeSignedIn(ctx context.Context, base, host string, signed SignedIn) (Instance, error) {
	if signed.Token == "" || signed.Email == "" {
		return Instance{}, errors.New("relay: the sign-in response carries no token")
	}
	return a.store(ctx, base, host, signed.Email, signed.Token)
}

// store persists token, the account's binding, and this device's age
// identity, then registers the identity's recipient. The account is keyed by
// the email the relay reports, never the one typed, so it matches what the
// relay authenticates the token as.
//
// A failed registration undoes everything written here: a desktop the relay
// does not seal to would show every encrypted delivery as a placeholder.
func (a *Authenticator) store(ctx context.Context, base, host, email, token string) (Instance, error) {
	ref := credentials.Ref{Provider: Provider, Account: accountID(host, email)}
	previous, hadPrevious, err := a.previousKey(ref)
	if err != nil {
		return Instance{}, err
	}
	previousBinding, err := a.instances.Get(ref)
	if err != nil {
		return Instance{}, err
	}
	if err := a.creds.Set(ref, token); err != nil {
		return Instance{}, err
	}
	if err := a.instances.Set(ref, Binding{URL: base, Email: email}); err != nil {
		if rollbackErr := a.rollbackKey(ref, previous, hadPrevious); rollbackErr != nil {
			return Instance{}, errors.Join(err, fmt.Errorf("rollback relay key: %w", rollbackErr))
		}
		return Instance{}, err
	}

	identity, created, err := a.ensureIdentity(ref.Account)
	if err == nil {
		client := a.newClient(base, func() string { return token })
		_, err = client.AddRecipient(ctx, identity.Recipient().String(), deviceName())
		if err != nil {
			err = fmt.Errorf("registering this device's encryption key with the relay: %w", err)
		}
	}
	if err != nil {
		rollback := []error{err, a.rollbackKey(ref, previous, hadPrevious), a.rollbackBinding(ref, previousBinding)}
		if created {
			rollback = append(rollback, a.creds.Delete(ageRef(ref.Account)))
		}
		return Instance{}, errors.Join(rollback...)
	}

	a.notify(ref)
	return Instance{Account: ref.Account, URL: base, Email: email}, nil
}

// ensureIdentity returns the account's stored age identity, generating and
// storing one on first connect. created reports whether it was generated.
func (a *Authenticator) ensureIdentity(account string) (identity *age.X25519Identity, created bool, err error) {
	ref := ageRef(account)
	raw, err := a.creds.Get(ref)
	switch {
	case err == nil:
		identity, err = age.ParseX25519Identity(raw)
		if err == nil {
			return identity, false, nil
		}
		a.logger.Warn().Err(err).Str("credential", ref.String()).Msg("relay: stored age identity does not parse; generating a new one")
	case !errors.Is(err, credentials.ErrNotFound):
		return nil, false, fmt.Errorf("read relay age identity: %w", err)
	}
	identity, err = age.GenerateX25519Identity()
	if err != nil {
		return nil, false, fmt.Errorf("generate relay age identity: %w", err)
	}
	if err := a.creds.Set(ref, identity.String()); err != nil {
		return nil, false, fmt.Errorf("store relay age identity: %w", err)
	}
	return identity, true, nil
}

func requireEmail(email string) (string, error) {
	email = strings.TrimSpace(email)
	if email == "" {
		return "", errors.New("relay: email is required")
	}
	return email, nil
}

// maxDeviceName is the relay's limit on a token's device label.
const maxDeviceName = 80

// deviceName labels the minted token in the relay's token list, so the user
// can tell which machine to revoke.
func deviceName() string {
	name, err := os.Hostname()
	name = strings.TrimSpace(name)
	if err != nil || name == "" {
		return "Hive Desktop"
	}
	if runes := []rune(name); len(runes) > maxDeviceName {
		return string(runes[:maxDeviceName])
	}
	return name
}

// Disconnect is idempotent: an account with nothing stored disconnects
// cleanly. It first asks the relay to stop sealing to this device; a relay
// that cannot be reached does not block the disconnect.
func (a *Authenticator) Disconnect(ctx context.Context, account string) error {
	ref := credentials.Ref{Provider: Provider, Account: strings.TrimSpace(account)}
	if err := ref.Validate(); err != nil {
		return err
	}
	if err := a.removeRecipient(ctx, ref); err != nil {
		a.logger.Warn().Err(err).Str("credential", ref.String()).Msg("relay: removing this device's recipient failed")
	}
	credErr := a.creds.Delete(ref)
	identityErr := a.creds.Delete(ageRef(ref.Account))
	bindingErr := a.instances.Delete(ref)
	a.notify(ref)
	return errors.Join(credErr, identityErr, bindingErr)
}

func (a *Authenticator) removeRecipient(ctx context.Context, ref credentials.Ref) error {
	raw, err := a.creds.Get(ageRef(ref.Account))
	if errors.Is(err, credentials.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	identity, err := age.ParseX25519Identity(raw)
	if err != nil {
		return err
	}
	binding, err := a.instances.Get(ref)
	if err != nil || binding.URL == "" {
		return err
	}
	token, err := a.creds.Get(ref)
	if err != nil {
		return err
	}
	client := a.newClient(binding.URL, func() string { return token })
	recipients, err := client.ListRecipients(ctx)
	if err != nil {
		return err
	}
	mine := identity.Recipient().String()
	for _, r := range recipients {
		if r.Recipient == mine {
			return client.RemoveRecipient(ctx, r.ID)
		}
	}
	return nil
}

// Methods reports which sign-in methods a relay offers. It needs no key, so
// the connect form can ask before the user has one.
func (a *Authenticator) Methods(ctx context.Context, rawURL string) (Methods, error) {
	base, _, err := normalizeRelayURL(rawURL)
	if err != nil {
		return Methods{}, err
	}
	return a.newClient(base, nil).Methods(ctx)
}

// Accounts lists the stored relay credentials with their bindings. A ref
// with no binding is listed with an empty URL rather than dropped, so a
// half-connected account is still visible and can be disconnected.
func (a *Authenticator) Accounts(_ context.Context) ([]Instance, error) {
	refs, err := credentials.ListProvider(a.creds, Provider)
	if err != nil {
		return nil, err
	}
	out := make([]Instance, 0, len(refs))
	for _, ref := range refs {
		binding, err := a.instances.Get(ref)
		if err != nil {
			return nil, err
		}
		out = append(out, Instance{Account: ref.Account, URL: binding.URL, Email: binding.Email})
	}
	return out, nil
}

func (a *Authenticator) previousKey(ref credentials.Ref) (key string, ok bool, err error) {
	key, err = a.creds.Get(ref)
	if err == nil {
		return key, true, nil
	}
	if errors.Is(err, credentials.ErrNotFound) {
		return "", false, nil
	}
	return "", false, fmt.Errorf("read existing relay key: %w", err)
}

func (a *Authenticator) rollbackKey(ref credentials.Ref, previous string, hadPrevious bool) error {
	if hadPrevious {
		return a.creds.Set(ref, previous)
	}
	return a.creds.Delete(ref)
}

func (a *Authenticator) rollbackBinding(ref credentials.Ref, previous Binding) error {
	if previous.URL != "" {
		return a.instances.Set(ref, previous)
	}
	return a.instances.Delete(ref)
}

func (a *Authenticator) notify(ref credentials.Ref) {
	if a.onChange != nil {
		a.onChange(ref)
	}
}

// normalizeRelayURL returns the base URL to store and the host naming the
// account. HTTPS is required except for loopback, and userinfo is rejected so
// a key is never sent to a URL with credentials baked into it.
func normalizeRelayURL(raw string) (base, host string, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", errors.New("relay: URL is required")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", "", fmt.Errorf("relay: invalid URL: %w", err)
	}
	if parsed.Host == "" {
		return "", "", errors.New("relay: URL has no host (did you include https://?)")
	}
	if parsed.User != nil {
		return "", "", errors.New("relay: URL must not contain userinfo")
	}
	host = strings.ToLower(parsed.Host)
	switch parsed.Scheme {
	case "https":
	case "http":
		if !isLoopbackHost(host) {
			return "", "", errors.New("relay: URL must use https (http is allowed only for loopback)")
		}
	default:
		return "", "", errors.New("relay: URL must use https")
	}
	return parsed.Scheme + "://" + parsed.Host + strings.TrimRight(parsed.Path, "/"), host, nil
}

func isLoopbackHost(host string) bool {
	name := host
	if hostOnly, _, err := net.SplitHostPort(host); err == nil {
		name = hostOnly
	}
	name = strings.Trim(name, "[]")
	if strings.EqualFold(name, "localhost") {
		return true
	}
	if ip := net.ParseIP(name); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

// accountID is host plus email: host alone collides across two accounts on one
// relay, email alone across relays. A "/" would break the ref's
// provider/account split, so it is replaced.
func accountID(host, email string) string {
	clean := strings.NewReplacer("/", "_")
	return clean.Replace(host) + "-" + clean.Replace(email)
}
