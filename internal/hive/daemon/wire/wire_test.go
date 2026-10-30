package wire_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/colonyops/hive/internal/hive/daemon/wire"
	"github.com/colonyops/hive/internal/hive/gitstatus"
	"github.com/colonyops/hive/internal/platform/git"
)

func TestChangesSurviveTheWire(t *testing.T) {
	t.Parallel()

	changes := gitstatus.Changes{
		Resolved:        true,
		Branch:          "feat/x",
		DefaultBranch:   "main",
		Base:            gitstatus.DiffBaseBranch,
		BranchAvailable: true,
		Files: []git.ChangedFile{
			{Path: "new.go", OldPath: "old.go", Status: git.FileStatusRenamed, Additions: 3, Deletions: 1},
			{Path: "image.png", Status: git.FileStatusModified, Binary: true},
		},
		Truncated: true,
	}
	assert.Equal(t, changes, wire.ChangesFromProto(wire.ChangesToProto(changes)))
}
