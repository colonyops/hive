package grafana

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/grafana/client"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/sourcehttp"
	"github.com/colonyops/hive/internal/platform/credentials"
)

// defaultCooldown applies when a rate-limit response carried no reset time.
const defaultCooldown = time.Minute

// Fetchers hands out one fetcher per stack, created on first use. A fetcher owns
// that stack's cooldown — sourcehttp classifies a 429 but does not pause
// polling, so the provider must — and resolves URL and token freshly each poll,
// so connect, rotate, and disconnect take effect on the next tick.
type Fetchers struct {
	stacks          *StackStore
	creds           credentials.Store
	logger          zerolog.Logger
	newClient       clientFactory
	newOnCallClient onCallClientFactory

	mu    sync.Mutex
	byRef map[credentials.Ref]*fetcher
}

func NewFetchers(stacks *StackStore, creds credentials.Store, logger zerolog.Logger) *Fetchers {
	return &Fetchers{
		stacks: stacks,
		creds:  creds,
		logger: logger,
		newClient: func(base, token string) *client.Client {
			return client.NewClient(base, token, client.WithLogger(logger))
		},
		newOnCallClient: func(oncallBase, stackURL, token string) *client.OnCallClient {
			return client.NewOnCallClient(oncallBase, stackURL, token, client.WithLogger(logger))
		},
		byRef: map[credentials.Ref]*fetcher{},
	}
}

// For never fails: a fetcher for a not-yet-connected account resolves nothing until it connects.
func (f *Fetchers) For(ref credentials.Ref) *fetcher {
	f.mu.Lock()
	defer f.mu.Unlock()
	if fx, ok := f.byRef[ref]; ok {
		return fx
	}
	fx := &fetcher{
		ref:             ref,
		stacks:          f.stacks,
		resolve:         credentials.Bind(f.creds, ref),
		newClient:       f.newClient,
		newOnCallClient: f.newOnCallClient,
	}
	f.byRef[ref] = fx
	return fx
}

// InvalidateAll drops every stack's cached state — its cooldown, so a freshly
// connected account is not left waiting out one its predecessor incurred, and
// its OnCall URL, which belongs to the stack that was connected and not to
// whatever replaced it.
func (f *Fetchers) InvalidateAll() {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, fx := range f.byRef {
		fx.invalidate()
	}
}

// fetcher runs one stack's queries, gated by that stack's cooldown.
type fetcher struct {
	ref             credentials.Ref
	stacks          *StackStore
	resolve         credentials.Resolver
	newClient       clientFactory
	newOnCallClient onCallClientFactory

	mu            sync.Mutex
	cooldownUntil time.Time
	// oncallURL is resolved once per connected stack: it is plugin configuration
	// that only changes when the stack does, and InvalidateAll clears it.
	oncallURL string
}

func (fx *fetcher) Query(ctx context.Context, dsUID, promql string) (client.QueryResult, error) {
	c, err := fx.prepare()
	if err != nil {
		return client.QueryResult{}, err
	}
	result, err := c.Query(ctx, dsUID, promql)
	if err != nil {
		fx.noteError(err)
		return client.QueryResult{}, err
	}
	return result, nil
}

// Alerts lists the stack's firing alerts and the stack URL they came from —
// an alert item links back to the rule that raised it, and the base is resolved
// per poll, so returning it here is what keeps the link pointing at the stack
// the alert was actually read from.
func (fx *fetcher) Alerts(ctx context.Context, matchers []string) ([]client.Alert, string, error) {
	base, token, err := fx.stack()
	if err != nil {
		return nil, "", err
	}
	alerts, err := fx.newClient(base, token).Alerts(ctx, matchers)
	if err != nil {
		fx.noteError(err)
		return nil, "", err
	}
	return alerts, base, nil
}

// AlertGroups lists IRM alert groups, resolving the stack's OnCall host on the
// first call. The extra request is the cost of OnCall not living on the stack;
// it is paid once per connected stack, not once per poll.
func (fx *fetcher) AlertGroups(ctx context.Context, query client.AlertGroupQuery) ([]client.AlertGroup, error) {
	base, token, err := fx.stack()
	if err != nil {
		return nil, err
	}
	oncallBase, err := fx.resolveOnCallURL(ctx, base, token)
	if err != nil {
		fx.noteError(err)
		return nil, err
	}
	groups, err := fx.newOnCallClient(oncallBase, base, token).AlertGroups(ctx, query)
	if err != nil {
		fx.noteError(err)
		return nil, err
	}
	return groups, nil
}

// resolveOnCallURL returns the cached OnCall base, asking the stack's IRM
// plugin for it the first time.
func (fx *fetcher) resolveOnCallURL(ctx context.Context, base, token string) (string, error) {
	fx.mu.Lock()
	cached := fx.oncallURL
	fx.mu.Unlock()
	if cached != "" {
		return cached, nil
	}
	resolved, err := fx.newClient(base, token).OnCallURL(ctx)
	if err != nil {
		return "", fmt.Errorf("grafana %s: %w", fx.ref, err)
	}
	fx.mu.Lock()
	fx.oncallURL = resolved
	fx.mu.Unlock()
	return resolved, nil
}

// prepare returns a client, or an error when the stack is in cooldown, not
// connected, or has no resolvable token — the states a poll must not reach the
// network in.
func (fx *fetcher) prepare() (*client.Client, error) {
	base, token, err := fx.stack()
	if err != nil {
		return nil, err
	}
	return fx.newClient(base, token), nil
}

// stack resolves the base URL and token a request goes out with, refusing in
// the three states a poll must not reach the network in.
func (fx *fetcher) stack() (base, token string, err error) {
	if until, cooling := fx.inCooldown(); cooling {
		return "", "", fmt.Errorf("grafana %s: %w until %s", fx.ref, sourcehttp.ErrRateLimited, until.Format(time.RFC3339))
	}
	entry, err := fx.stacks.Get(fx.ref)
	if err != nil {
		return "", "", fmt.Errorf("grafana %s: reading stack URL: %w", fx.ref, err)
	}
	base = entry.URL
	if base == "" {
		return "", "", fmt.Errorf("grafana %s: stack is not connected", fx.ref)
	}
	token, err = fx.resolve()
	if err != nil {
		return "", "", fmt.Errorf("grafana %s: resolving token: %w", fx.ref, err)
	}
	if token == "" {
		return "", "", fmt.Errorf("grafana %s: %w", fx.ref, sourcehttp.ErrUnauthorized)
	}
	return base, token, nil
}

// noteError arms the cooldown when the failure was a rate limit, so the next
// tick waits out the server's reset instead of hammering it.
func (fx *fetcher) noteError(err error) {
	if rateLimit, ok := errors.AsType[*sourcehttp.RateLimitError](err); ok {
		fx.enterCooldown(rateLimit.ResetAt)
	}
}

func (fx *fetcher) inCooldown() (time.Time, bool) {
	fx.mu.Lock()
	defer fx.mu.Unlock()
	if fx.cooldownUntil.IsZero() || time.Now().After(fx.cooldownUntil) {
		return time.Time{}, false
	}
	return fx.cooldownUntil, true
}

func (fx *fetcher) enterCooldown(until time.Time) {
	if until.IsZero() {
		until = time.Now().Add(defaultCooldown)
	}
	fx.mu.Lock()
	fx.cooldownUntil = until
	fx.mu.Unlock()
}

func (fx *fetcher) invalidate() {
	fx.mu.Lock()
	fx.cooldownUntil = time.Time{}
	fx.oncallURL = ""
	fx.mu.Unlock()
}
