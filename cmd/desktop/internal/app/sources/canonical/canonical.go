// Package canonical is the item contract a connector uses when its payload is
// the canonical item shape itself (docs/decisions/0008) rather than a
// provider's own: the top-level `state` vocabulary, the classifier that maps it
// to lifecycle, and the rewrite an authoritative snapshot mints for an item
// that left it.
//
// It exists so a user learns one contract. A webhook delivery and an exec
// source's snapshot item are both JSON the user shaped themselves, so the same
// `state: resolved` must mean the same thing in both; a per-connector copy of
// these rules would drift silently, one connector archiving on a word another
// ignores.
//
// A leaf, for the same reason connector is: the connectors that share it must
// not import each other.
package canonical

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
)

// TerminalState is the state a source mints for an item that left an
// authoritative snapshot. It is one of terminalStates, so the classifier
// archives it on the next ingest exactly as a payload that said so itself.
const TerminalState = "resolved"

// terminalStates are the canonical `state` values that end an item's
// lifecycle. Comparison is case-insensitive; any other or absent state keeps
// the item active — a stateless payload is manual triage only.
var terminalStates = map[string]bool{"resolved": true, "closed": true, "done": true}

// State extracts the canonical top-level `state` from a payload: lowercased
// and trimmed; "" for non-object payloads or a missing/non-string state.
func State(payload []byte) string {
	_, _, state := models.CanonicalFields(payload)
	return strings.ToLower(strings.TrimSpace(state))
}

// isTerminal reports whether a state ends the item's lifecycle.
func isTerminal(state string) bool { return terminalStates[state] }

// WithState rewrites a payload's top-level state, preserving every other field
// so an item archived on absence keeps its title, url and everything a
// downstream node reads. A payload that is not a JSON object is returned
// unchanged: there is nothing to rewrite, and the classifier reads its state as
// absent either way.
func WithState(payload []byte, state string) []byte {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		return payload
	}
	fields["state"], _ = json.Marshal(state)
	out, err := json.Marshal(fields)
	if err != nil {
		return payload
	}
	return out
}

// Classifier maps the canonical top-level `state` to lifecycle exactly like the
// GitHub classifier: a first observation is "received" (terminal on arrival
// stays unarchived, matching GitHub); an observation that newly enters a
// terminal state system-archives with the state as both the event kind and the
// archive reason; one that leaves a terminal state resurfaces as "reopened";
// anything else is "updated" activity.
//
// An unchanged re-observation never reaches classification — IngestObservation
// skips it on the source-head comparison. The occurrence key is per observation
// so downstream action dedup fires once per change.
type Classifier struct{}

var _ models.Classifier = Classifier{}

func (Classifier) Classify(previous *models.Observation, current models.Observation) models.Classification {
	state := State(current.Payload)
	curTerminal := isTerminal(state)
	lifecycle := models.LifecycleActive
	if curTerminal {
		lifecycle = models.LifecycleTerminal
	}
	out := models.Classification{
		Kind:          "updated",
		Transition:    models.TransitionNone,
		Attention:     models.AttentionActivity,
		Lifecycle:     lifecycle,
		SourceState:   state,
		OccurrenceKey: current.ExternalID + "@" + strconv.FormatInt(current.ObservedAt, 10),
		Summary:       current.Title,
	}
	if previous == nil {
		out.Kind = "received"
		return out
	}
	prevTerminal := isTerminal(State(previous.Payload))
	switch {
	case !prevTerminal && curTerminal:
		out.Kind, out.Summary, out.Transition, out.ArchivedReason = state, titleCase(state), models.TransitionEnteredTerminal, state
	case prevTerminal && !curTerminal:
		out.Kind, out.Summary, out.Transition = "reopened", "Reopened", models.TransitionLeftTerminal
	}
	return out
}

// titleCase renders a canonical state string as an event summary.
func titleCase(value string) string {
	if value == "" {
		return "State changed"
	}
	return strings.ToUpper(value[:1]) + value[1:]
}
