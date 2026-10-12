package memory_test

import (
	"testing"

	"github.com/colonyops/hive/cmd/relay/internal/relay"
	"github.com/colonyops/hive/cmd/relay/internal/store/memory"
	"github.com/colonyops/hive/cmd/relay/internal/store/storetest"
)

func TestContract(t *testing.T) {
	storetest.Run(t, func(t *testing.T) relay.Store { return memory.New() })
}
