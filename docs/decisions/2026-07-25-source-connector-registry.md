# Source connectors are declared, not sniffed

- **Status:** accepted
- **Date:** 2026-07-25

## Context

Optional connector behaviors discovered by type assertion fail open: a connector that stops satisfying an
interface still compiles, polls, and appends, and only the feed looks subtly wrong. Nothing enumerated the
connectors, so neither editors nor generated schemas could be derived from one declaration.

## Decision

A connector is declared in two halves, the same split `flow.registry` and `runtime.behaviors` use.

**`connector.Descriptor` is static data**: type, title, provider, mode, stability, declared capabilities,
and a config factory. It has no dependencies, lives in the connector's own package, and is listed in the
`descriptors` map in `cmd/desktop/internal/app/sources/registry.go`.

**`connector.Factory` is a struct of optional funcs**, built in `sourceFactories` in `app.go` where the
dependencies exist. What is set is what is supported. `TestFactoriesCoverEveryDescriptor` fails on a
connector the editor offers and nothing polls; `TestFactoriesMatchDescribedCapabilities` fails when a
descriptor and its factory disagree.

**Capabilities are read off the instance, never asserted for.** The producer reads `instance.Classifier`
and `instance.Absence`, and the resolver dispatches `Factory.Prefetch` per connector type.

**Pull and push are separate modes.** `ingest.Resolver` answers `PullInstances()` and `PushInstances()`
from one walk, so the poll producer never sees a push instance and a push connector never fakes a blocking
read. The resolver is the one place a disabled flow or node is excluded, for both paths.

**Node types are namespaced** (`sources.github`, `sources.webhook`, ...). `flow.registry` and
`runtime.behaviors` derive their source entries from the connector registry, so adding a connector is a
change under `sources/` plus its editor entry in `frontend/src/pipeline/nodes/<type>/`.

**Config belongs to the connector**, with `json`/`yaml`/`jsonschema` tags and `Validate() error`.
`connector.Schema` reflects the JSON Schema from the struct with `invopop/jsonschema`.

**Package shape.** The registry is a package-level map (`gochecknoinits` is on, so no self-registration),
so `sources` imports the connector packages and they cannot import it back. The shared vocabulary lives in
the leaf `sources/connector`. A composition-root registry would not work for declaration: `flow` decodes
node config inside `UnmarshalYAML`, which has no way to receive one.

## Consequences

- `flow.SourceConfig` wraps a connector config and re-creates the strict decode in its own
  `(Un)Marshal(JSON|YAML)` methods. A custom unmarshaler bypasses the outer decoder's `KnownFields(true)`,
  and losing it would let an unknown key in a source config parse as nothing.
- The webhook `Listener` takes connector instances, not the flow set, so connector packages never import
  `flow`.
- Shared validation vocabularies used both by `flow` and by connector configs (the feed icon set in
  `cmd/desktop/internal/app/icons`) live in leaves, to avoid the import cycle.
