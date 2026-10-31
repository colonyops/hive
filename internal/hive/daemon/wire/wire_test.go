package wire_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	daemonv1 "github.com/colonyops/hive/internal/hive/daemon/gen/hive/daemon/v1"
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
		Against:         "refs/heads/release",
		Commit:          "abc123",
		Commits:         []git.CommitInfo{{ID: "abc123", Subject: "fix it", Author: "t", Unix: 42}},
		Files: []git.ChangedFile{
			{Path: "new.go", OldPath: "old.go", Status: git.FileStatusRenamed, Additions: 3, Deletions: 1, Stamp: "1-2"},
			{Path: "image.png", Status: git.FileStatusModified, Binary: true},
		},
		Truncated: true,
	}
	assert.Equal(t, changes, wire.ChangesFromProto(wire.ChangesToProto(changes)))
}

func TestDiffTargetSurvivesTheWire(t *testing.T) {
	t.Parallel()

	target := gitstatus.DiffTarget{Base: gitstatus.DiffBaseBranch, Against: "refs/heads/release", Commit: "abc123"}
	assert.Equal(t, target, wire.DiffTargetFromProto(wire.DiffTargetToProto(target)))
}

func TestFileDiffsSurviveTheWire(t *testing.T) {
	t.Parallel()

	diffs := []gitstatus.FileDiff{
		{Path: "a.go", State: gitstatus.FileDiffStateText, Text: "@@ -1 +1 @@\n+x\n"},
		{Path: "b.png", State: gitstatus.FileDiffStateBinary},
		{Path: "c.json", State: gitstatus.FileDiffStateTruncated},
		{Path: "pipe", State: gitstatus.FileDiffStateUnavailable},
		{Path: "gone.txt", State: gitstatus.FileDiffStateGone},
		{Path: "d.go", State: gitstatus.FileDiffStateError, Error: "exit 128"},
	}
	assert.Equal(t, diffs, wire.FileDiffsFromProto(wire.FileDiffsToProto(diffs)))
}

func TestInvalidUTF8CanCrossTheWire(t *testing.T) {
	t.Parallel()

	const latin1 = "caf\xe9"
	changes := wire.ChangesToProto(gitstatus.Changes{
		Branch:        latin1,
		DefaultBranch: latin1,
		Against:       "refs/heads/" + latin1,
		Commits:       []git.CommitInfo{{ID: "abc123", Subject: latin1, Author: latin1}},
		Files:         []git.ChangedFile{{Path: latin1, OldPath: latin1, Status: git.FileStatusRenamed}},
	})
	_, err := proto.Marshal(changes)
	require.NoError(t, err)

	_, err = proto.Marshal(&daemonv1.BranchesResponse{Refs: wire.RefsToProto([]string{"refs/heads/" + latin1})})
	require.NoError(t, err)
}
