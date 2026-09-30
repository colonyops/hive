package gitea

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/desktop/internal/app/credentials"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/gitea/giteaclient"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/sourcehttp"
)

// Instance is one connected Gitea or Forgejo account, as the Integrations
// screen renders it.
type Instance struct {
	Account string `json:"account"`
	URL     string `json:"url"`
	Login   string `json:"login"`
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Authenticator connects and disconnects Gitea instances. Like Grafana's and
// unlike GitHub's there is no device flow: Gitea's is per-instance and needs an
// OAuth application registered on that server, which a desktop app cannot
// assume exists, so an account is connected by validating a pasted URL and
// access token once.
type Authenticator struct {
	creds     credentials.Store
	instances *InstanceStore
	newClient clientFactory
	onChange  func(credentials.Ref)
	logger    zerolog.Logger
}

func NewAuthenticator(creds credentials.Store, instances *InstanceStore, logger zerolog.Logger, onChange func(credentials.Ref)) *Authenticator {
	return &Authenticator{
		creds:     creds,
		instances: instances,
		newClient: func(base, token string) *giteaclient.Client {
			return giteaclient.NewClient(base, token, giteaclient.WithLogger(logger))
		},
		onChange: onChange,
		logger:   logger,
	}
}

// Connect validates the URL and token before persisting either, so a rejected
// paste leaves nothing behind.
//
// The version probe runs first and is what makes a wrong URL diagnosable: a
// host that is not a Gitea or Forgejo instance answers /api/v1/version with a
// 404 or HTML, where /api/v1/user would answer 401 and read as "your token was
// rejected". The probe cannot separate the two failures on its own — Gitea
// validates any presented credential, so an invalid token 401s even /version —
// which is why an unauthorized answer there still reads as a token rejection.
func (a *Authenticator) Connect(ctx context.Context, rawURL, token string) (Instance, error) {
	base, host, err := normalizeInstanceURL(rawURL)
	if err != nil {
		return Instance{}, err
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return Instance{}, fmt.Errorf("gitea: token is empty")
	}

	client := a.newClient(base, token)
	version, err := client.Version(ctx)
	if errors.Is(err, sourcehttp.ErrUnauthorized) {
		return Instance{}, fmt.Errorf("gitea rejected the token")
	}
	if err != nil {
		return Instance{}, fmt.Errorf("%s does not answer as a Gitea or Forgejo instance: %w", base, err)
	}

	user, err := client.User(ctx)
	if errors.Is(err, sourcehttp.ErrUnauthorized) {
		return Instance{}, fmt.Errorf("gitea rejected the token")
	}
	if err != nil {
		return Instance{}, fmt.Errorf("validate token: %w", err)
	}
	if user.Login == "" {
		return Instance{}, fmt.Errorf("gitea: the token authenticates no account")
	}

	ref := credentials.Ref{Provider: Provider, Account: accountID(host, user.Login)}
	binding := Binding{URL: base, Login: user.Login, Version: version}

	previousToken, hadPrevious, err := a.previousToken(ref)
	if err != nil {
		return Instance{}, err
	}
	if err := a.creds.Set(ref, token); err != nil {
		return Instance{}, err
	}
	if err := a.instances.Set(ref, binding); err != nil {
		if rollbackErr := a.rollbackToken(ref, previousToken, hadPrevious); rollbackErr != nil {
			return Instance{}, errors.Join(err, fmt.Errorf("rollback gitea token: %w", rollbackErr))
		}
		return Instance{}, err
	}

	a.notify(ref)
	return Instance{
		Account: ref.Account,
		URL:     base,
		Login:   user.Login,
		Name:    user.FullName,
		Version: version,
	}, nil
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

func (a *Authenticator) previousToken(ref credentials.Ref) (token string, ok bool, err error) {
	token, err = a.creds.Get(ref)
	if err == nil {
		return token, true, nil
	}
	if errors.Is(err, credentials.ErrNotFound) {
		return "", false, nil
	}
	return "", false, fmt.Errorf("read existing gitea token: %w", err)
}

func (a *Authenticator) rollbackToken(ref credentials.Ref, previousToken string, hadPrevious bool) error {
	if hadPrevious {
		return a.creds.Set(ref, previousToken)
	}
	return a.creds.Delete(ref)
}

func (a *Authenticator) notify(ref credentials.Ref) {
	if a.onChange != nil {
		a.onChange(ref)
	}
}

// normalizeInstanceURL returns the base URL to store and the host identifying
// the instance. HTTPS is required except for loopback, and userinfo is
// rejected: a token must never be sent to a host with credentials baked into
// the URL.
//
// The path is preserved, trimmed of its trailing slash, because a Gitea served
// under a subpath (https://example.com/git) answers its API there.
func normalizeInstanceURL(raw string) (base, host string, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", fmt.Errorf("gitea: instance URL is required")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", "", fmt.Errorf("gitea: invalid instance URL: %w", err)
	}
	if parsed.Host == "" {
		return "", "", fmt.Errorf("gitea: instance URL has no host (did you include https://?)")
	}
	if parsed.User != nil {
		return "", "", fmt.Errorf("gitea: instance URL must not contain userinfo")
	}
	host = strings.ToLower(parsed.Host)
	switch parsed.Scheme {
	case "https":
	case "http":
		if !isLoopbackHost(host) {
			return "", "", fmt.Errorf("gitea: instance URL must use https (http is allowed only for loopback)")
		}
	default:
		return "", "", fmt.Errorf("gitea: instance URL must use https")
	}
	base = parsed.Scheme + "://" + parsed.Host + strings.TrimRight(parsed.Path, "/")
	return base, host, nil
}

func isLoopbackHost(host string) bool {
	name := host
	if hostOnly, _, splitErr := net.SplitHostPort(host); splitErr == nil {
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

// accountID is an account's credential account half: the instance host and the
// login the token authenticates as. Host alone would collide when someone holds
// two accounts on one server; login alone would collide across servers.
func accountID(host, login string) string {
	return fmt.Sprintf("%s-%s", host, login)
}
