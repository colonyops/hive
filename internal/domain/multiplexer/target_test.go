package multiplexer

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTargetValidation(t *testing.T) {
	require.NoError(t, (Target{Session: "s"}).ValidateSession())
	require.NoError(t, (Target{Session: "s", Window: "1"}).ValidateWindow())
	require.NoError(t, (Target{Session: "s", Window: "1", Pane: "2"}).ValidatePane())
	require.NoError(t, (Target{Pane: "%7"}).ValidatePane())
	require.Error(t, (Target{}).ValidateSession())
	require.Error(t, (Target{Session: "s", Window: "1"}).ValidateSession())
	require.Error(t, (Target{Session: "s", Window: "1", Pane: "2"}).ValidateWindow())
	require.Error(t, (Target{Session: "s", Pane: "%7"}).ValidatePane())
	require.Error(t, (Target{Pane: "bad\x00id"}).ValidatePane())
	require.Error(t, (Target{Session: "bad\x00name"}).ValidateSession())
}
