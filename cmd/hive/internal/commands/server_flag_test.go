package commands

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/hive/internal/app"
)

func TestTargetServerDoesNothingDuringShellCompletion(t *testing.T) {
	name := "box"

	_, err := targetServer(&app.App{}, &name)(t.Context(), nil)

	require.NoError(t, err)
}
