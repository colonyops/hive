package wire_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

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
			{Path: "new.go", OldPath: "old.go", Status: git.FileStatusRenamed, Additions: 3, Deletions: 1, Stamp: "1-2"},
			{Path: "image.png", Status: git.FileStatusModified, Binary: true},
		},
		Truncated: true,
	}
	assert.Equal(t, changes, wire.ChangesFromProto(wire.ChangesToProto(changes)))
}

func TestFileDiffsSurviveTheWire(t *testing.T) {
	t.Parallel()

	diffs := []gitstatus.FileDiff{
		{Path: "a.go", Text: "@@ -1 +1 @@\n+x\n"},
		{Path: "b.png", Binary: true},
		{Path: "c.json", Truncated: true},
		{Path: "pipe", Unavailable: true},
		{Path: "gone.txt", Gone: true},
		{Path: "d.go", Error: "exit 128"},
	}
	assert.Equal(t, diffs, wire.FileDiffsFromProto(wire.FileDiffsToProto(diffs)))
}

func TestInvalidUTF8PathsCanCrossTheWire(t *testing.T) {
	t.Parallel()

	changes := wire.ChangesToProto(gitstatus.Changes{Files: []git.ChangedFile{{Path: "caf\xe9.txt", Status: git.FileStatusUntracked}}})
	_, err := proto.Marshal(changes)
	require.NoError(t, err)
}
