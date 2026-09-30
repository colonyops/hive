package app

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/queries"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/stores"
)

func TestKindOf_WalksAWrappedChain(t *testing.T) {
	t.Parallel()

	cause := errors.New("disk on fire")
	classified := Wrap(cause, KindUnavailable, "reading the catalog")
	// A service returning a typed error that a caller then wraps again with
	// fmt.Errorf is the normal shape: KindOf has to see through it.
	outer := fmt.Errorf("loading actions: %w", classified)

	assert.Equal(t, KindUnavailable, KindOf(outer))
	assert.ErrorIs(t, outer, cause, "the cause stays reachable")
}

func TestKindOf_UnclassifiedIsInternal(t *testing.T) {
	t.Parallel()

	assert.Equal(t, KindInternal, KindOf(errors.New("plain")))
	assert.Equal(t, KindInternal, KindOf(nil))
}

func TestWrap_NilErrorIsNil(t *testing.T) {
	t.Parallel()

	// Returning Wrap(err, ...) unconditionally has to stay safe, or every
	// call site grows an if — and it has to be nil through an error-typed
	// return, not a non-nil interface holding a nil pointer. Wrap returning
	// *Error passes an errors.Is check and still reports every success as a
	// failure.
	passthrough := func() error { return Wrap(nil, KindInvalid, "unused") }
	require.NoError(t, passthrough())
}

func TestError_MessageAndJSON(t *testing.T) {
	t.Parallel()

	bare := Errorf(KindNotFound, "action %q not found", "review-pr")
	assert.Equal(t, `action "review-pr" not found`, bare.Error())

	wrapped := Wrap(errors.New("no such column"), KindInternal, "listing runs")
	assert.Equal(t, "listing runs: no such column", wrapped.Error())

	encoded, err := json.Marshal(wrapped)
	require.NoError(t, err)

	var decoded struct {
		Kind    Kind   `json:"kind"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	assert.Equal(t, KindInternal, decoded.Kind)
	assert.Equal(t, "listing runs", decoded.Message)
	assert.NotContains(t, string(encoded), "no such column", "the cause is for the log, not the wire")
}

// TestRerunOutputCommand_NoPriorRunUnwraps guards the asymmetry this commit
// fixed: the store converted sql.ErrNoRows with no %w, so KindOf could only
// ever report internal for a case the user can act on.
func TestRerunOutputCommand_NoPriorRunUnwraps(t *testing.T) {
	t.Parallel()

	db, err := queries.Open(t.Context(), t.TempDir(), queries.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = stores.New(db, stores.Options{}).OutputCommands.Rerun(t.Context(), "review-pr", "item-1", nil, models.ItemRef{})
	require.Error(t, err)
	require.ErrorIs(t, err, sql.ErrNoRows)
	assert.True(t, stores.IsNotFound(err))

	// Which is what lets the boundary classify it.
	classified := Wrap(err, KindInvalid, "rerunning action")
	assert.Equal(t, KindInvalid, KindOf(classified))
}
