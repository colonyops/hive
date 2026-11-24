package wire

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckProtocol(t *testing.T) {
	require.NoError(t, CheckProtocol(ProtocolVersion, "v1", "v1"))
	require.NoError(t, CheckProtocol(0, "v0.60.0", "v0.68.0"), "a daemon from before the field")

	for _, protocol := range []int{MinProtocolVersion - 1, ProtocolVersion + 1} {
		err := CheckProtocol(protocol, "v0.60.0", "v0.68.0")
		var incompatible *IncompatibleError
		require.ErrorAs(t, err, &incompatible, "protocol %d", protocol)
		assert.Contains(t, err.Error(), "v0.60.0")
		assert.Contains(t, err.Error(), "v0.68.0")
	}
}

func TestCheckCatalog(t *testing.T) {
	require.NoError(t, CheckCatalog(CatalogVersion, "v1", "v1"))

	for _, catalog := range []int{0, MinCatalogVersion - 1, CatalogVersion + 1} {
		err := CheckCatalog(catalog, "v0.60.0", "v0.68.0")
		var refused *CatalogError
		require.ErrorAs(t, err, &refused, "catalog %d", catalog)
		assert.Contains(t, err.Error(), "v0.60.0")
		assert.Contains(t, err.Error(), "v0.68.0")
	}
	assert.Contains(t, CheckCatalog(0, "v0.60.0", "v0.68.0").Error(), "serves no engine operations")
}
