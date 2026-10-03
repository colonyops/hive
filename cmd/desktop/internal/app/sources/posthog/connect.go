package posthog

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/posthog/client"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/sourcehttp"
	"github.com/colonyops/hive/internal/platform/credentials"
)

// Project is one project the connect flow can offer or has connected.
type Project struct {
	Account string `json:"account"`
	URL     string `json:"url"`
	ID      int    `json:"id"`
	Name    string `json:"name"`
}

// clientFactory is a field, not a direct call, so a test can stub the client without the network.
type clientFactory func(base, token string) *client.Client

// Authenticator connects and disconnects PostHog projects. Connecting is two
// steps rather than one because a personal API key spans projects: Projects
// validates the key and lists what it can see, then Connect binds one of them
// to its own credential ref. Connecting a second project repeats the second
// step with the same key.
type Authenticator struct {
	creds     credentials.Store
	projects  *ProjectStore
	newClient clientFactory
	onChange  func(credentials.Ref)
	logger    zerolog.Logger
}

func NewAuthenticator(creds credentials.Store, projects *ProjectStore, logger zerolog.Logger, onChange func(credentials.Ref)) *Authenticator {
	return &Authenticator{
		creds:    creds,
		projects: projects,
		newClient: func(base, token string) *client.Client {
			return client.NewClient(base, token, client.WithLogger(logger))
		},
		onChange: onChange,
		logger:   logger,
	}
}

// Projects validates the key and lists what it can reach, without persisting
// anything: the picker this feeds is shown before the user has committed to a
// project, and a rejected key must leave nothing behind.
func (a *Authenticator) Projects(ctx context.Context, rawURL, token string) ([]Project, error) {
	base, _, err := normalizeHostURL(rawURL)
	if err != nil {
		return nil, err
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, fmt.Errorf("posthog: API key is empty")
	}

	found, err := a.newClient(base, token).Projects(ctx)
	if err != nil {
		return nil, describeAuthError(err)
	}
	if len(found) == 0 {
		return nil, fmt.Errorf("posthog: the key reached %s but can see no projects (check its project scopes)", base)
	}

	out := make([]Project, 0, len(found))
	for _, project := range found {
		out = append(out, Project{URL: base, ID: project.ID, Name: project.Name})
	}
	return out, nil
}

// Connect binds one project to a credential. It re-validates the key and
// confirms the project is one this key can actually see, so a hand-supplied
// project id cannot store a binding that will only fail at poll time.
func (a *Authenticator) Connect(ctx context.Context, rawURL, token string, projectID int) (Project, error) {
	base, host, err := normalizeHostURL(rawURL)
	if err != nil {
		return Project{}, err
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return Project{}, fmt.Errorf("posthog: API key is empty")
	}
	if projectID <= 0 {
		return Project{}, fmt.Errorf("posthog: a project must be selected")
	}

	found, err := a.newClient(base, token).Projects(ctx)
	if err != nil {
		return Project{}, describeAuthError(err)
	}
	name, ok := projectName(found, projectID)
	if !ok {
		return Project{}, fmt.Errorf("posthog: the key cannot see project %d on %s", projectID, base)
	}

	ref := credentials.Ref{Provider: Provider, Account: accountID(host, projectID)}
	previousToken, hadPrevious, err := a.previousToken(ref)
	if err != nil {
		return Project{}, err
	}
	if err := a.creds.Set(ref, token); err != nil {
		return Project{}, err
	}
	binding := Binding{URL: base, ProjectID: projectID, Name: name}
	if err := a.projects.Set(ref, binding); err != nil {
		if rollbackErr := a.rollbackToken(ref, previousToken, hadPrevious); rollbackErr != nil {
			return Project{}, errors.Join(err, fmt.Errorf("rollback posthog key: %w", rollbackErr))
		}
		return Project{}, err
	}
	a.notify(ref)
	return Project{Account: ref.Account, URL: base, ID: projectID, Name: name}, nil
}

// Disconnect is idempotent: an account with nothing stored disconnects cleanly.
func (a *Authenticator) Disconnect(ctx context.Context, account string) error {
	ref := credentials.Ref{Provider: Provider, Account: strings.TrimSpace(account)}
	if err := ref.Validate(); err != nil {
		return err
	}
	credErr := a.creds.Delete(ref)
	projectErr := a.projects.Delete(ref)
	a.notify(ref)
	if credErr != nil {
		return credErr
	}
	return projectErr
}

func (a *Authenticator) previousToken(ref credentials.Ref) (token string, ok bool, err error) {
	token, err = a.creds.Get(ref)
	if err == nil {
		return token, true, nil
	}
	if errors.Is(err, credentials.ErrNotFound) {
		return "", false, nil
	}
	return "", false, fmt.Errorf("read existing posthog key: %w", err)
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

func projectName(projects []client.Project, id int) (string, bool) {
	for _, project := range projects {
		if project.ID == id {
			return project.Name, true
		}
	}
	return "", false
}

// describeAuthError turns the taxonomy into something a connect dialog can
// show. A 401 here almost always means the key lacks project:read rather than
// that it is invalid, because that is the one scope this call needs.
func describeAuthError(err error) error {
	if errors.Is(err, sourcehttp.ErrUnauthorized) {
		return fmt.Errorf("posthog rejected the API key (it needs the project:read scope)")
	}
	return fmt.Errorf("list projects: %w", err)
}

// ingestHosts maps PostHog's event-ingestion hostnames onto the API hostname
// for the same region. They are the hosts printed in every install snippet, so
// they are what a user pastes — but they serve only the public capture
// endpoints and answer /api/projects/ with a 401, which reads as "bad key"
// rather than "wrong host". Rewriting is friendlier than rejecting, and there
// is no case where an API call should go to an ingestion host.
var ingestHosts = map[string]string{
	"us.i.posthog.com":  "us.posthog.com",
	"eu.i.posthog.com":  "eu.posthog.com",
	"app.posthog.com":   "us.posthog.com",
	"i.posthog.com":     "us.posthog.com",
	"app.i.posthog.com": "us.posthog.com",
}

// normalizeHostURL returns the base URL to store and the host identifying the
// instance. HTTPS is required except for loopback, and userinfo is rejected: a
// key must never be sent to a host with credentials baked into the URL.
func normalizeHostURL(raw string) (base, host string, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", fmt.Errorf("posthog: host URL is required")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", "", fmt.Errorf("posthog: invalid host URL: %w", err)
	}
	if u.Host == "" {
		return "", "", fmt.Errorf("posthog: host URL has no host (did you include https://?)")
	}
	if u.User != nil {
		return "", "", fmt.Errorf("posthog: host URL must not contain userinfo")
	}
	host = strings.ToLower(u.Host)
	switch u.Scheme {
	case "https":
	case "http":
		if !isLoopbackHost(host) {
			return "", "", fmt.Errorf("posthog: host URL must use https (http is allowed only for loopback)")
		}
	default:
		return "", "", fmt.Errorf("posthog: host URL must use https")
	}

	path := strings.TrimRight(u.Path, "/")
	if api, ok := ingestHosts[host]; ok {
		// The ingestion hosts are cloud-only and serve nothing under a path
		// prefix, so a pasted path is part of the mistake being corrected.
		host, path = api, ""
	}
	base = u.Scheme + "://" + host + path
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

// accountID is a project's credential account: its host and the project the
// key is bound to. Host alone would collide across projects on one instance,
// which is the multi-project routing case.
func accountID(host string, projectID int) string {
	return host + "-" + strconv.Itoa(projectID)
}
