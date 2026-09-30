package app

import (
	"context"
	"net"
	"strconv"

	"github.com/colonyops/hive/cmd/desktop/internal/app/actions"
	"github.com/colonyops/hive/cmd/desktop/internal/app/events"
	"github.com/colonyops/hive/cmd/desktop/internal/app/prompts"
	"github.com/colonyops/hive/cmd/desktop/internal/app/settings"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/gitea"
	ghsource "github.com/colonyops/hive/cmd/desktop/internal/app/sources/github"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/grafana"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/posthog"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/webhook"
)

// The services in this file are thin. They exist so App is the single
// entry point rather than a partial one: an adapter that has to reach past
// the facade for a job list will reach past it for something else next.

// ActionsService owns the editable actions catalog.
type ActionsService struct {
	catalog *actions.ActionStore
	events  *events.Bus
}

func newActionsService(catalog *actions.ActionStore, bus *events.Bus) *ActionsService {
	return &ActionsService{catalog: catalog, events: bus}
}

// publish reads the count after mutation so the event carries the live
// catalog size.
func (s *ActionsService) publish(ctx context.Context) {
	s.events.Publish(ctx, events.ActionsUpdated{Count: len(s.catalog.List())})
}

// List returns the effective last-good catalog plus a current parse error, if
// a hand edit made the latest actions.yml invalid.
func (s *ActionsService) List(context.Context) actions.EditableCatalog {
	return s.catalog.ListEditable()
}

func (s *ActionsService) Create(ctx context.Context, a actions.EditableAction) (actions.EditableAction, error) {
	out, err := s.catalog.Create(a)
	if err != nil {
		return out, Wrap(err, KindInvalid, "creating action %q", a.ID)
	}
	s.publish(ctx)
	return out, nil
}

func (s *ActionsService) Update(ctx context.Context, id string, a actions.EditableAction) (actions.EditableAction, error) {
	out, err := s.catalog.Update(ctx, id, a)
	if err != nil {
		return out, Wrap(err, KindInvalid, "updating action %q", id)
	}
	s.publish(ctx)
	return out, nil
}

// Reorder persists the catalog order. A stale list — a hand edit added or
// removed an action meanwhile — is rejected so the caller reloads, which is
// the caller's view having moved rather than a failure.
func (s *ActionsService) Reorder(ctx context.Context, ids []string) error {
	if err := s.catalog.Reorder(ids); err != nil {
		return Wrap(err, KindConflict, "reordering the actions catalog")
	}
	s.publish(ctx)
	return nil
}

func (s *ActionsService) Delete(ctx context.Context, id string) error {
	if err := s.catalog.Delete(ctx, id); err != nil {
		return Wrap(err, KindConflict, "deleting action %q", id)
	}
	s.publish(ctx)
	return nil
}

// The launchers half of actions.yml. It is served by this service rather than
// one of its own because it is the same file, the same store and the same wake
// on change — the launchers are simply the other list in it (ADR launchers-are-their-own-list-in-actions-yml). List
// answers for both, so there is no LaunchersList here.

func (s *ActionsService) CreateLauncher(ctx context.Context, l actions.Launcher) (actions.Launcher, error) {
	out, err := s.catalog.CreateLauncher(l)
	if err != nil {
		return out, Wrap(err, KindInvalid, "creating launcher %q", l.ID)
	}
	s.publish(ctx)
	return out, nil
}

func (s *ActionsService) UpdateLauncher(ctx context.Context, id string, l actions.Launcher) (actions.Launcher, error) {
	out, err := s.catalog.UpdateLauncher(id, l)
	if err != nil {
		return out, Wrap(err, KindInvalid, "updating launcher %q", id)
	}
	s.publish(ctx)
	return out, nil
}

func (s *ActionsService) DeleteLauncher(ctx context.Context, id string) error {
	if err := s.catalog.DeleteLauncher(id); err != nil {
		return Wrap(err, KindInvalid, "deleting launcher %q", id)
	}
	s.publish(ctx)
	return nil
}

// GitHubService wraps the GitHub connector's connection with context
// threading. It is provider-specific because acquisition is: the device flow
// is GitHub's, and a connector that takes a pasted API token needs none of
// these methods. Enumerating what is connected, across every connector, is
// the Integrations surface's job rather than this one's.
type GitHubService struct{ conn ghsource.Connection }

func newGitHubService(conn ghsource.Connection) *GitHubService {
	return &GitHubService{conn: conn}
}

func (s *GitHubService) Status(ctx context.Context) ghsource.ConnectionStatus {
	return s.conn.Status(ctx)
}

func (s *GitHubService) StartDeviceFlow(ctx context.Context) (ghsource.DeviceFlowInfo, error) {
	info, err := s.conn.StartDeviceFlow(ctx)
	return info, Wrap(err, KindUnauthenticated, "starting the device flow")
}

func (s *GitHubService) CancelDeviceFlow(context.Context) { s.conn.CancelDeviceFlow() }

func (s *GitHubService) SetToken(ctx context.Context, token string) (ghsource.ConnectionStatus, error) {
	status, err := s.conn.SetToken(ctx, token)
	return status, Wrap(err, KindUnauthenticated, "accepting the token")
}

func (s *GitHubService) Disconnect(context.Context) error {
	return Wrap(s.conn.Disconnect(), KindInternal, "disconnecting GitHub")
}

// GrafanaService wraps the Grafana stack auth with context threading and error
// mapping. Leaner than GitHubService: no device flow, no polled status.
type GrafanaService struct{ auth *grafana.Authenticator }

func newGrafanaService(auth *grafana.Authenticator) *GrafanaService {
	return &GrafanaService{auth: auth}
}

func (s *GrafanaService) Connect(ctx context.Context, url, token string) (grafana.Stack, error) {
	stack, err := s.auth.Connect(ctx, url, token)
	return stack, Wrap(err, KindUnauthenticated, "connecting to Grafana")
}

func (s *GrafanaService) Disconnect(ctx context.Context, account string) error {
	return Wrap(s.auth.Disconnect(ctx, account), KindInternal, "disconnecting Grafana")
}

// PostHogService wraps the PostHog project auth. Connecting is two calls
// rather than one because a personal API key spans projects: Projects lists
// what the key can reach so the user can pick, Connect binds the one they
// picked.
type PostHogService struct{ auth *posthog.Authenticator }

func newPostHogService(auth *posthog.Authenticator) *PostHogService {
	return &PostHogService{auth: auth}
}

func (s *PostHogService) Projects(ctx context.Context, url, token string) ([]posthog.Project, error) {
	projects, err := s.auth.Projects(ctx, url, token)
	return projects, Wrap(err, KindUnauthenticated, "listing PostHog projects")
}

func (s *PostHogService) Connect(ctx context.Context, url, token string, projectID int) (posthog.Project, error) {
	project, err := s.auth.Connect(ctx, url, token, projectID)
	return project, Wrap(err, KindUnauthenticated, "connecting to PostHog")
}

func (s *PostHogService) Disconnect(ctx context.Context, account string) error {
	return Wrap(s.auth.Disconnect(ctx, account), KindInternal, "disconnecting PostHog")
}

// GiteaService wraps the Gitea instance auth. Connecting is one call: a token
// belongs to exactly one account on exactly one instance, so there is nothing
// to enumerate and pick between the way a PostHog key's projects are.
type GiteaService struct{ auth *gitea.Authenticator }

func newGiteaService(auth *gitea.Authenticator) *GiteaService {
	return &GiteaService{auth: auth}
}

func (s *GiteaService) Connect(ctx context.Context, url, token string) (gitea.Instance, error) {
	instance, err := s.auth.Connect(ctx, url, token)
	return instance, Wrap(err, KindUnauthenticated, "connecting to Gitea")
}

func (s *GiteaService) Disconnect(ctx context.Context, account string) error {
	return Wrap(s.auth.Disconnect(ctx, account), KindInternal, "disconnecting Gitea")
}

// PromptsService owns the paste-ready LLM prompts. Prompt text lives in
// app/prompts; this resolves the install-specific environment they render
// against.
type PromptsService struct {
	paths    settings.Paths
	settings *settings.Store
	webhooks *WebhookService
}

func newPromptsService(paths settings.Paths, settingsStore *settings.Store, webhooks *WebhookService) *PromptsService {
	return &PromptsService{paths: paths, settings: settingsStore, webhooks: webhooks}
}

// service is rebuilt per call rather than cached: the paths and webhook state
// it embeds are user-editable while the app runs, and a cached service would
// keep handing out a stale port after the listener rebinds.
func (s *PromptsService) service(ctx context.Context) (*prompts.Service, error) {
	env := prompts.Env{
		FlowsDir:           s.paths.FlowsDir,
		ActionsPath:        s.paths.ActionsPath,
		SettingsPath:       s.paths.SettingsPath,
		AgentWorkspacesDir: s.paths.AgentWorkspacesDir,
	}
	if _, port := s.webhooks.Endpoint(ctx); port > 0 {
		env.WebhookBaseURL = WebhookBaseURLAt(s.webhooks.Host(), port)
		env.MCPEndpoint = MCPEndpointAt(s.webhooks.Host(), port)
	}
	// A settings read failure must not take the prompts page down with it:
	// every other prompt is still correct, so fall back to reporting the
	// listener as enabled and let the webhook settings pane surface the error.
	if cfg, err := s.settings.Effective(); err == nil {
		env.WebhookEnabled = cfg.HTTP.Enabled
		env.MCPEnabled = cfg.HTTP.Enabled
	} else {
		env.WebhookEnabled = false
		env.MCPEnabled = false
	}
	svc, err := prompts.New(env)
	return svc, Wrap(err, KindInternal, "building the prompt catalog")
}

// Catalog returns every prompt that belongs in a settings listing, rendered
// against this install.
func (s *PromptsService) Catalog(ctx context.Context, input prompts.Input) ([]prompts.Prompt, error) {
	svc, err := s.service(ctx)
	if err != nil {
		return nil, err
	}
	return svc.Catalog(input), nil
}

// Render returns one prompt by id, including the context-scoped prompts
// Catalog omits.
func (s *PromptsService) Render(ctx context.Context, id string, input prompts.Input) (prompts.Prompt, error) {
	svc, err := s.service(ctx)
	if err != nil {
		return prompts.Prompt{}, err
	}
	prompt, err := svc.Render(id, input)
	if err != nil {
		return prompts.Prompt{}, Wrap(err, KindNotFound, "rendering prompt %q", id)
	}
	return prompt, nil
}

// WebhookBaseURLAt is the URL prefix sources.webhook node paths are served
// under, for a validated listener host. It lives here rather than in the
// adapter because the prompt text embeds it, and a prompt is core-owned.
func WebhookBaseURLAt(host string, port int) string {
	return "http://" + net.JoinHostPort(host, strconv.Itoa(port)) + webhook.PathPrefix
}

// HTTPBaseURLAt is the loopback server's base URL, which the catalogue joins
// with each app-hosted entry's RuntimePath to resolve its live address.
func HTTPBaseURLAt(host string, port int) string {
	return "http://" + net.JoinHostPort(host, strconv.Itoa(port))
}

// MCPEndpointAt is the URL of the agent MCP server — the same loopback server
// the webhook listener uses (ADR agent-http-api), at the /mcp path (ADR mcp-replaces-the-agent-facing-http-api). It lives
// here, not in the adapter, because the prompt text embeds it; the /mcp
// literal avoids an import cycle back into the adapter.
func MCPEndpointAt(host string, port int) string {
	return HTTPBaseURLAt(host, port) + "/mcp"
}
