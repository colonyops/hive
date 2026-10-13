package drive

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSpaceKeyRoundTrips(t *testing.T) {
	repo, err := RepositorySpace("ColonyOps", "Hive")
	require.NoError(t, err)
	ws, err := WorkspaceSpace("research")
	require.NoError(t, err)

	tests := []struct {
		key  string
		want Space
	}{
		{"shared", Shared},
		{"ColonyOps/Hive", repo},
		{"workspaces/research", ws},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			got, err := ParseSpaceKey(tt.key)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.key, got.Key, "the key is the space root's path under the drive root")
		})
	}
	assert.Equal(t, Space{Kind: SpaceKindRepository, Key: "ColonyOps/Hive", Name: "ColonyOps/Hive"}, repo, "case is kept")
	assert.Equal(t, Space{Kind: SpaceKindWorkspace, Key: "workspaces/research", Name: "research"}, ws)
}

func TestParseSpaceKeyRejects(t *testing.T) {
	for _, key := range []string{
		"", "/", "a", "a/b/c", "shared/x", "workspaces", "workspaces/", "workspaces/a/b",
		"../x", "a/..", "a/.", "./b", "/b", "a/", "Shared", "SHARED/x",
	} {
		t.Run(key, func(t *testing.T) {
			_, err := ParseSpaceKey(key)
			require.ErrorIs(t, err, ErrInvalidSpace)
		})
	}
}

func TestRepositorySpaceRefusesReservedOwners(t *testing.T) {
	for _, owner := range []string{WorkspacesDir, SharedKey} {
		_, err := RepositorySpace(owner, "x")
		require.ErrorIs(t, err, ErrInvalidSpace, "owner %q", owner)
	}
	for _, parts := range [][2]string{{"", "x"}, {"x", ""}, {".", "x"}, {"a/b", "x"}, {"x", "a/b"}, {"..", "x"}} {
		_, err := RepositorySpace(parts[0], parts[1])
		require.ErrorIs(t, err, ErrInvalidSpace, "%q/%q", parts[0], parts[1])
	}
}

func TestWorkspaceSpaceHoldsToOneComponent(t *testing.T) {
	for _, name := range []string{"", ".", "..", "a/b", "../escape", "/abs"} {
		_, err := WorkspaceSpace(name)
		require.ErrorIs(t, err, ErrInvalidSpace, "name %q", name)
	}
	got, err := WorkspaceSpace("Hive Notes")
	require.NoError(t, err)
	assert.Equal(t, "workspaces/Hive Notes", got.Key)
}

func TestSplitSpacePath(t *testing.T) {
	repo := must(RepositorySpace("colonyops", "hive"))
	ws := must(WorkspaceSpace("hive"))
	tests := []struct {
		rel   string
		space Space
		path  string
	}{
		{"shared", Shared, ""},
		{"shared/notes/a.md", Shared, "notes/a.md"},
		{"workspaces/hive", ws, ""},
		{"workspaces/hive/canvases/a.canvas.json", ws, "canvases/a.canvas.json"},
		{"colonyops/hive", repo, ""},
		{"colonyops/hive/plans/x.md", repo, "plans/x.md"},
	}
	for _, tt := range tests {
		t.Run(tt.rel, func(t *testing.T) {
			space, path, err := SplitSpacePath(tt.rel)
			require.NoError(t, err)
			assert.Equal(t, tt.space, space)
			assert.Equal(t, tt.path, path)
		})
	}
	for _, rel := range []string{"", ".", "colonyops", "workspaces", "workspaces/", "Shared/x", "../x/y"} {
		t.Run("above a space "+rel, func(t *testing.T) {
			_, _, err := SplitSpacePath(rel)
			require.ErrorIs(t, err, ErrInvalidSpace)
		})
	}
}

func must(space Space, err error) Space {
	if err != nil {
		panic(err)
	}
	return space
}
