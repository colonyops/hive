package grafana

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/grafana/client"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/sourcehttp"
	"github.com/colonyops/hive/internal/platform/credentials"
)

type Stack struct {
	Account string `json:"account"`
	URL     string `json:"url"`
	OrgID   int    `json:"orgID"`
	OrgName string `json:"orgName"`
}

// clientFactory is a field, not a direct call, so a test can stub the client without the network.
type clientFactory func(base, token string) *client.Client

// onCallClientFactory is the same for the OnCall API, which answers on its own
// host and so needs the stack URL alongside the base it is reached at.
type onCallClientFactory func(oncallBase, stackURL, token string) *client.OnCallClient

// Authenticator connects and disconnects Grafana stacks. Unlike the GitHub
// connector there is no device flow or polled status: a stack is connected by
// validating a pasted URL and service-account token once.
type Authenticator struct {
	creds     credentials.Store
	stacks    *StackStore
	newClient clientFactory
	onChange  func(credentials.Ref)
	logger    zerolog.Logger
}

func NewAuthenticator(creds credentials.Store, stacks *StackStore, logger zerolog.Logger, onChange func(credentials.Ref)) *Authenticator {
	return &Authenticator{
		creds:  creds,
		stacks: stacks,
		newClient: func(base, token string) *client.Client {
			return client.NewClient(base, token, client.WithLogger(logger))
		},
		onChange: onChange,
		logger:   logger,
	}
}

// Connect validates the URL and token before persisting either, so a rejected
// paste leaves nothing behind.
func (a *Authenticator) Connect(ctx context.Context, rawURL, token string) (Stack, error) {
	base, host, err := normalizeStackURL(rawURL)
	if err != nil {
		return Stack{}, err
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return Stack{}, fmt.Errorf("grafana: token is empty")
	}

	org, err := a.newClient(base, token).ValidateToken(ctx)
	if errors.Is(err, sourcehttp.ErrUnauthorized) {
		return Stack{}, fmt.Errorf("grafana rejected the token")
	}
	if err != nil {
		return Stack{}, fmt.Errorf("validate token: %w", err)
	}

	ref := credentials.Ref{Provider: Provider, Account: accountID(host, org.ID)}
	previousToken, hadPrevious, err := a.previousToken(ref)
	if err != nil {
		return Stack{}, err
	}
	if err := a.creds.Set(ref, token); err != nil {
		return Stack{}, err
	}
	if err := a.stacks.Set(ref, stackEntry{URL: base}); err != nil {
		if rollbackErr := a.rollbackToken(ref, previousToken, hadPrevious); rollbackErr != nil {
			return Stack{}, errors.Join(err, fmt.Errorf("rollback grafana token: %w", rollbackErr))
		}
		return Stack{}, err
	}
	a.notify(ref)
	return Stack{Account: ref.Account, URL: base, OrgID: org.ID, OrgName: org.Name}, nil
}

// Disconnect is idempotent: an account with nothing stored disconnects cleanly.
func (a *Authenticator) Disconnect(ctx context.Context, account string) error {
	ref := credentials.Ref{Provider: Provider, Account: strings.TrimSpace(account)}
	if err := ref.Validate(); err != nil {
		return err
	}
	credErr := a.creds.Delete(ref)
	stackErr := a.stacks.Delete(ref)
	a.notify(ref)
	if credErr != nil {
		return credErr
	}
	return stackErr
}

func (a *Authenticator) previousToken(ref credentials.Ref) (token string, ok bool, err error) {
	token, err = a.creds.Get(ref)
	if err == nil {
		return token, true, nil
	}
	if errors.Is(err, credentials.ErrNotFound) {
		return "", false, nil
	}
	return "", false, fmt.Errorf("read existing grafana token: %w", err)
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

// normalizeStackURL returns the base URL to store and the host identifying the
// stack. HTTPS is required except for loopback, and userinfo is rejected: a
// token must never be sent to a host with credentials baked into the URL.
func normalizeStackURL(raw string) (base, host string, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", fmt.Errorf("grafana: stack URL is required")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", "", fmt.Errorf("grafana: invalid stack URL: %w", err)
	}
	if u.Host == "" {
		return "", "", fmt.Errorf("grafana: stack URL has no host (did you include https://?)")
	}
	if u.User != nil {
		return "", "", fmt.Errorf("grafana: stack URL must not contain userinfo")
	}
	host = strings.ToLower(u.Host)
	switch u.Scheme {
	case "https":
	case "http":
		if !isLoopbackHost(host) {
			return "", "", fmt.Errorf("grafana: stack URL must use https (http is allowed only for loopback)")
		}
	default:
		return "", "", fmt.Errorf("grafana: stack URL must use https")
	}
	base = u.Scheme + "://" + u.Host + strings.TrimRight(u.Path, "/")
	return base, host, nil
}

func isLoopbackHost(host string) bool {
	h := host
	if hostOnly, _, splitErr := net.SplitHostPort(host); splitErr == nil {
		h = hostOnly
	}
	h = strings.Trim(h, "[]")
	if strings.EqualFold(h, "localhost") {
		return true
	}
	if ip := net.ParseIP(h); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

// accountID is a stack's credential account: its host and the org the token
// authenticates against. Host alone would collide across orgs on one stack.
func accountID(host string, orgID int) string {
	return fmt.Sprintf("%s-%d", host, orgID)
}
