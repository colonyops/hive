package core_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/drive-server/internal/config"
	"github.com/colonyops/hive/cmd/drive-server/internal/core"
	"github.com/colonyops/hive/internal/domain/drive"
)

func principalFor(t *testing.T, e *env, email string) core.Principal {
	t.Helper()
	acct, _, err := e.accounts.Ensure(t.Context(), email, false)
	require.NoError(t, err)
	pol, ok := e.policies.Match(email)
	require.True(t, ok)
	return core.Principal{UserID: acct.ID, Email: email, Scope: core.ScopePush, Policy: pol}
}

func quotaPolicy(size config.Size) config.Policy {
	return config.Policy{Name: "dev", Matches: []string{"*@example.com"}, Quotas: config.Quotas{Limit: size}}
}

func TestPutRejectsOtherKindsAndBadPaths(t *testing.T) {
	e := newEnv(t, devPolicy())
	p := principalFor(t, e, "dev@example.com")

	for _, path := range []string{"notes.txt", "board.canvas.json", "data.json", "noext"} {
		_, err := e.docs.Put(t.Context(), p, drive.Shared, path, []byte("x"))
		requireKind(t, err, core.KindInvalid)
	}
	for _, path := range []string{"", "../a.md", "/a.md", ".hidden/a.md", "a//b.md"} {
		_, err := e.docs.Put(t.Context(), p, drive.Shared, path, []byte("x"))
		requireKind(t, err, core.KindInvalid)
	}

	for _, path := range []string{"a.md", "dir/b.HTML"} {
		_, err := e.docs.Put(t.Context(), p, drive.Shared, path, []byte("x"))
		require.NoError(t, err, path)
	}
}

func TestPutSizeCap(t *testing.T) {
	e := newEnv(t, devPolicy())
	p := principalFor(t, e, "dev@example.com")

	doc, err := e.docs.Put(t.Context(), p, drive.Shared, "a.md", []byte(strings.Repeat("x", testMaxDocumentSize)))
	require.NoError(t, err)
	assert.EqualValues(t, testMaxDocumentSize, doc.Size)

	_, err = e.docs.Put(t.Context(), p, drive.Shared, "b.md", []byte(strings.Repeat("x", testMaxDocumentSize+1)))
	requireKind(t, err, core.KindTooLarge)
	_, _, err = e.docs.Get(t.Context(), p, drive.Shared, "b.md")
	requireKind(t, err, core.KindNotFound)
}

func TestPutQuotaCountsOnlyTheDeltaOnReupload(t *testing.T) {
	e := newEnv(t, quotaPolicy(config.Size{Bytes: 100}))
	p := principalFor(t, e, "dev@example.com")
	body := func(n int) []byte { return []byte(strings.Repeat("x", n)) }

	_, err := e.docs.Put(t.Context(), p, drive.Shared, "a.md", body(60))
	require.NoError(t, err)
	_, err = e.docs.Put(t.Context(), p, drive.Shared, "a.md", body(100))
	require.NoError(t, err, "replacing 60 bytes with 100 stays within 100")

	_, err = e.docs.Put(t.Context(), p, drive.Shared, "b.md", body(1))
	requireKind(t, err, core.KindTooLarge)

	_, err = e.docs.Put(t.Context(), p, drive.Shared, "a.md", body(101))
	requireKind(t, err, core.KindTooLarge)
	_, got, err := e.docs.Get(t.Context(), p, drive.Shared, "a.md")
	require.NoError(t, err)
	assert.Len(t, got, 100, "a rejected push changes nothing")

	used, err := e.docs.Usage(t.Context(), p)
	require.NoError(t, err)
	assert.EqualValues(t, 100, used)
}

func TestPutUnlimitedSkipsQuota(t *testing.T) {
	e := newEnv(t, quotaPolicy(config.Size{Unlimited: true}))
	p := principalFor(t, e, "dev@example.com")
	for _, name := range []string{"a.md", "b.md", "c.md"} {
		_, err := e.docs.Put(t.Context(), p, drive.Shared, name, []byte(strings.Repeat("x", testMaxDocumentSize)))
		require.NoError(t, err)
	}
}

func TestPutKeepsIDOnReupload(t *testing.T) {
	e := newEnv(t, devPolicy())
	p := principalFor(t, e, "dev@example.com")

	first, err := e.docs.Put(t.Context(), p, drive.Shared, "a.md", []byte("one"))
	require.NoError(t, err)
	e.clock.Advance(1)
	second, err := e.docs.Put(t.Context(), p, drive.Shared, "a.md", []byte("two"))
	require.NoError(t, err)

	assert.Equal(t, first.ID, second.ID)
	assert.NotEqual(t, first.SHA256, second.SHA256)
	assert.True(t, second.UpdatedAt.After(first.UpdatedAt))

	other, err := e.docs.Put(t.Context(), p, drive.Shared, "b.md", []byte("two"))
	require.NoError(t, err)
	assert.NotEqual(t, first.ID, other.ID)
}

func TestLoweredQuotaBlocksPushNotRead(t *testing.T) {
	e := newEnv(t, quotaPolicy(config.Size{Bytes: 100}))
	p := principalFor(t, e, "dev@example.com")
	_, err := e.docs.Put(t.Context(), p, drive.Shared, "a.md", []byte(strings.Repeat("x", 80)))
	require.NoError(t, err)

	e.policies.Swap([]config.Policy{quotaPolicy(config.Size{Bytes: 10})})
	lowered := principalFor(t, e, "dev@example.com")

	_, err = e.docs.Put(t.Context(), lowered, drive.Shared, "b.md", []byte("x"))
	requireKind(t, err, core.KindTooLarge)
	_, _, err = e.docs.Get(t.Context(), lowered, drive.Shared, "a.md")
	require.NoError(t, err)
	list, err := e.docs.List(t.Context(), lowered, drive.Shared)
	require.NoError(t, err)
	assert.Len(t, list, 1)
}

func TestSpacesListDeleteAndIsolation(t *testing.T) {
	e := newEnv(t, devPolicy())
	a := principalFor(t, e, "a@example.com")
	b := principalFor(t, e, "b@example.com")
	repo, err := drive.RepositorySpace("colonyops", "hive")
	require.NoError(t, err)
	ws, err := drive.WorkspaceSpace("w")
	require.NoError(t, err)

	for _, s := range []drive.Space{drive.Shared, repo, ws} {
		_, err := e.docs.Put(t.Context(), a, s, "dir/a.md", []byte("aaa"))
		require.NoError(t, err)
	}

	spaces, err := e.docs.Spaces(t.Context(), a)
	require.NoError(t, err)
	require.Len(t, spaces, 3)
	kinds := map[string]drive.SpaceKind{}
	for _, s := range spaces {
		kinds[s.Space] = s.Kind
		assert.Equal(t, 1, s.Documents)
		assert.EqualValues(t, 3, s.Bytes)
	}
	assert.Equal(t, map[string]drive.SpaceKind{
		"shared": drive.SpaceKindShared, "colonyops/hive": drive.SpaceKindRepository, "workspaces/w": drive.SpaceKindWorkspace,
	}, kinds)

	bSpaces, err := e.docs.Spaces(t.Context(), b)
	require.NoError(t, err)
	assert.Empty(t, bSpaces)
	requireKind(t, e.docs.Delete(t.Context(), b, drive.Shared, "dir/a.md"), core.KindNotFound)

	require.NoError(t, e.docs.Delete(t.Context(), a, drive.Shared, "dir/a.md"))
	requireKind(t, e.docs.Delete(t.Context(), a, drive.Shared, "dir/a.md"), core.KindNotFound)
	requireKind(t, e.docs.Delete(t.Context(), a, drive.Shared, "../a.md"), core.KindInvalid)
}

func TestAccountDeleteRemovesUserFile(t *testing.T) {
	e := newEnv(t, devPolicy())
	p := principalFor(t, e, "dev@example.com")
	_, err := e.docs.Put(t.Context(), p, drive.Shared, "a.md", []byte("x"))
	require.NoError(t, err)

	require.NoError(t, e.accounts.Delete(t.Context(), "dev@example.com"))

	entries, err := os.ReadDir(filepath.Join(e.dataDir, "users"))
	require.NoError(t, err)
	assert.Empty(t, entries)
}
