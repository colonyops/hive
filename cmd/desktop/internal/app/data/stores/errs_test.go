package stores

import (
	"database/sql"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestErrTransformQueryOne_WrapsNotFoundAndUnwrapsToSqlErrNoRows(t *testing.T) {
	err := errTransformQueryOne("widget", "42", sql.ErrNoRows)
	require.Error(t, err)
	assert.True(t, IsNotFound(err))
	require.ErrorIs(t, err, sql.ErrNoRows, "a caller still matching on sql.ErrNoRows must keep working")
	assert.Contains(t, err.Error(), "widget")
	assert.Contains(t, err.Error(), "42")
}

func TestErrTransformQueryOne_PassesThroughOtherErrorsAndNil(t *testing.T) {
	require.NoError(t, errTransformQueryOne("widget", "42", nil))

	other := fmt.Errorf("disk on fire")
	err := errTransformQueryOne("widget", "42", other)
	assert.Same(t, other, err)
	assert.False(t, IsNotFound(err))
}

func TestErrTransformQueryMany_SwallowsNoRowsPassesOthers(t *testing.T) {
	assert.NoError(t, errTransformQueryMany(sql.ErrNoRows))
	assert.NoError(t, errTransformQueryMany(nil))

	other := fmt.Errorf("disk on fire")
	assert.Same(t, other, errTransformQueryMany(other))
}

// A stale revision must not satisfy IsNotFound, or callers can mistake a
// retryable conflict for a deleted row.
func TestErrStale_IsNotANotFoundError(t *testing.T) {
	assert.False(t, IsNotFound(ErrStale))
	assert.False(t, IsNotFound(fmt.Errorf("wrapped: %w", ErrStale)))
}

func TestIsNotFound_FalseForNilAndUnrelatedErrors(t *testing.T) {
	assert.False(t, IsNotFound(nil))
	assert.False(t, IsNotFound(errors.New("plain")))
}

func TestWrap_NilInNilOut(t *testing.T) {
	require.NoError(t, wrap("doing a thing", nil))
	assert.EqualError(t, wrap("doing a thing", errors.New("boom")), "doing a thing: boom")
}
