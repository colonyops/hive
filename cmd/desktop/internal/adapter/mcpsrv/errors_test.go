package mcpsrv

import (
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
	"github.com/colonyops/hive/internal/ops"
)

// hive dt picks its exit code from the kind a tool error leads with, so every
// app kind must parse as an ops kind of the same name.
func TestEveryAppKindParsesAsAnOpsKind(t *testing.T) {
	for _, k := range app.AllKinds() {
		err := toolError(zerolog.Nop(), app.Errorf(k, "refused"))
		got, ok := ops.ParseKind(err.Error())
		if assert.True(t, ok, "app kind %q is not an ops kind", k) {
			assert.Equal(t, string(k), string(got))
		}
	}
}
