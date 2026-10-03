package gitea

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"golang.org/x/sync/errgroup"

	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/gitea/giteaclient"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/sourcehttp"
	"github.com/colonyops/hive/internal/platform/credentials"
)

// defaultCooldown applies when a rate-limited response carried no reset time.
// A self-hosted Gitea usually has no rate limiting at all; where a reverse
// proxy imposes one it rarely says when it lifts, so the fallback is a minute
// rather than a guess.
const defaultCooldown = time.Minute

// absenceConcurrency bounds the parallel single-issue lookups one absence
// confirmation runs. Gitea has no batched query — GitHub's absence check is one
// GraphQL request, this one is a request per item — so the fan-out is capped to
// keep a tick from opening dozens of connections to someone's self-hosted
// instance.
const absenceConcurrency = 4

// clientFactory is a field, not a direct call, so a test can stub the client
// without the network.
type clientFactory func(base, token string) *giteaclient.Client

// Fetchers hands out one fetcher per connected account, created on first use.
// A fetcher owns that account's cooldown — sourcehttp classifies a 429 but does
// not pause polling, so the provider must — and resolves its host binding and
// token freshly on each poll, so connect, rotate and disconnect take effect on
// the next tick.
type Fetchers struct {
	instances *InstanceStore
	creds     credentials.Store
	logger    zerolog.Logger
	newClient clientFactory

	mu    sync.Mutex
	byRef map[credentials.Ref]*fetcher
}

func NewFetchers(instances *InstanceStore, creds credentials.Store, logger zerolog.Logger) *Fetchers {
	return &Fetchers{
		instances: instances,
		creds:     creds,
		logger:    logger,
		newClient: func(base, token string) *giteaclient.Client {
			return giteaclient.NewClient(base, token, giteaclient.WithLogger(logger))
		},
		byRef: map[credentials.Ref]*fetcher{},
	}
}

// For never fails: a fetcher for a not-yet-connected account resolves nothing
// until it connects.
func (f *Fetchers) For(ref credentials.Ref) *fetcher {
	f.mu.Lock()
	defer f.mu.Unlock()
	if fx, ok := f.byRef[ref]; ok {
		return fx
	}
	fx := &fetcher{
		ref:       ref,
		instances: f.instances,
		resolve:   credentials.Bind(f.creds, ref),
		newClient: f.newClient,
		logger:    f.logger,
	}
	f.byRef[ref] = fx
	return fx
}

// InvalidateAll clears every account's cooldown so a freshly connected account
// is not left waiting out a cooldown its predecessor incurred.
func (f *Fetchers) InvalidateAll() {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, fx := range f.byRef {
		fx.clearCooldown()
	}
}

// fetcher runs one account's requests, gated by that account's cooldown.
type fetcher struct {
	ref       credentials.Ref
	instances *InstanceStore
	resolve   credentials.Resolver
	newClient clientFactory
	logger    zerolog.Logger

	mu            sync.Mutex
	cooldownUntil time.Time
}

// Search runs a node's search and returns its snapshot.
//
// Several requests are a union over one snapshot, so a partial failure fails
// the whole call: a successful Produce is authoritative, and returning the
// searches that did answer would archive every item that lived only in the one
// that did not.
func (fx *fetcher) Search(ctx context.Context, cfg *Config) ([]Item, error) {
	client, err := fx.prepare()
	if err != nil {
		return nil, err
	}

	requests := cfg.searchRequests()
	results := make([][]Item, len(requests))
	for i, req := range requests {
		issues, err := client.SearchIssues(ctx, req)
		if err != nil {
			fx.noteError(err)
			return nil, fmt.Errorf("gitea %s: searching: %w", fx.ref, err)
		}
		results[i] = searchItems(issues)
	}
	return mergeItems(results, cfg.effectiveLimit()), nil
}

// Notifications drains the account's notification inbox.
func (fx *fetcher) Notifications(ctx context.Context, limit int) ([]Item, error) {
	client, err := fx.prepare()
	if err != nil {
		return nil, err
	}
	threads, err := client.Notifications(ctx, limit)
	if err != nil {
		fx.noteError(err)
		return nil, fmt.Errorf("gitea %s: reading notifications: %w", fx.ref, err)
	}
	return notificationItems(threads), nil
}

// itemState is the current lifecycle of one item that left a snapshot. Found is
// false when the item is gone or its repository is no longer readable, which is
// a verdict rather than a failure.
type itemState struct {
	Found     bool
	State     string
	Title     string
	URL       string
	UpdatedAt int64
}

// itemRef addresses one item for a state lookup.
type itemRef struct {
	Repo string
	Num  int
}

// ItemStates looks up the current state of items that left a source's result
// set. Results are index-parallel to refs; a ref that cannot be addressed comes
// back zero-valued (Found false) rather than failing the batch.
func (fx *fetcher) ItemStates(ctx context.Context, refs []itemRef) ([]itemState, error) {
	out := make([]itemState, len(refs))
	if len(refs) == 0 {
		return out, nil
	}
	client, err := fx.prepare()
	if err != nil {
		return out, err
	}

	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(absenceConcurrency)
	for i, ref := range refs {
		owner, name, ok := strings.Cut(ref.Repo, "/")
		if !ok || owner == "" || name == "" || ref.Num <= 0 {
			continue
		}
		group.Go(func() error {
			issue, found, err := client.Issue(groupCtx, owner, name, ref.Num)
			if err != nil {
				return err
			}
			if !found {
				return nil
			}
			// Distinct index per goroutine, so the slice needs no lock.
			out[i] = itemState{
				Found:     true,
				State:     issue.LifecycleState(),
				Title:     issue.Title,
				URL:       issue.HTMLURL,
				UpdatedAt: issue.UpdatedAt.UnixMilli(),
			}
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		fx.noteError(err)
		return out, fmt.Errorf("gitea %s: confirming absent items: %w", fx.ref, err)
	}
	return out, nil
}

// prepare returns a client for the account, or an error when it is in
// cooldown, not connected, or has no resolvable token — the states a poll must
// not reach the network in.
func (fx *fetcher) prepare() (*giteaclient.Client, error) {
	if until, cooling := fx.inCooldown(); cooling {
		return nil, fmt.Errorf("gitea %s: %w until %s", fx.ref, sourcehttp.ErrRateLimited, until.Format(time.RFC3339))
	}
	binding, err := fx.instances.Get(fx.ref)
	if err != nil {
		return nil, fmt.Errorf("gitea %s: reading instance binding: %w", fx.ref, err)
	}
	if binding.URL == "" {
		return nil, fmt.Errorf("gitea %s: instance is not connected", fx.ref)
	}
	token, err := fx.resolve()
	if err != nil {
		return nil, fmt.Errorf("gitea %s: resolving token: %w", fx.ref, err)
	}
	if token == "" {
		return nil, fmt.Errorf("gitea %s: %w", fx.ref, sourcehttp.ErrUnauthorized)
	}
	return fx.newClient(binding.URL, token), nil
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
