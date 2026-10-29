package wire

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckProtocol(t *testing.T) {
	require.NoError(t, CheckProtocol(ProtocolVersion, "v1", "v1"))

	for _, protocol := range []int{0, ProtocolVersion + 1} {
		err := CheckProtocol(protocol, "v0.60.0", "v0.68.0")
		var incompatible *IncompatibleError
		require.ErrorAs(t, err, &incompatible, "protocol %d", protocol)
		assert.Contains(t, err.Error(), "v0.60.0")
		assert.Contains(t, err.Error(), "v0.68.0")
	}
}
