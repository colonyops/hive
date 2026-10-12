package mcpsrv

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCanvasOwnerNamesTheLegacyOwnerKey(t *testing.T) {
	for key, owner := range map[string]string{
		"shared":          "@global",
		"workspaces/demo": "demo",
		"acme/site":       "acme/site",
	} {
		assert.Equal(t, owner, canvasOwner(key), key)
	}
}
