package git_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/platform/git"
)

func fileVersions(t *testing.T, g *git.Executor, dir string, diff git.DiffRange, path string) git.FileVersions {
	t.Helper()
	files, err := g.ChangedFiles(t.Context(), dir, diff)
	require.NoError(t, err)
	for _, file := range files {
		if file.Path == path {
			versions, err := g.FileVersions(t.Context(), dir, diff, file, 1<<20)
			require.NoError(t, err)
			return versions
		}
	}
	require.FailNow(t, "path not in the change list", path)
	return git.FileVersions{}
}

func TestFileVersionsReadsBothSidesOfTheWorkingTree(t *testing.T) {
	t.Parallel()
	dir, g := newRepo(t)
	writeFile(t, dir, "image.bin", "\x00\x02")
	writeFile(t, dir, "fresh.bin", "\x00\x03")
	runGit(t, dir, "rm", "-q", "gone.txt")
	runGit(t, dir, "mv", "moved.txt", "renamed.txt")

	diff := git.WorkingTree("HEAD")
	modified := fileVersions(t, g, dir, diff, "image.bin")
	assert.Equal(t, []byte("\x00\x01"), modified.Before)
	assert.Equal(t, []byte("\x00\x02"), modified.After)

	added := fileVersions(t, g, dir, diff, "fresh.bin")
	assert.Nil(t, added.Before)
	assert.Equal(t, []byte("\x00\x03"), added.After)

	deleted := fileVersions(t, g, dir, diff, "gone.txt")
	assert.Equal(t, []byte("bye\n"), deleted.Before)
	assert.Nil(t, deleted.After)

	renamed := fileVersions(t, g, dir, diff, "renamed.txt")
	assert.Equal(t, renamed.Before, renamed.After, "a rename reads its old path on the before side")
}

func TestFileVersionsFollowsTheIndexForStagedAndUnstaged(t *testing.T) {
	t.Parallel()
	dir, g := newRepo(t)
	writeFile(t, dir, "image.bin", "staged")
	runGit(t, dir, "add", "image.bin")
	writeFile(t, dir, "image.bin", "unstaged")

	staged := fileVersions(t, g, dir, git.Staged(), "image.bin")
	assert.Equal(t, []byte("\x00\x01"), staged.Before)
	assert.Equal(t, []byte("staged"), staged.After)

	unstaged := fileVersions(t, g, dir, git.Unstaged(), "image.bin")
	assert.Equal(t, []byte("staged"), unstaged.Before)
	assert.Equal(t, []byte("unstaged"), unstaged.After)
}

func TestFileVersionsReadsACommit(t *testing.T) {
	t.Parallel()
	dir, g := newRepo(t)
	writeFile(t, dir, "image.bin", "committed")
	runGit(t, dir, "commit", "-qam", "second")
	writeFile(t, dir, "image.bin", "dirty")

	head, err := g.ResolveCommit(t.Context(), dir, "HEAD")
	require.NoError(t, err)
	diff, err := g.CommitRange(t.Context(), dir, head)
	require.NoError(t, err)
	versions := fileVersions(t, g, dir, diff, "image.bin")
	assert.Equal(t, []byte("\x00\x01"), versions.Before)
	assert.Equal(t, []byte("committed"), versions.After)
}

func TestFileVersionsKeepsAnEmptyFileApartFromAnAbsentOne(t *testing.T) {
	t.Parallel()
	dir, g := newRepo(t)
	writeFile(t, dir, "image.bin", "")
	runGit(t, dir, "commit", "-qam", "empty")
	writeFile(t, dir, "image.bin", "\x00")

	versions := fileVersions(t, g, dir, git.WorkingTree("HEAD"), "image.bin")
	assert.NotNil(t, versions.Before)
	assert.Empty(t, versions.Before)
}

func TestFileVersionsRefusesASymlinkInTheWorkingTree(t *testing.T) {
	t.Parallel()
	dir, g := newRepo(t)
	outside := filepath.Join(t.TempDir(), "secret.png")
	require.NoError(t, os.WriteFile(outside, []byte("outside the checkout"), 0o600))
	require.NoError(t, os.Remove(filepath.Join(dir, "image.bin")))
	require.NoError(t, os.Symlink(outside, filepath.Join(dir, "image.bin")))

	files, err := g.ChangedFiles(t.Context(), dir, git.WorkingTree("HEAD"))
	require.NoError(t, err)
	require.Len(t, files, 1)

	_, err = g.FileVersions(t.Context(), dir, git.WorkingTree("HEAD"), files[0], 1<<20)
	require.ErrorIs(t, err, git.ErrNoTextDiff)
}

func TestFileVersionsRefusesAFileOverTheLimit(t *testing.T) {
	t.Parallel()
	dir, g := newRepo(t)
	writeFile(t, dir, "image.bin", "0123456789")
	files, err := g.ChangedFiles(t.Context(), dir, git.WorkingTree("HEAD"))
	require.NoError(t, err)
	require.Len(t, files, 1)

	_, err = g.FileVersions(t.Context(), dir, git.WorkingTree("HEAD"), files[0], 4)
	require.ErrorIs(t, err, git.ErrFileTooLarge)
}
