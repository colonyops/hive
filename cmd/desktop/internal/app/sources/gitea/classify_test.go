package gitea

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
)

func observation(t *testing.T, item Item) models.Observation {
	t.Helper()
	payload, err := json.Marshal(item)
	require.NoError(t, err)
	return models.Observation{ExternalID: item.ID, Title: item.Title, URL: item.URL, ObservedAt: item.UpdatedAt, Payload: payload}
}

func searchItem(state string, updatedAt int64, labels ...string) Item {
	return Item{
		ID: "acme/app#42", Kind: "PR", Repo: "acme/app", Num: 42,
		Title: "Fix the checkout flow", State: state, Origin: OriginSearch,
		UpdatedAt: updatedAt, Labels: labels, URL: "https://git.example.com/acme/app/pulls/42",
	}
}

func TestClassifyFirstObservation(t *testing.T) {
	t.Parallel()

	current := observation(t, searchItem("open", 1000))
	out := classifier{}.Classify(nil, current)

	assert.Equal(t, "observed", out.Kind)
	assert.Equal(t, "Added to workspace", out.Summary)
	assert.Equal(t, models.AttentionActivity, out.Attention)
	assert.Equal(t, models.LifecycleActive, out.Lifecycle)
	assert.NotEmpty(t, out.OccurrenceKey)
}

func TestClassifyEntersTerminalOnMergeAndClose(t *testing.T) {
	t.Parallel()

	for state, summary := range map[string]string{"merged": "Merged", "closed": "Closed"} {
		t.Run(state, func(t *testing.T) {
			t.Parallel()
			previous := observation(t, searchItem("open", 1000))
			current := observation(t, searchItem(state, 2000))

			out := classifier{}.Classify(&previous, current)

			assert.Equal(t, state, out.Kind)
			assert.Equal(t, summary, out.Summary)
			assert.Equal(t, models.TransitionEnteredTerminal, out.Transition)
			assert.Equal(t, models.LifecycleTerminal, out.Lifecycle)
			assert.Equal(t, state, out.ArchivedReason)
		})
	}
}

func TestClassifyReopen(t *testing.T) {
	t.Parallel()

	previous := observation(t, searchItem("closed", 1000))
	current := observation(t, searchItem("open", 2000))

	out := classifier{}.Classify(&previous, current)

	assert.Equal(t, "reopened", out.Kind)
	assert.Equal(t, models.TransitionLeftTerminal, out.Transition)
	assert.Equal(t, models.LifecycleActive, out.Lifecycle)
}

func TestClassifyLabelChange(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		previous []string
		current  []string
		summary  string
	}{
		"added":   {previous: []string{"bug"}, current: []string{"bug", "p1"}, summary: "Labels added: p1"},
		"removed": {previous: []string{"bug", "p1"}, current: []string{"bug"}, summary: "Labels removed: p1"},
		"swapped": {previous: []string{"bug"}, current: []string{"p1"}, summary: "Labels changed"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			previous := observation(t, searchItem("open", 1000, tc.previous...))
			current := observation(t, searchItem("open", 2000, tc.current...))

			out := classifier{}.Classify(&previous, current)

			assert.Equal(t, "labels", out.Kind)
			assert.Equal(t, tc.summary, out.Summary)
			assert.Equal(t, models.AttentionActivity, out.Attention)
		})
	}
}

// Gitea reports labels in whatever order it holds them; the same set reordered
// is not a change.
func TestClassifyReorderedLabelsAreTrivial(t *testing.T) {
	t.Parallel()

	previous := observation(t, searchItem("open", 1000, "bug", "p1"))
	current := observation(t, searchItem("open", 1000, "p1", "bug"))

	out := classifier{}.Classify(&previous, current)

	assert.Equal(t, models.AttentionTrivial, out.Attention)
	assert.Equal(t, models.TransitionNone, out.Transition)
}

// A notification carries no labels at all. Comparing one against a search
// observation of the same item would report every label removed, then re-added,
// on every tick — Gitea has no notification reason to disambiguate with, so the
// payload records where it came from instead.
func TestClassifyDoesNotCompareLabelsAcrossOrigins(t *testing.T) {
	t.Parallel()

	previous := observation(t, searchItem("open", 1000, "bug", "p1"))
	fromInbox := searchItem("open", 2000)
	fromInbox.Origin, fromInbox.Labels = OriginNotifications, []string{}
	current := observation(t, fromInbox)

	out := classifier{}.Classify(&previous, current)

	assert.Equal(t, "updated", out.Kind, "a notification must not read as every label being removed")
	assert.Equal(t, "Updated on Gitea", out.Summary)
}

func TestClassifyUnchangedItemIsTrivial(t *testing.T) {
	t.Parallel()

	previous := observation(t, searchItem("open", 1000, "bug"))
	current := observation(t, searchItem("open", 1000, "bug"))

	out := classifier{}.Classify(&previous, current)

	assert.Equal(t, models.AttentionTrivial, out.Attention)
	assert.Equal(t, models.TransitionNone, out.Transition)
	assert.Empty(t, out.OccurrenceKey)
}

type stubStates struct {
	gotRefs []itemRef
	states  []itemState
	err     error
}

func (s *stubStates) ItemStates(_ context.Context, refs []itemRef) ([]itemState, error) {
	s.gotRefs = refs
	if s.states != nil {
		return s.states, s.err
	}
	return make([]itemState, len(refs)), s.err
}

func TestConfirmAbsenceArchivesATerminalItem(t *testing.T) {
	t.Parallel()

	previous := []models.Observation{observation(t, searchItem("open", 1000))}
	stub := &stubStates{states: []itemState{{
		Found: true, State: "merged", Title: "Fix the checkout flow",
		URL: "https://git.example.com/acme/app/pulls/42", UpdatedAt: 2000,
	}}}

	verdicts, err := (&absenceConfirmer{fetcher: stub}).ConfirmAbsence(t.Context(), previous)
	require.NoError(t, err)

	assert.Equal(t, []itemRef{{Repo: "acme/app", Num: 42}}, stub.gotRefs)
	verdict, ok := verdicts["acme/app#42"]
	require.True(t, ok)
	assert.True(t, verdict.Terminal)
	require.NotNil(t, verdict.Current)
	assert.Equal(t, "Fix the checkout flow", verdict.Current.Title, "absence hydration must not blank the inbox row")

	var hydrated Item
	require.NoError(t, json.Unmarshal(verdict.Current.Payload, &hydrated))
	assert.Equal(t, "merged", hydrated.State)
	assert.Equal(t, int64(2000), hydrated.UpdatedAt)
}

// An item that merely left the result set — aged out, or no longer matching the
// filters — is still open, so it must not be archived.
func TestConfirmAbsenceKeepsAStillOpenItem(t *testing.T) {
	t.Parallel()

	previous := []models.Observation{observation(t, searchItem("open", 1000))}
	stub := &stubStates{states: []itemState{{Found: true, State: "open", UpdatedAt: 1500}}}

	verdicts, err := (&absenceConfirmer{fetcher: stub}).ConfirmAbsence(t.Context(), previous)
	require.NoError(t, err)

	verdict := verdicts["acme/app#42"]
	assert.False(t, verdict.Terminal)
}

// A deleted item, or one whose repository the token lost access to, yields no
// verdict: the flow's resurface policy decides, rather than a failed lookup
// archiving something that may still be live.
func TestConfirmAbsenceYieldsNoVerdictForAnUnresolvableItem(t *testing.T) {
	t.Parallel()

	previous := []models.Observation{observation(t, searchItem("open", 1000))}
	stub := &stubStates{states: []itemState{{Found: false}}}

	verdicts, err := (&absenceConfirmer{fetcher: stub}).ConfirmAbsence(t.Context(), previous)
	require.NoError(t, err)
	assert.Empty(t, verdicts)
}

func TestConfirmAbsenceSkipsUnaddressableItemsAndPropagatesErrors(t *testing.T) {
	t.Parallel()

	previous := []models.Observation{
		observation(t, Item{ID: "no-repo#1", Num: 1}),
		observation(t, Item{ID: "acme/app#0", Repo: "acme/app"}),
		{ExternalID: "not-json", Payload: []byte("{")},
	}
	stub := &stubStates{err: errors.New("boom")}

	verdicts, err := (&absenceConfirmer{fetcher: stub}).ConfirmAbsence(t.Context(), previous)

	assert.Empty(t, stub.gotRefs, "nothing addressable was sent")
	assert.Empty(t, verdicts)
	assert.ErrorContains(t, err, "boom")
}
