package httpapi

import (
	"os"
	"testing"

	"github.com/colonyops/hive/internal/platform/tmuxtest"
)

func TestMain(m *testing.M) {
	os.Exit(tmuxtest.Run(m))
}
