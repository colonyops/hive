package core_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/drive-server/internal/config"
	"github.com/colonyops/hive/cmd/drive-server/internal/core"
	"github.com/colonyops/hive/internal/domain/drive"
	"github.com/colonyops/hive/internal/platform/driveapi"
)

func sharedDoc(t *testing.T, e *env, p core.Principal, path, expiresIn string) (driveapi.Share, string) {
	t.Helper()
	_, err := e.docs.Put(t.Context(), p, drive.Shared, path, []byte("# hello"))
	require.NoError(t, err)
	share, err := e.shares.Create(t.Context(), p, drive.Shared, path, expiresIn)
	require.NoError(t, err)
	slug, ok := strings.CutPrefix(share.URL, testBaseURL+"/s/")
	require.True(t, ok, share.URL)
	return share, slug
}

func resolveErr(t *testing.T, e *env, slug string) error {
	t.Helper()
	_, _, _, err := e.shares.Resolve(t.Context(), slug) //nolint:dogsled // only the error matters
	return err
}

func requireGone(t *testing.T, e *env, slug string) {
	t.Helper()
	err := resolveErr(t, e, slug)
	requireKind(t, err, core.KindNotFound)
}

func TestCreateIsIdempotentAndResolveReturnsTheDocument(t *testing.T) {
	e := newEnv(t, devPolicy())
	p := principalFor(t, e, "dev@example.com")

	first, slug := sharedDoc(t, e, p, "a.md", "")
	assert.Len(t, slug, 22, "16 random bytes, base64url without padding")
	second, err := e.shares.Create(t.Context(), p, drive.Shared, "a.md", "")
	require.NoError(t, err)
	assert.Equal(t, first, second)

	owner, doc, body, err := e.shares.Resolve(t.Context(), slug)
	require.NoError(t, err)
	assert.Equal(t, "dev@example.com", owner.Email)
	assert.Equal(t, "a.md", doc.Path)
	assert.Equal(t, "# hello", string(body))
}

func TestCreateExpiry(t *testing.T) {
	e := newEnv(t, devPolicy())
	p := principalFor(t, e, "dev@example.com")

	share, slug := sharedDoc(t, e, p, "a.md", "1d")
	require.NotNil(t, share.ExpiresAt)
	assert.Equal(t, e.clock.Now().Add(24*time.Hour), *share.ExpiresAt)

	again, err := e.shares.Create(t.Context(), p, drive.Shared, "a.md", "")
	require.NoError(t, err)
	assert.Equal(t, share.ExpiresAt, again.ExpiresAt, "an empty expiry leaves the existing one")
	assert.Equal(t, "1d", again.ExpiresIn)

	longer, err := e.shares.Create(t.Context(), p, drive.Shared, "a.md", "30d")
	require.NoError(t, err)
	assert.Equal(t, share.URL, longer.URL)
	assert.Equal(t, e.clock.Now().Add(30*24*time.Hour), *longer.ExpiresAt)

	e.clock.Advance(25 * time.Hour)
	err = resolveErr(t, e, slug)
	require.NoError(t, err, "30d replaced 1d")
	e.clock.Advance(30 * 24 * time.Hour)
	requireGone(t, e, slug)
}

func TestCreateRejectsBadExpiryAndMissingDocument(t *testing.T) {
	e := newEnv(t, devPolicy())
	p := principalFor(t, e, "dev@example.com")
	_, err := e.docs.Put(t.Context(), p, drive.Shared, "a.md", []byte("x"))
	require.NoError(t, err)

	for _, in := range []string{"2d", "1h", "forever", "1D"} {
		_, err := e.shares.Create(t.Context(), p, drive.Shared, "a.md", in)
		requireKind(t, err, core.KindInvalid)
	}
	_, err = e.shares.Create(t.Context(), p, drive.Shared, "missing.md", "")
	requireKind(t, err, core.KindNotFound)
	_, err = e.shares.Create(t.Context(), p, drive.Shared, "../a.md", "")
	requireKind(t, err, core.KindInvalid)
}

func TestExpiredRowIsNotFound(t *testing.T) {
	e := newEnv(t, devPolicy())
	p := principalFor(t, e, "dev@example.com")
	_, slug := sharedDoc(t, e, p, "a.md", "")

	_, err := e.rawDB(t).Exec(`UPDATE share_links SET expires_at = ? WHERE slug = ?`, e.clock.Now().Add(-time.Second).UnixNano(), slug)
	require.NoError(t, err)
	requireGone(t, e, slug)
}

func TestRevoke(t *testing.T) {
	e := newEnv(t, devPolicy())
	p := principalFor(t, e, "dev@example.com")
	_, slug := sharedDoc(t, e, p, "a.md", "")

	require.NoError(t, e.shares.Revoke(t.Context(), p, drive.Shared, "a.md"))
	requireGone(t, e, slug)
	require.NoError(t, e.shares.Revoke(t.Context(), p, drive.Shared, "a.md"), "revoking an unshared document is a no-op")
	requireKind(t, e.shares.Revoke(t.Context(), p, drive.Shared, "missing.md"), core.KindNotFound)

	_, _, err := e.docs.Get(t.Context(), p, drive.Shared, "a.md")
	require.NoError(t, err, "the document stays")
}

func TestDeletingTheDocumentBreaksItsLink(t *testing.T) {
	e := newEnv(t, devPolicy())
	p := principalFor(t, e, "dev@example.com")
	_, slug := sharedDoc(t, e, p, "a.md", "")

	require.NoError(t, e.docs.Delete(t.Context(), p, drive.Shared, "a.md"))
	requireGone(t, e, slug)
	var n int
	require.NoError(t, e.rawDB(t).QueryRow(`SELECT COUNT(*) FROM share_links`).Scan(&n))
	assert.Zero(t, n, "Delete removes the row itself")
}

func TestLinkFollowsReupload(t *testing.T) {
	e := newEnv(t, devPolicy())
	p := principalFor(t, e, "dev@example.com")
	_, slug := sharedDoc(t, e, p, "a.md", "")

	_, err := e.docs.Put(t.Context(), p, drive.Shared, "a.md", []byte("v2"))
	require.NoError(t, err)
	_, _, body, err := e.shares.Resolve(t.Context(), slug)
	require.NoError(t, err)
	assert.Equal(t, "v2", string(body))
}

func removeDocumentOnly(t *testing.T, e *env, p core.Principal, path string) {
	t.Helper()
	store, release, err := e.users.Open(t.Context(), p.UserID)
	require.NoError(t, err)
	defer release()
	_, err = store.Delete(t.Context(), drive.Shared.Key, path)
	require.NoError(t, err)
}

func linkRows(t *testing.T, e *env) int {
	t.Helper()
	var n int
	require.NoError(t, e.rawDB(t).QueryRow(`SELECT COUNT(*) FROM share_links`).Scan(&n))
	return n
}

func TestResolveRemovesADanglingRow(t *testing.T) {
	e := newEnv(t, devPolicy())
	p := principalFor(t, e, "dev@example.com")
	_, slug := sharedDoc(t, e, p, "a.md", "")

	removeDocumentOnly(t, e, p, "a.md")
	require.Equal(t, 1, linkRows(t, e))
	requireGone(t, e, slug)
	assert.Zero(t, linkRows(t, e))
}

func TestSweeperDropsExpiredAndDanglingLinks(t *testing.T) {
	e := newEnv(t, devPolicy())
	p := principalFor(t, e, "dev@example.com")
	other := principalFor(t, e, "other@example.com")

	_, keep := sharedDoc(t, e, p, "keep.md", "")
	sharedDoc(t, e, p, "expires.md", "1d")
	sharedDoc(t, e, p, "dangling.md", "")
	sharedDoc(t, e, other, "other-dangling.md", "")
	removeDocumentOnly(t, e, p, "dangling.md")
	removeDocumentOnly(t, e, other, "other-dangling.md")
	e.clock.Advance(48 * time.Hour)
	require.Equal(t, 4, linkRows(t, e))

	require.NoError(t, e.sweeper.Tick(t.Context()))
	assert.Equal(t, 1, linkRows(t, e))
	err := resolveErr(t, e, keep)
	require.NoError(t, err)
}

func TestSweeperWalksMoreThanOneBatch(t *testing.T) {
	e := newEnv(t, devPolicy())
	p := principalFor(t, e, "dev@example.com")
	for i := range 250 {
		path := "doc-" + strings.Repeat("x", i%3) + string(rune('a'+i%26)) + string(rune('a'+i/26)) + ".md"
		sharedDoc(t, e, p, path, "")
		removeDocumentOnly(t, e, p, path)
	}
	require.Equal(t, 250, linkRows(t, e))
	require.NoError(t, e.sweeper.Tick(t.Context()))
	assert.Zero(t, linkRows(t, e))
}

func TestLockedOwnersLinkStops(t *testing.T) {
	e := newEnv(t, devPolicy())
	p := principalFor(t, e, "dev@example.com")
	_, slug := sharedDoc(t, e, p, "a.md", "")

	e.policies.Swap([]config.Policy{})
	requireGone(t, e, slug)

	e.policies.Swap([]config.Policy{devPolicy()})
	err := resolveErr(t, e, slug)
	require.NoError(t, err, "unlocking restores the link")
}

func TestUnknownSlug(t *testing.T) {
	e := newEnv(t, devPolicy())
	requireGone(t, e, "nope")
}

func TestListFillsShared(t *testing.T) {
	e := newEnv(t, devPolicy())
	p := principalFor(t, e, "dev@example.com")
	share, _ := sharedDoc(t, e, p, "a.md", "7d")
	_, err := e.docs.Put(t.Context(), p, drive.Shared, "b.md", []byte("x"))
	require.NoError(t, err)

	docs, err := e.docs.List(t.Context(), p, drive.Shared)
	require.NoError(t, err)
	require.Len(t, docs, 2)
	require.NotNil(t, docs[0].Shared)
	assert.Equal(t, share, *docs[0].Shared)
	assert.Nil(t, docs[1].Shared)
}

func TestCreateStoresThePresetAndSwitchesIt(t *testing.T) {
	e := newEnv(t, devPolicy())
	p := principalFor(t, e, "dev@example.com")
	now := e.clock.Now()

	first, slug := sharedDoc(t, e, p, "a.md", "7d")
	assert.Equal(t, "7d", first.ExpiresIn)

	for _, step := range []struct {
		in, wantIn string
		wantAt     time.Duration
	}{
		{"30d", "30d", 30 * 24 * time.Hour},
		{driveapi.ShareExpiryNone, "", 0},
		{"1d", "1d", 24 * time.Hour},
	} {
		got, err := e.shares.Create(t.Context(), p, drive.Shared, "a.md", step.in)
		require.NoError(t, err)
		assert.Equal(t, first.URL, got.URL)
		assert.Equal(t, step.wantIn, got.ExpiresIn)
		if step.wantAt == 0 {
			assert.Nil(t, got.ExpiresAt)
		} else {
			require.NotNil(t, got.ExpiresAt)
			assert.Equal(t, now.Add(step.wantAt), *got.ExpiresAt)
		}

		again, err := e.shares.Create(t.Context(), p, drive.Shared, "a.md", "")
		require.NoError(t, err)
		assert.Equal(t, got, again, "an empty expiry reads the stored state back")
	}

	_, err := e.shares.Create(t.Context(), p, drive.Shared, "a.md", driveapi.ShareExpiryNone)
	require.NoError(t, err)
	e.clock.Advance(365 * 24 * time.Hour)
	require.NoError(t, resolveErr(t, e, slug), "a cleared expiry never lapses")
}

func TestCreateWithNoExpiryMakesALinkWithoutOne(t *testing.T) {
	e := newEnv(t, devPolicy())
	p := principalFor(t, e, "dev@example.com")
	share, _ := sharedDoc(t, e, p, "a.md", driveapi.ShareExpiryNone)
	assert.Nil(t, share.ExpiresAt)
	assert.Empty(t, share.ExpiresIn)
}
