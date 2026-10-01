// Package connector is the vocabulary a source connector is declared in:
// the Descriptor that describes one, the Config it parses, and the Instance
// the app constructs per use.
//
// It is deliberately a leaf. The registry that holds every descriptor is a
// package-level map in one file (cmd/desktop/internal/app/sources), which means that
// package imports the connector packages — so the connector packages cannot
// import it back. They name their vocabulary from here instead.
package connector

import (
	"context"
	"time"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
)

// Config is everything a connector needs from a node's `type:`-specific
// fields. A connector supplies a struct with json/yaml/jsonschema tags and
// this one method; it never decodes YAML itself, and it never reports port
// counts — a source is 0-in/1-out by definition, and the graph vocabulary
// belongs to the flow package that wraps this.
//
// Validate covers cross-field rules the schema cannot express ("kind
// notifications takes no query"). Shape and type errors are the decoder's.
type Config interface {
	Validate() error
}

// Mode separates the two ingestion shapes. They are kept distinct rather than
// unified because a push connector would otherwise have to fake a blocking
// read: the poll producer drains ModePull instances on a tick, and a ModePush
// connector's own ingress delivers into the log when something arrives.
type Mode uint8

const (
	// ModePull is drained by the poll producer once per tick.
	ModePull Mode = iota + 1
	// ModePush delivers on its own schedule through its own ingress.
	ModePush
)

// String is the wire form. As with Stability, an undeclared mode reads as
// "unknown" rather than "0".
func (m Mode) String() string {
	switch m {
	case ModePull:
		return "pull"
	case ModePush:
		return "push"
	default:
		return "unknown"
	}
}

// Stability is how much a connector's config shape may still move. It is
// declared per connector so an editor, an MCP tool listing, or the docs can
// warn before a user builds on something that is about to change.
type Stability uint8

const (
	// Experimental config may change without a deprecation path.
	Experimental Stability = iota + 1
	// Beta config is settled but under-exercised.
	Beta
	// Stable config only changes compatibly.
	Stable
)

// String is the wire form the Integrations screen and the docs render. A
// value outside the set reads as "unknown" rather than a bare number, so a
// descriptor that forgot to declare one is visible instead of showing "0".
func (s Stability) String() string {
	switch s {
	case Experimental:
		return "experimental"
	case Beta:
		return "beta"
	case Stable:
		return "stable"
	default:
		return "unknown"
	}
}

// Capability is what a connector supports beyond producing messages. It is
// declared on the Descriptor and wired on the Factory; nothing sniffs for it
// with a type assertion, because a missed assertion is a silent downgrade —
// no classifier, no absence confirmation — that surfaces as wrong inbox state
// rather than as a failure.
type Capability uint16

const (
	// CapClassify means the connector supplies its own models.Classifier
	// instead of falling back to generic observed/updated classification.
	CapClassify Capability = 1 << iota
	// CapConfirmAbsence means the connector can be asked what happened to an
	// item that stopped appearing in its snapshot, rather than leaving it to
	// age out.
	CapConfirmAbsence
	// CapBatchPrefetch means every instance of this connector type is offered
	// to one batched pre-pass before any of them is drained, so a provider
	// that can answer many queries in one request does.
	CapBatchPrefetch
)

// Has reports whether every capability in want is declared.
func (c Capability) Has(want Capability) bool { return c&want == want }

// Descriptor declares one connector type. It is static data with no
// dependencies, which is what lets the registry hold it as package state and
// lets the flow package derive a node type from it. Everything that needs a
// running connector goes through Factory instead.
type Descriptor struct {
	// Type is the node `type:` discriminator and the registry key, namespaced
	// so connectors group and sort together: "sources.github".
	Type string
	// Title is the human label — the palette entry, the docs heading.
	Title string
	// ProviderTitle titles the Integrations card for a provider that ships
	// several connector types; empty for a single-type connector.
	ProviderTitle string
	// Provider is the credentials provider this connector fetches as
	// ("github"), matching credentials.Ref.Provider. Empty means the
	// connector needs no credential at all — the webhook listener is local
	// ingress and has nothing to connect to.
	//
	// A name, not a store: the descriptor stays static data with no
	// dependencies, which is what lets the flow and runtime registries derive
	// node types from it as package state. What is stored under the name is
	// resolved where the credential store exists.
	Provider string
	// Mode selects pull or push ingestion.
	Mode Mode
	// Stability is how settled Config's shape is.
	Stability Stability
	// Capabilities is what this connector supports beyond producing messages.
	// A Factory that fills a capability this does not declare, or declares one
	// it does not fill, fails TestFactoriesMatchDescribedCapabilities.
	Capabilities Capability
	// NewConfig returns a fresh, zero-valued config. Each call must return a
	// distinct value — the decoder mutates it in place.
	NewConfig func() Config
}

// Node identifies the flow node one instance was built for. The producer and
// the log both address a source by its flow-qualified id, so the two forms
// are derived once here rather than re-concatenated at each use.
type Node struct {
	FlowID string
	NodeID string
	// Policy is the owning flow's resurface policy, which governs what
	// happens when an item this source dropped comes back.
	Policy models.ResurfacePolicy
}

// ID is the flow-qualified source id, "<flowID>/<nodeID>".
func (n Node) ID() string { return n.FlowID + "/" + n.NodeID }

// Topic is the event-log topic this source's rows are appended under, so a
// flow only ingests rows from its own source nodes.
func (n Node) Topic() string { return "source:" + n.ID() }

// Metadata is the source-side data the ingestion boundary needs. It is
// returned by the factory rather than discovered from the instance, so a
// connector that forgets it fails to compile instead of silently ingesting as
// SourceKind "generic".
type Metadata struct {
	// ProfileID scopes inbox membership; it is the owning flow's id.
	ProfileID string
	// SourceKind names the provider ("github", "webhook") in inbox rows.
	SourceKind string
	// SourceScope distinguishes several sources of one kind within a flow.
	SourceScope string
	// Policy governs resurfacing of an item that reappears.
	Policy models.ResurfacePolicy
}

// PullSource produces the current state of one source as a sequence of
// messages, calling emit once per item. Produce is called synchronously from
// a producer tick and returns once every current item has been emitted, or
// once fetching or an emit call fails. A tick drains several instances at
// once, so anything they share must be safe for concurrent use.
//
// A successful call is an authoritative snapshot, including an empty one: the
// producer records the complete emitted key/payload set after it returns, and
// an item missing from it is treated as absent. Implementations set Key to
// the item's stable identity and Payload to the item's JSON; the producer
// supplies the topic.
type PullSource interface {
	Produce(ctx context.Context, emit func(models.Msg) error) error
}

// Instance is one constructed connector: the source itself plus whatever
// capabilities its descriptor declares. The optional fields are set by the
// factory, never discovered — reading Classifier is how the producer asks
// "does this connector classify?", and there is no assertion to get wrong.
type Instance struct {
	// Type is the descriptor's type; Node is the flow node it serves.
	Type string
	Node Node
	// Metadata is the ingestion-boundary data for this instance.
	Metadata Metadata
	// Pull produces this source's current items. Set for ModePull only.
	Pull PullSource
	// MinInterval is the shortest time this instance wants between drains.
	// Zero — the default — is drained on every tick, which is what every
	// connector did before the field existed.
	//
	// A floor, not a schedule: there is one global ticker, so the effective
	// cadence rounds up to the next tick after the floor expires. A skipped
	// tick is not an empty snapshot — the producer does not call Produce at
	// all, so nothing about the source's tracked set changes.
	MinInterval time.Duration
	// Classifier turns an observation into an inbox event. Set iff the
	// descriptor declares CapClassify.
	Classifier models.Classifier
	// Absence answers what happened to an item that left the snapshot. Set
	// iff the descriptor declares CapConfirmAbsence.
	Absence models.AbsenceConfirmer
	// Config is the parsed config this instance was built from, kept so a
	// batched prefetch can re-read it without a second decode.
	Config Config
}

// Factory constructs live instances of one connector type. It is the second
// half of the declaration: Descriptor is static and lives in the connector
// package, Factory is built where the dependencies exist (app.New holds the
// GitHub fetcher) and carries the funcs the capabilities promised.
//
// A struct of optional funcs rather than an interface with optional methods,
// for the same reason runtime.behavior is: what is set is what is supported,
// and a test can compare that against what the descriptor declared.
type Factory struct {
	// New builds one instance from a node's parsed config.
	New func(node Node, cfg Config) (Instance, error)
	// Prefetch is offered every instance of this type once per tick, before
	// any is drained, so a provider that can batch does. Nil unless the
	// descriptor declares CapBatchPrefetch. A failure is advisory: the
	// producer logs it and drains each source individually.
	Prefetch func(ctx context.Context, instances []Instance) error
}
