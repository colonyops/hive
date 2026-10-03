package configmigrate

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestLeafPackage_DoesNotImportDomainPackages guards the architectural
// constraint that configmigrate spans settings/flow/actions and therefore must
// not import any of them back, or package app: it is a leaf that they import,
// never the reverse.
func TestLeafPackage_DoesNotImportDomainPackages(t *testing.T) {
	const pkg = "github.com/colonyops/hive/cmd/desktop/internal/app/configmigrate"

	out, err := exec.Command("go", "list", "-deps", pkg).CombinedOutput()
	require.NoError(t, err, "go list -deps failed: %s", out)

	deps := strings.Fields(string(out))

	forbidden := []string{
		"github.com/colonyops/hive/cmd/desktop/internal/app",
		"github.com/colonyops/hive/cmd/desktop/internal/app/settings",
		"github.com/colonyops/hive/cmd/desktop/internal/app/flow",
		"github.com/colonyops/hive/cmd/desktop/internal/app/actions",
		"github.com/colonyops/hive/cmd/desktop/internal/app/agentws",
	}

	for _, dep := range deps {
		for _, f := range forbidden {
			if dep == f {
				t.Errorf("configmigrate must not depend on %s (leaf package rule); full deps:\n%s", f, out)
			}
		}
	}
}
