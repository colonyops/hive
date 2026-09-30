# Source connectors are declared, not sniffed

- **Status:** accepted
- **Date:** 2026-07-25

## Context

A source connector was not a declared thing. It was a `Produce` method, plus three optional behaviours the producer discovered at run time by type assertion: `MetadataSource` for its ingestion metadata, `SearchDefSource` for inclusion in the batched GitHub prefetch, and a `map[sourceKind]store.SourceAdapter` lookup for its classifier and absence confirmer.

Every one of those fails open. A connector that stops satisfying an interface still compiles, still polls, and still appends — it just ingests as `SourceKind "generic"` with no classifier and no absence confirmation, which surfaces as a feed that looks subtly wrong rather than as an error. The GitHub connector carried three `var _ ingest.X = (*githubSource)(nil)` assertions as a hand-rolled substitute for a declaration, and a test whose entire purpose was to prove the assertions still matched across a package boundary.

Three further consequences followed from having nothing that enumerates connectors:

- `ingest.SearchDefSource` named `feed.SourceDef`, a GitHub-shaped type, in a connector-neutral package.
- `flow.GithubSourceConfig` held GitHub API page caps (search 100, notifications 50) in a package whose own doc comment forbids it from importing `feed`. The rules lived apart from the code they constrain.
- Neither an editor form nor an MCP tool's input schema could be generated from one declaration, which is the ADR go-owned-llm-prompts pattern this was meant to extend.

## Decision

A connector is declared in two halves, the same split `flow.registry` and `runtime.behaviors` already use.

**`connector.Descriptor` is static data** — type, title, mode, stability, declared capabilities, and a config factory. It has no dependencies, so the registry can hold it as package state and the flow package can derive a node type from each entry. It lives in the connector's own package and is listed in `sources/registry.go`.

**`connector.Factory` is a struct of optional funcs**, constructed in `app.New` where the dependencies exist. What is set is what is supported. Two tests hold the halves together: `TestFactoriesCoverEveryDescriptor` fails on a connector the editor offers and nothing polls, and `TestFactoriesMatchDescribedCapabilities` fails when a descriptor promises a capability its factory does not wire.

**Capabilities are read off the instance, never asserted for.** The producer reads `instance.Classifier`, `instance.Absence`, and the resolver dispatches `Factory.Prefetch` per connector type. There is no assertion left to get wrong.

**Pull and push are separate modes.** The resolver answers `PullInstances()` and `PushInstances()` from the same walk, so the poll producer never sees a push instance with a nil `PullSource`, and a push connector never has to fake a blocking read.

**Node types are namespaced**: `github-source` → `sources.github`, `webhook-source` → `sources.webhook`. `flow.registry` and `runtime.behaviors` derive their source entries from the connector registry rather than listing them, so adding a connector is a change to `internal/app/sources` alone.

**Config belongs to the connector**, with `json`/`yaml`/`jsonschema` tags and a `Validate() error`. The JSON Schema is reflected from the struct via `invopop/jsonschema`, so it cannot describe a field the struct lacks.

### Package shape

The registry is a package-level map in one file — `gochecknoinits` is on, so there is no self-registration — which means `sources` imports the connector packages and they cannot import it back. The vocabulary therefore lives in a leaf, `sources/connector`, which both sides name:

```
sources/connector/   Descriptor, Factory, Instance, Config, capabilities
sources/github/      imports connector, store, activity, feed
sources/webhook/     imports connector, store, activity
sources/             registry.go — the map
flow/                imports sources; derives its source node types
ingest/              imports flow + sources; resolves instances
```

Building the registry at the composition root instead (the OpenTelemetry Collector's shape) works for instances but not for declaration: `flow` decodes node config inside `UnmarshalYAML`, which has no place to receive a registry.

## Consequences

- **`flows/*.yaml` files carrying `type: github-source` or `type: webhook-source` no longer load.** Breaking config changes are permitted (`architecture.md#data-that-must-survive`), only `desktop-v*-dev` tags exist, and there is no migration shim per repo rules.
- `flow.SourceConfig` wraps a connector config to supply the port counts a source has by definition. Its four `(Un)Marshal(JSON|YAML)` methods re-create the strict decode themselves: a custom unmarshaler bypasses the outer decoder's `KnownFields(true)` / `DisallowUnknownFields`, and losing that would make an unknown key in a source node's config parse as nothing rather than fail the load.
- The webhook `Listener` takes connector instances rather than the flow set, so the connector package no longer imports `flow`. Enablement filtering moved to the resolver, which is now the single place a disabled flow or node is excluded — for both ingress paths.
- The feed icon set moved to `internal/app/icons`. It is validated by a feed node (in `flow`) and by the webhook connector's config (outside it), and a shared leaf is the only home that does not recreate the cycle.
- `ingest` is connector-neutral again: it no longer names `feed.SourceDef`. This closes the leak the pipeline-split open question flagged.
- **`appkit/httpclient` is deferred, not skipped.** GitHub's fetches go through the vendored `hivecore/github.Client`, whose transport is a concrete `*http.Client` field with only a `WithHTTPClient(*http.Client)` option — an `appkit/httpclient.Client` wraps an `*http.Client` rather than being one, so it cannot be substituted without an upstream change in `colonyops/hive` and a re-vendor. The app's only other outbound HTTP is the updater and `cmd/release`, neither a connector fetch path, so adopting it now would leave it with no consumer in the layer it exists for. `architecture.md` records this against the row that mandates it.
- The JSON Schema has no consumer yet. Its consumers are phase 7 (MCP tool input schemas, an HTTP surface) and, later, a schema-rendered editor form; the editor forms stay hand-written for now, being hand-tuned and the one piece that can land later without changing a Go contract.
- Adding a connector no longer touches TypeScript for *routing* — but a new connector still needs a `nodes/<type>/` editor entry until those forms are schema-driven.
