package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/canonical"
)

func TestConfirmAbsence_ResolvesEveryDepartedItem(t *testing.T) {
	prev := []models.Observation{
		{ExternalID: "a", Title: "Alpha", Payload: []byte(`{"id":"a","title":"Alpha","state":"open"}`)},
		{ExternalID: "b", Title: "Beta", Payload: []byte(`{"id":"b","title":"Beta"}`)},
	}

	verdicts, err := absence{}.ConfirmAbsence(t.Context(), prev)
	require.NoError(t, err)

	require.Len(t, verdicts, 2)
	for _, id := range []string{"a", "b"} {
		verdict := verdicts[id]
		require.NotNil(t, verdict.Current, "item %q", id)
		assert.True(t, verdict.Terminal, "item %q left an authoritative snapshot, so it is gone", id)
		assert.Equal(t, canonical.TerminalState, canonical.State(verdict.Current.Payload))
	}
	// The archived item still renders, so the rewrite keeps what it renders from.
	assert.JSONEq(t, `{"id":"a","title":"Alpha","state":"resolved"}`, string(verdicts["a"].Current.Payload))
}

func TestConfirmAbsence_NothingAbsentIsNoVerdicts(t *testing.T) {
	verdicts, err := absence{}.ConfirmAbsence(t.Context(), nil)

	require.NoError(t, err)
	assert.Empty(t, verdicts)
}
