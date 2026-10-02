package dispatch

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/colonyops/hive/pkg/tmpl"

	"github.com/colonyops/hive/cmd/desktop/internal/app/actions"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
)

// DefaultItemKind is the kind an item carries when its payload declares
// none. Every item has a kind so every item is automatable: an action with
// applies_to: [Item] targets exactly the untyped ones, instead of them being
// reachable only by an action with no applies_to at all. Must stay in sync
// with the frontend's DEFAULT_ITEM_KIND (lib/itemPresentation.ts), which
// feeds the actions editor's autocomplete. Matching is case-insensitive, so
// a hand-written applies_to: [item] works too. See
// docs/decisions/2026-07-24-canonical-item-contract.md.
const DefaultItemKind = "Item"

// DecodedActionItem is the canonical action-item projection of a persisted
// inbox payload: identity, canonical kind, and the payload bytes handed to
// the output worker for template rendering. The desktop service deliberately
// never accepts this shape from a client.
type DecodedActionItem struct {
	ID      string
	Kind    string
	Payload []byte
}

// DecodeActionItem projects any source's persisted inbox payload into the
// canonical action item. Object payloads: `id` accepts a string or number;
// a missing/blank id is filled from the row's external id (re-marshaling
// only in that case); `kind` is the canonical top-level string, falling back
// to DefaultItemKind so every item is targetable by applies_to. Non-object
// payloads (arrays, scalars) pass through untouched — ID comes from
// externalID, kind is the default, and no id injection is attempted. All
// other fields pass through untouched (grab bag preserved). The default is
// applied here, at the one decode boundary, rather than written into stored
// payloads: no migration, and the raw payload still answers "did the source
// actually send a kind?".
func DecodeActionItem(payload []byte, externalID string) (DecodedActionItem, error) {
	id, kind, _ := models.CanonicalFields(payload)
	if kind == "" {
		kind = DefaultItemKind
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil || fields == nil {
		// Non-object or invalid JSON payloads pass through unchanged. JSON null
		// also yields a nil map.
		return DecodedActionItem{ID: externalID, Kind: kind, Payload: payload}, nil
	}

	if id != "" {
		return DecodedActionItem{ID: id, Kind: kind, Payload: payload}, nil
	}

	// Missing/blank id: inject externalID, re-marshaling only in this case.
	// json.Marshal on a map[string]json.RawMessage writes each value's raw
	// bytes verbatim, so every other field survives byte-for-byte.
	idBytes, err := json.Marshal(externalID)
	if err != nil {
		return DecodedActionItem{}, fmt.Errorf("encoding action item id: %w", err)
	}
	fields["id"] = idBytes
	encoded, err := json.Marshal(fields)
	if err != nil {
		return DecodedActionItem{}, fmt.Errorf("encoding action item: %w", err)
	}
	return DecodedActionItem{ID: externalID, Kind: kind, Payload: encoded}, nil
}

// RenderRepoTarget renders a launch-session action's repo_template over the
// SAME OutputData context the executor renders with (zero CommandID /
// IsRerun) and trims the result. Extracted from LaunchSessionExecutor —
// which now calls it — so the applicability probe and the executor can
// never drift: a template referencing .Raw or .Key probes exactly as it
// executes. Actions without a repo_template (interactive or workspace
// launch-session actions, and every other type) render "" with no error;
// they impose no repository requirement on the payload.
func RenderRepoTarget(action actions.Action, key string, payload []byte, inputs map[string]string) (string, error) {
	cfg, ok := action.Config.(*actions.LaunchSessionConfig)
	if !ok {
		return "", nil
	}
	return renderRepoTemplate(cfg.RepoTemplate, key, payload, inputs)
}

func renderRepoTemplate(repoTemplate, key string, payload []byte, inputs map[string]string) (string, error) {
	if repoTemplate == "" {
		return "", nil
	}

	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return "", fmt.Errorf("decode payload: %w", err)
	}

	renderer := tmpl.New(tmpl.Config{})
	rendered, err := renderer.Render(repoTemplate, OutputData{
		Key:     key,
		Payload: decoded,
		Raw:     json.RawMessage(payload),
		Inputs:  inputs,
	})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(rendered), nil
}

// ActionApplicability reports whether action can run against item and, when
// it cannot, a human-readable reason (applies_to mismatch, blank repo
// render, or the underlying template render error) so the invoke path stays
// debuggable. The hard-requirement rule derives from
// actions.Action.HeadlessCapable() — non-blank repo_template ⇒ requires a
// renderable, non-blank repo target — keeping the knowledge of which action
// types impose payload requirements next to the type definitions in package
// actions. All other action types and templates impose no applicability
// requirement.
func ActionApplicability(action actions.Action, item DecodedActionItem) (ok bool, reason string) {
	if !actions.AppliesTo(action, item.Kind) {
		return false, fmt.Sprintf("action %q does not apply to kind %q", action.ID, item.Kind)
	}

	launch, isLaunchSession := action.Config.(*actions.LaunchSessionConfig)
	if !isLaunchSession || !action.HeadlessCapable() || strings.TrimSpace(launch.RepoTemplate) == "" {
		return true, ""
	}

	// The probe has no collected values, so it resolves the declared
	// defaults — the same values a headless run would get. Under the
	// HeadlessCapable guard above resolution cannot fail; if that ever
	// breaks, the action correctly reads as not applicable.
	inputs, err := action.ResolveInputs(nil)
	if err != nil {
		return false, fmt.Sprintf("action %q: inputs: %v", action.ID, err)
	}
	repo, err := RenderRepoTarget(action, item.ID, item.Payload, inputs)
	if err != nil {
		return false, fmt.Sprintf("action %q: repo_template: %v", action.ID, err)
	}
	if repo == "" {
		return false, fmt.Sprintf("action %q: repo_template rendered blank", action.ID)
	}
	return true, ""
}
