package exec

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/connector"
	"github.com/colonyops/hive/pkg/duration"
)

func TestFactory_BuildsAnInstanceFromTheConfig(t *testing.T) {
	factory := NewFactory(hostEnvironment{})
	cfg := &Config{
		Command:  "echo '[]'",
		Timeout:  duration.Duration(30 * time.Second),
		Interval: duration.Duration(time.Hour),
		Cwd:      "/tmp",
		Env:      map[string]string{"TOKEN": "x"},
	}

	instance, err := factory.New(connector.Node{FlowID: "oncall", NodeID: "src"}, cfg)
	require.NoError(t, err)

	assert.Equal(t, Descriptor.Type, instance.Type)
	assert.Equal(t, SourceKind, instance.Metadata.SourceKind)
	assert.Equal(t, "src", instance.Metadata.SourceScope, "several exec sources can share a flow")
	assert.Equal(t, time.Hour, instance.MinInterval, "the config's interval is the producer's cadence floor")

	pull, ok := instance.Pull.(*source)
	require.True(t, ok)
	assert.Equal(t, "source:oncall/src", pull.topic)
	assert.Equal(t, 30*time.Second, pull.timeout)
	assert.Equal(t, "/tmp", pull.cwd)
	assert.Equal(t, map[string]string{"TOKEN": "x"}, pull.env)
}

func TestFactory_RejectsAnotherConnectorsConfig(t *testing.T) {
	_, err := NewFactory(hostEnvironment{}).New(connector.Node{FlowID: "f", NodeID: "n"}, &wrongConfig{})

	require.ErrorContains(t, err, "want *exec.Config")
}

type wrongConfig struct{}

func (*wrongConfig) Validate() error { return nil }
