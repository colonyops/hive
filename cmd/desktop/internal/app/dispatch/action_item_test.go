package dispatch

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/actions"
)

// ── DecodeActionItem ─────────────────────────────────────────────────────────

func TestDecodeActionItem_IDIsAStringOrNumber(t *testing.T) {
	payload := []byte(`{"id":"pr-1","kind":"PR","title":"Fix it"}`)
	item, err := DecodeActionItem(payload, "ext-1")
	require.NoError(t, err)
	assert.Equal(t, "pr-1", item.ID)
	assert.Equal(t, "PR", item.Kind)
	assert.JSONEq(t, string(payload), string(item.Payload))

	payload = []byte(`{"id":123,"kind":"Issue"}`)
	item, err = DecodeActionItem(payload, "ext-2")
	require.NoError(t, err)
	assert.Equal(t, "123", item.ID)
	assert.Equal(t, "Issue", item.Kind)
	assert.JSONEq(t, string(payload), string(item.Payload))
}

func TestDecodeActionItem_MissingOrBlankIDFillsFromExternalID(t *testing.T) {
	payload := []byte(`{"kind":"PR","title":"Fix it"}`)
	item, err := DecodeActionItem(payload, "ext-42")
	require.NoError(t, err)
	assert.Equal(t, "ext-42", item.ID)
	assert.Equal(t, "PR", item.Kind)
	var fields map[string]any
	require.NoError(t, json.Unmarshal(item.Payload, &fields))
	assert.Equal(t, "ext-42", fields["id"])
	assert.Equal(t, "Fix it", fields["title"], "grab-bag fields survive the re-marshal")

	payload = []byte(`{"id":"","kind":"PR"}`)
	item, err = DecodeActionItem(payload, "ext-7")
	require.NoError(t, err)
	assert.Equal(t, "ext-7", item.ID)

	payload = []byte(`{"id":"   ","kind":"PR"}`)
	item, err = DecodeActionItem(payload, "ext-8")
	require.NoError(t, err)
	assert.Equal(t, "ext-8", item.ID, "whitespace-only id counts as blank")
}

func TestDecodeActionItem_KindAbsentFallsBackToDefault(t *testing.T) {
	payload := []byte(`{"id":"pr-1","title":"Fix it"}`)
	item, err := DecodeActionItem(payload, "ext-1")
	require.NoError(t, err)
	assert.Equal(t, DefaultItemKind, item.Kind)
	assert.JSONEq(t, string(payload), string(item.Payload), "the default is derived, never written into the payload")

	blank, err := DecodeActionItem([]byte(`{"id":"pr-1","kind":"  "}`), "ext-1")
	require.NoError(t, err)
	assert.Equal(t, DefaultItemKind, blank.Kind, "a whitespace-only kind is no kind")
}

func TestDecodeActionItem_NonObjectPayloadPassesThroughUntouched(t *testing.T) {
	for _, payload := range [][]byte{
		[]byte(`[1,2,3]`),
		[]byte(`"just a string"`),
		[]byte(`42`),
		[]byte(`null`),
	} {
		item, err := DecodeActionItem(payload, "ext-9")
		require.NoError(t, err)
		assert.Equal(t, "ext-9", item.ID, "identity falls back to externalID for %s", payload)
		assert.Equal(t, DefaultItemKind, item.Kind, "non-object payloads take the default kind for %s", payload)
		assert.Equal(t, payload, item.Payload, "non-object payload passes through byte-for-byte for %s", payload)
	}
}

func TestDecodeActionItem_GrabBagFieldsPassThroughUnmodified(t *testing.T) {
	payload := []byte(`{"id":"pr-1","kind":"PR","branch":"feat/x","prompt":"do it","reason":"flaky"}`)
	item, err := DecodeActionItem(payload, "ext-1")
	require.NoError(t, err)
	assert.JSONEq(t, string(payload), string(item.Payload))
}

// ── ActionApplicability ──────────────────────────────────────────────────────

func probeLaunchSessionAction(id, repoTemplate string, appliesTo ...string) actions.Action {
	return actions.Action{
		ID:           id,
		Type:         "launch-session",
		AppliesTo:    appliesTo,
		ShowInDetail: true,
		Config:       &actions.LaunchSessionConfig{PromptTemplate: "go", RepoTemplate: repoTemplate},
	}
}

func TestActionApplicability_LaunchSessionWithRepoTemplateSatisfied(t *testing.T) {
	action := probeLaunchSessionAction("deploy", "{{ .Payload.repo }}", "pr")
	item := DecodedActionItem{ID: "1", Kind: "PR", Payload: []byte(`{"repo":"acme/site"}`)}

	ok, reason := ActionApplicability(action, item)
	assert.True(t, ok)
	assert.Empty(t, reason)
}

func TestActionApplicability_PayloadMissingRepoIsRenderErrorAndInapplicable(t *testing.T) {
	action := probeLaunchSessionAction("deploy", "{{ .Payload.repo }}", "pr")
	item := DecodedActionItem{ID: "1", Kind: "PR", Payload: []byte(`{"title":"no repo key"}`)}

	ok, reason := ActionApplicability(action, item)
	assert.False(t, ok)
	assert.Contains(t, reason, "repo_template", "the reason surfaces the underlying template error")
}

func TestActionApplicability_BareTemplateOverBlankRepoIsInapplicable(t *testing.T) {
	action := probeLaunchSessionAction("deploy", "{{ .Payload.repo }}", "pr")
	item := DecodedActionItem{ID: "1", Kind: "PR", Payload: []byte(`{"repo":""}`)}

	ok, reason := ActionApplicability(action, item)
	assert.False(t, ok)
	assert.Contains(t, reason, "repo_template")
}

// TestActionApplicability_LiteralWrappedEmptyRepoStaysApplicableByDesign pins
// the documented behavior: a template whose literal text wraps an empty
// field renders non-blank, so it stays applicable even though the rendered
// clone target is broken. The broken target fails visibly at run time
// instead of silently hiding the action.
func TestActionApplicability_LiteralWrappedEmptyRepoStaysApplicableByDesign(t *testing.T) {
	action := probeLaunchSessionAction("deploy", "https://github.com/{{ .Payload.repo }}.git", "pr")
	item := DecodedActionItem{ID: "1", Kind: "PR", Payload: []byte(`{"repo":""}`)}

	ok, reason := ActionApplicability(action, item)
	assert.True(t, ok, "https://github.com/.git is a non-blank render")
	assert.Empty(t, reason)
}

func TestActionApplicability_WhitespaceOnlyRenderIsInapplicable(t *testing.T) {
	action := probeLaunchSessionAction("deploy", "   {{ .Payload.repo }}   ", "pr")
	item := DecodedActionItem{ID: "1", Kind: "PR", Payload: []byte(`{"repo":""}`)}

	ok, reason := ActionApplicability(action, item)
	assert.False(t, ok)
	assert.Contains(t, reason, "repo_template")
}

func TestActionApplicability_InteractiveLaunchSessionImposesNoRequirement(t *testing.T) {
	action := probeLaunchSessionAction("launch", "", "pr")
	item := DecodedActionItem{ID: "1", Kind: "PR", Payload: []byte(`{}`)}

	ok, reason := ActionApplicability(action, item)
	assert.True(t, ok)
	assert.Empty(t, reason)
}

func TestActionApplicability_ShellAndPublishMessageImposeNoRequirement(t *testing.T) {
	shell := actions.Action{ID: "notify", Type: "shell", AppliesTo: []string{"pr"}, ShowInDetail: true, Config: &actions.ShellConfig{CommandTemplate: "{{ .Payload.repo }}"}}
	publish := actions.Action{ID: "publish", Type: "publish-message", AppliesTo: []string{"pr"}, ShowInDetail: true, Config: &actions.PublishMessageConfig{MessageTemplate: "{{ .Payload.repo }}", Topic: "topic"}}
	// Payload deliberately lacks "repo" — a bare template referencing it
	// would render-error, but shell/publish-message templates are not
	// probed for applicability (they fail visibly at run time instead).
	item := DecodedActionItem{ID: "1", Kind: "PR", Payload: []byte(`{}`)}

	ok, reason := ActionApplicability(shell, item)
	assert.True(t, ok)
	assert.Empty(t, reason)

	ok, reason = ActionApplicability(publish, item)
	assert.True(t, ok)
	assert.Empty(t, reason)
}

func TestActionApplicability_AppliesToMatchingIsCaseInsensitive(t *testing.T) {
	action := probeLaunchSessionAction("deploy", "", "pr")
	item := DecodedActionItem{ID: "1", Kind: "PR", Payload: []byte(`{}`)}

	ok, _ := ActionApplicability(action, item)
	assert.True(t, ok, "applies_to matches the canonical kind case-insensitively")

	item.Kind = "Issue"
	ok, reason := ActionApplicability(action, item)
	assert.False(t, ok)
	assert.Contains(t, reason, "does not apply")
}

func TestActionApplicability_EmptyAppliesToMatchesAnyKind(t *testing.T) {
	action := probeLaunchSessionAction("deploy", "", "pr")
	action.AppliesTo = nil
	item := DecodedActionItem{ID: "1", Kind: "anything", Payload: []byte(`{}`)}

	ok, _ := ActionApplicability(action, item)
	assert.True(t, ok)
}

// TestActionApplicability_UntypedItemIsTargetableByDefaultKind is the point
// of DefaultItemKind: a payload that declares no kind is not a dead end for
// automation. It is reachable both by an action with no applies_to and by
// one scoped to the default kind (case-insensitively, so a hand-written
// "item" works), while staying out of scope for unrelated kinds.
func TestActionApplicability_UntypedItemIsTargetableByDefaultKind(t *testing.T) {
	item, err := DecodeActionItem([]byte(`{"id":"x","title":"no kind here"}`), "ext-1")
	require.NoError(t, err)

	anyKind := probeLaunchSessionAction("any", "")
	ok, _ := ActionApplicability(anyKind, item)
	assert.True(t, ok, "empty applies_to still matches everything")

	byDefault := probeLaunchSessionAction("untyped", "", DefaultItemKind)
	ok, _ = ActionApplicability(byDefault, item)
	assert.True(t, ok, "applies_to: [Item] targets untyped items")

	lowercase := probeLaunchSessionAction("untyped-lower", "", "item")
	ok, _ = ActionApplicability(lowercase, item)
	assert.True(t, ok, "applies_to matching stays case-insensitive")

	scoped := probeLaunchSessionAction("scoped", "", "pr")
	ok, reason := ActionApplicability(scoped, item)
	assert.False(t, ok, "an untyped item is not swept up by unrelated kinds")
	assert.Contains(t, reason, "does not apply")
}
