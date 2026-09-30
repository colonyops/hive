package posthog

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/desktop/internal/app/credentials"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/posthog/client"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/sourcehttp"
)

// defaultCooldown applies when a rate-limit response carried no reset time.
// PostHog's private endpoints are limited per minute and per hour, and it does
// not always send Retry-After, so the fallback is a minute rather than a guess
// at the hourly window.
const defaultCooldown = time.Minute

// Fetchers hands out one fetcher per connected project, created on first use.
// A fetcher owns that project's cooldown — sourcehttp classifies a 429 but
// does not pause polling, so the provider must — and resolves binding and key
// freshly each poll, so connect, rotate, and disconnect take effect on the
// next tick.
type Fetchers struct {
	projects  *ProjectStore
	creds     credentials.Store
	logger    zerolog.Logger
	newClient clientFactory

	mu    sync.Mutex
	byRef map[credentials.Ref]*fetcher
}

func NewFetchers(projects *ProjectStore, creds credentials.Store, logger zerolog.Logger) *Fetchers {
	return &Fetchers{
		projects: projects,
		creds:    creds,
		logger:   logger,
		newClient: func(base, token string) *client.Client {
			return client.NewClient(base, token, client.WithLogger(logger))
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
		ref:       ref,
		projects:  f.projects,
		resolve:   credentials.Bind(f.creds, ref),
		newClient: f.newClient,
		logger:    f.logger,
	}
	f.byRef[ref] = fx
	return fx
}

// InvalidateAll clears every project's cooldown so a freshly connected account
// is not left waiting out a cooldown its predecessor incurred.
func (f *Fetchers) InvalidateAll() {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, fx := range f.byRef {
		fx.clearCooldown()
	}
}

// fetcher runs one project's queries, gated by that project's cooldown.
type fetcher struct {
	ref       credentials.Ref
	projects  *ProjectStore
	resolve   credentials.Resolver
	newClient clientFactory
	logger    zerolog.Logger

	mu            sync.Mutex
	cooldownUntil time.Time
}

// Issues runs the error-tracking issue query. It returns the project binding
// alongside the issues because deep links are built from the host and project
// id, and re-reading the store to get them would race with a disconnect.
func (fx *fetcher) Issues(ctx context.Context, req client.IssuesRequest) ([]client.Issue, Binding, error) {
	c, binding, err := fx.prepare()
	if err != nil {
		return nil, Binding{}, err
	}
	issues, err := c.Issues(ctx, binding.ProjectID, req)
	if err != nil {
		fx.noteError(err)
		return nil, Binding{}, err
	}
	return issues, binding, nil
}

func (fx *fetcher) Alerts(ctx context.Context) ([]client.Alert, Binding, error) {
	c, binding, err := fx.prepare()
	if err != nil {
		return nil, Binding{}, err
	}
	alerts, truncated, err := c.Alerts(ctx, binding.ProjectID)
	if err != nil {
		fx.noteError(err)
		return nil, Binding{}, err
	}
	if truncated {
		// A silently short list reads as "these are all my alerts". Say so
		// instead; the fix is PostHog-side (fewer alerts) or a paged fetch.
		fx.logger.Warn().
			Ctx(ctx).
			Str("account", fx.ref.Account).
			Int("fetched", len(alerts)).
			Msg("posthog project has more alerts than one page; the tail is not polled")
	}
	return alerts, binding, nil
}

// prepare returns a client and the project binding, or an error when the
// project is in cooldown, not connected, or has no resolvable key — the states
// a poll must not reach the network in.
func (fx *fetcher) prepare() (*client.Client, Binding, error) {
	if until, cooling := fx.inCooldown(); cooling {
		return nil, Binding{}, fmt.Errorf("posthog %s: %w until %s", fx.ref, sourcehttp.ErrRateLimited, until.Format(time.RFC3339))
	}
	binding, err := fx.projects.Get(fx.ref)
	if err != nil {
		return nil, Binding{}, fmt.Errorf("posthog %s: reading project binding: %w", fx.ref, err)
	}
	if binding.URL == "" || binding.ProjectID == 0 {
		return nil, Binding{}, fmt.Errorf("posthog %s: project is not connected", fx.ref)
	}
	token, err := fx.resolve()
	if err != nil {
		return nil, Binding{}, fmt.Errorf("posthog %s: resolving API key: %w", fx.ref, err)
	}
	if token == "" {
		return nil, Binding{}, fmt.Errorf("posthog %s: %w", fx.ref, sourcehttp.ErrUnauthorized)
	}
	return fx.newClient(binding.URL, token), binding, nil
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

func (fx *fetcher) clearCooldown() {
	fx.mu.Lock()
	fx.cooldownUntil = time.Time{}
	fx.mu.Unlock()
}
