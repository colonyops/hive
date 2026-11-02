package settings_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/settings"
)

func TestClampScale(t *testing.T) {
	t.Parallel()

	require.Equal(t, settings.DefaultScale, settings.ClampScale(0, 75, 150), "zero is nothing persisted")
	require.Equal(t, 75, settings.ClampScale(10, 75, 150))
	require.Equal(t, 150, settings.ClampScale(900, 75, 150))
	require.Equal(t, 115, settings.ClampScale(115, 75, 150))
}
