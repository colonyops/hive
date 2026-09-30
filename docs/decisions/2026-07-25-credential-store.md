# Credentials are keyed by account, in a store of our own

- **Status:** accepted
- **Date:** 2026-07-25

## Context

The app had one credential: a GitHub token, held by the vendored
`internal/hivecore/github/token.go`. That store is single-slot by construction —
`keyringAccount` is a package constant, so there is exactly one token and it has
no provider or account dimension. Around it sat `internal/app/auth`, a device
flow plus a `Status{Login, Name, AvatarURL}` that the whole app treated as
sign-in state: `App.vue` would not render anything until it resolved, first-run
onboarding presented "Connect GitHub" as step 1, and the feed, the flows canvas
and settings all sat behind it.

That was already wrong for the shape the app is moving to. A `sources.webhook`
node needs no credential at all, and the next integration (Grafana) needs a
token *per instance* — a home Grafana and a work Grafana are two accounts, not
two names for one. It was also wrong for GitHub itself: a personal account and a
work account are a normal thing to want side by side, and the single slot made
that unrepresentable.

The vendored store cannot be extended in place. `internal/hivecore/` is
read-only by policy (ADR vendor-hive-internals) — a change there lands in `colonyops/hive` first
and comes back through the sync tool, and the CI drift check enforces it. A
second provider was therefore going to be a new store no matter what; the only
question was where it lived and what it was keyed by.

## Decision

**A credential is keyed by `Ref{Provider, Account}`**, a value object that
parses from and prints as `github/octocat`. It is never a bare string in a
signature. `internal/app/credentials` is a leaf package — it imports no `app`,
`flow` or connector — so a connector can depend on it without a cycle, the same
rule that put the connector vocabulary in a leaf.

**Multi-account from the start, and `credential:` is required** on a source
node. An optional field with a single-account fallback was drafted and rejected:
it would have to be unwound for Grafana, and breaking changes to `flows/*.yaml`
belong before alpha, not after. This is the second such break in two phases
(ADR source-connector-registry renamed the `type:` discriminator), which is exactly the argument for
taking it now.

**Values live in the OS keychain; a separate JSON index holds the refs.**
Keychains do not enumerate — `zalando/go-keyring` has no `List` — so `List()`
needs something else to read. The index is `<StateDir>/credentials.json`,
written temp-then-rename:

- The *state* dir, not the config dir. It is app-local state, and a ref index
  sitting next to dotfiles-managed `flows/` would invite someone to hand-edit it
  into disagreement with the keychain.
- A file, not a table in `desktop-pipeline.db`. The store has to be
  constructible and testable before the database exists — `sourceFactories`
  needs it during `app.New`, ahead of the producer.

**The keychain is the truth and the index is a cache.** A ref present in the
index but absent from the keychain reads as `ErrNotFound` and the entry is
pruned. The alternative — trusting the index — surfaces a divergence as a
"Connected" badge over a credential that is gone, which is worse than reporting
disconnected.

**Values are plain strings.** There is no redacting `Secret` type, local or from
`appkit/secret`. A redacting wrapper earns its place when secrets are loaded
from a config file and then flow through structs that get marshalled and
logged; the "config holds refs, never tokens" rule means that path does not
exist here. A value's whole life is keychain → connector factory → provider
client, and none of those marshal it. `architecture.md` listed `secret.Secret`
as a Value Object with a home here; that row was corrected rather than a type
built to satisfy it.

**Lookup is generic; only acquisition is provider-specific.** `Resolve`, `Bind`,
`ListProvider` and `EnvOverrideName` name no provider — the headless override is
derived from the provider name, so `HIVE_GITHUB_TOKEN` keeps working and
`HIVE_GRAFANA_TOKEN` comes for free. Acquisition belongs to the connector:
GitHub's device flow and PAT fallback are `sources/github/connect.go`, and the
`Connection` interface is declared by the connector that implements it, so
Grafana — a secret-marked config field with no state machine — declares none.

**`Bind` reads through on every call.** Connecting, rotating or disconnecting an
account takes effect on the next fetch with nothing to invalidate. Capturing the
value at wiring time instead would make a rotation need a restart.

**GitHub is a connector, not a login.** `internal/app/auth` is deleted; the
vocabulary is connected/disconnected, not authenticated/unauthenticated, because
nothing is gated on it. First run is create workspace → connect GitHub → feed:
the workspace is the one thing that exists without a credential, so it goes
first, and connecting is what seeds its starter graph. Settings ▸ Integrations
is a projection of the connector registry joined to what the store holds.

## Consequences

- **Existing `flows/*.yaml` break.** Every `sources.github` node needs a
  `credential:`. There is no migration code, per the standing rule — the flow
  fails to load with the field named in the error.
- **The old token is stranded.** It sits at the vendored single-slot account
  (`sh.hive.desktop` / `github-token`); the new store looks for
  `sh.hive.desktop` / `github/<login>` plus the ref index. An upgrading install
  shows disconnected until it reconnects through the UI, which writes the new
  ref.
- **One fetcher per account.** A `feed.LiveProvider` was already a per-token
  object — its response cache, conditional-request state and rate-limit cooldown
  each hold one account — so honouring `credential:` per node meant handing out
  one provider per ref rather than sharing a singleton. Two accounts can no
  longer see each other's items or stall each other's fetches, at the cost of
  one provider per account in memory. `Prefetch` buckets by account, because a
  batched search is one request on one token.
- **Cache invalidation scopes per connector.** A connection transition drops
  that provider's fetch caches before anything is notified. It over-invalidates
  — every GitHub account, not just the changed ref — deliberately:
  over-invalidating costs a refetch, under-invalidating serves one account's
  items to another.
- **`HIVE_GITHUB_TOKEN` names no account,** so a listing of stored refs is empty
  while it is set. Integrations reports the override separately from the
  accounts; collapsing the two would print "Not connected" beside a working feed
  in exactly the CI, server-build and e2e configurations that depend on it.
- **Keychain access in tests is a hazard.** A read can prompt, and a fixture run
  that prompts hangs. Mock modes get `MemoryStore`, keychain tests use
  `keyring.MockInit()` — which swaps a package-level provider, so none of them
  may run in parallel — and `TestStoreContract` holds both stores to one
  contract so a memory store cannot drift into testing something production
  does not do.
