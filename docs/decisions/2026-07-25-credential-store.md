# Credentials are keyed by account, in a store of our own

- **Status:** accepted
- **Date:** 2026-07-25

## Context

Integrations need more than one credential: a personal and a work GitHub account side by side, a token per
Grafana stack, and none at all for a webhook source. A single-slot token store cannot represent that.

## Decision

**A credential is keyed by `Ref{Provider, Account}`**, a value object that parses from and prints as
`github/octocat`, never a bare string in a signature. The store is `internal/platform/credentials`, a leaf
any connector can import without a cycle.

**Multi-account, and `credential:` is required** on every source node whose connector declares a
`Provider`.

**Values live in the OS keychain; a JSON index holds the refs.** Keychains do not enumerate
(`zalando/go-keyring` has no `List`), so `List()` reads the index at `<StateDir>/credentials.json`
(`settings.Paths.CredentialsIndexPath`), written atomically. It is in the state dir, not next to
dotfiles-managed `flows/`, so nobody hand-edits it, and it is a file, not a table, so the store exists
before the database does.

**The keychain is the truth and the index is a cache.** A ref in the index but not in the keychain reads
as not found and is pruned, so a vanished credential shows disconnected rather than "Connected".

**Values are plain strings.** Config holds refs, never tokens, so a value's whole life is keychain to
connector factory to provider client, and nothing marshals or logs it. A redacting `Secret` type would
guard a path that does not exist.

**Lookup is generic; acquisition is provider-specific.** `Resolve`, `Bind`, `ListProvider`, and
`EnvOverrideName` name no provider; the headless override derives from the provider name
(`HIVE_GITHUB_TOKEN`, `HIVE_GRAFANA_TOKEN`). Acquisition belongs to the connector: GitHub's device flow
and PAT fallback are `cmd/desktop/internal/app/sources/github/connect.go`.

**`Bind` reads through on every call**, so connecting, rotating, or disconnecting takes effect on the next
fetch with nothing to invalidate and no restart.

**GitHub is a connector, not a login.** The vocabulary is connected/disconnected, and nothing in the app is
gated on it. Settings > Integrations is the connector registry joined to what the store holds.

## Consequences

- **One fetcher per account.** A provider's response cache, conditional-request state, and rate-limit
  cooldown each belong to one token, so two accounts cannot see each other's items or stall each other.
  GitHub's `Prefetch` buckets by account, because a batched search is one request on one token.
- **Cache invalidation over-invalidates on purpose.** A connection transition drops every fetch cache for
  that provider, not only the changed ref: a refetch is cheaper than serving one account's items to
  another.
- **An env override names no account**, so the stored-ref listing is empty while it is set. Integrations
  reports the override separately; otherwise CI, server builds, and e2e would show "Not connected" beside a
  working feed.
- **Keychain access in tests can prompt and hang.** Mock modes use `MemoryStore`. Keychain tests use
  `keyring.MockInit()`, which swaps package-level state, so they must not run in parallel.
  `TestStoreContract` holds both stores to one contract.
