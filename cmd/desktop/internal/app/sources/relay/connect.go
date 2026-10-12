package relay

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"

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

	return a.store(base, host, me.Email, key)
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
	return a.storeSignedIn(base, host, signed)
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
	return a.storeSignedIn(base, host, signed)
}

func (a *Authenticator) storeSignedIn(base, host string, signed SignedIn) (Instance, error) {
	if signed.Token == "" || signed.Email == "" {
		return Instance{}, errors.New("relay: the sign-in response carries no token")
	}
	return a.store(base, host, signed.Email, signed.Token)
}

// store persists token and the account's binding. The account is keyed by the
// email the relay reports, never the one typed, so it matches what the relay
// authenticates the token as.
func (a *Authenticator) store(base, host, email, token string) (Instance, error) {
	ref := credentials.Ref{Provider: Provider, Account: accountID(host, email)}
	previous, hadPrevious, err := a.previousKey(ref)
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

	a.notify(ref)
	return Instance{Account: ref.Account, URL: base, Email: email}, nil
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

// Disconnect is idempotent: an account with nothing stored disconnects cleanly.
func (a *Authenticator) Disconnect(_ context.Context, account string) error {
	ref := credentials.Ref{Provider: Provider, Account: strings.TrimSpace(account)}
	if err := ref.Validate(); err != nil {
		return err
	}
	credErr := a.creds.Delete(ref)
	bindingErr := a.instances.Delete(ref)
	a.notify(ref)
	if credErr != nil {
		return credErr
	}
	return bindingErr
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
