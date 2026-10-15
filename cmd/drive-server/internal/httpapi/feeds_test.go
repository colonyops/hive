package httpapi_test

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/drive-server/internal/core"
	"github.com/colonyops/hive/internal/domain/drive"
	"github.com/colonyops/hive/internal/platform/driveapi"
)

func getDocs(t *testing.T, s *server, target, token string) []driveapi.Document {
	t.Helper()
	rec := s.do(http.MethodGet, target, token, "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var docs []driveapi.Document
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &docs))
	return docs
}

func docPaths(docs []driveapi.Document) []string {
	out := make([]string, len(docs))
	for i, d := range docs {
		out[i] = d.Space + ":" + d.Path
	}
	return out
}

func TestRecentSpansSpacesNewestFirstWithShares(t *testing.T) {
	s := newServer(t)
	tok := s.token(t, "dev@example.com", core.ScopePush)
	spaces := mustSpace(t)
	for _, put := range []struct {
		space drive.Space
		path  string
	}{{spaces["shared"], "a.md"}, {spaces["workspace"], "b.md"}, {spaces["repository"], "c.md"}} {
		rec := s.do(http.MethodPut, driveapi.DocPath(put.space, put.path), tok, "x")
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	}
	rec := s.do(http.MethodPost, "/api/v1/workspaces/my%20chats/shares/b.md", tok, "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	docs := getDocs(t, s, "/api/v1/recent", tok)
	assert.Equal(t, []string{"colonyops/hive:c.md", "workspaces/my chats:b.md", "shared:a.md"}, docPaths(docs))
	assert.Nil(t, docs[0].Shared)
	require.NotNil(t, docs[1].Shared)
	assert.Contains(t, docs[1].Shared.URL, "/s/")

	assert.Equal(t, []string{"colonyops/hive:c.md"}, docPaths(getDocs(t, s, "/api/v1/recent?limit=1", tok)))
}

func TestRecentLimitIsValidatedAndCapped(t *testing.T) {
	s := newServer(t)
	tok := s.token(t, "dev@example.com", core.ScopeRead)

	for _, bad := range []string{"0", "-1", "x"} {
		assert.Equal(t, http.StatusUnprocessableEntity, s.do(http.MethodGet, "/api/v1/recent?limit="+bad, tok, "").Code, bad)
	}
	assert.Equal(t, http.StatusOK, s.do(http.MethodGet, "/api/v1/recent?limit=9999", tok, "").Code)
	assert.Equal(t, http.StatusUnauthorized, s.do(http.MethodGet, "/api/v1/recent", "", "").Code)
	assert.Equal(t, http.StatusUnauthorized, s.do(http.MethodGet, "/api/v1/shared-links", "", "").Code)
}

func TestRecentCapsAtMaximum(t *testing.T) {
	s := newServer(t)
	tok := s.token(t, "dev@example.com", core.ScopePush)
	for i := range core.MaxRecentLimit + 3 {
		rec := s.do(http.MethodPut, driveapi.DocPath(drive.Shared, "n"+strconv.Itoa(i)+".md"), tok, "x")
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	}
	assert.Len(t, getDocs(t, s, "/api/v1/recent?limit=9999", tok), core.MaxRecentLimit)
	assert.Len(t, getDocs(t, s, "/api/v1/recent", tok), core.DefaultRecentLimit)
}

func TestSharedLinksListsOnlySharedDocuments(t *testing.T) {
	s := newServer(t)
	tok := s.token(t, "dev@example.com", core.ScopePush)
	spaces := mustSpace(t)
	for _, space := range []drive.Space{spaces["shared"], spaces["repository"]} {
		for _, name := range []string{"one.md", "two.md"} {
			require.Equal(t, http.StatusOK, s.do(http.MethodPut, driveapi.DocPath(space, name), tok, "x").Code)
		}
	}
	require.Equal(t, http.StatusOK, s.do(http.MethodPost, "/api/v1/shared/shares/one.md", tok, "").Code)
	require.Equal(t, http.StatusOK, s.do(http.MethodPost, "/api/v1/repos/colonyops/hive/shares/two.md", tok, "").Code)

	docs := getDocs(t, s, "/api/v1/shared-links", tok)
	assert.Equal(t, []string{"colonyops/hive:two.md", "shared:one.md"}, docPaths(docs))
	for _, d := range docs {
		require.NotNil(t, d.Shared, d.Path)
	}

	require.Equal(t, http.StatusNoContent, s.do(http.MethodDelete, "/api/v1/shared/shares/one.md", tok, "").Code)
	assert.Equal(t, []string{"colonyops/hive:two.md"}, docPaths(getDocs(t, s, "/api/v1/shared-links", tok)))
}

func TestIsolationRecentAndSharedLinksHoldOnlyOwnDocuments(t *testing.T) {
	s := newServer(t)
	a, b := setupTenants(t, s)
	require.Equal(t, http.StatusOK, s.do(http.MethodPost, "/api/v1/shared/shares/only-a.md", a.token, "").Code)
	require.Equal(t, http.StatusOK, s.do(http.MethodPost, "/api/v1/shared/shares/shared-path.md", b.token, "").Code)

	for _, doc := range getDocs(t, s, "/api/v1/recent", b.token) {
		assert.NotEqual(t, "only-a.md", doc.Path)
	}
	assert.Len(t, getDocs(t, s, "/api/v1/recent", b.token), 3)
	assert.Len(t, getDocs(t, s, "/api/v1/recent", a.token), 6)

	assert.Equal(t, []string{"shared:only-a.md"}, docPaths(getDocs(t, s, "/api/v1/shared-links", a.token)))
	assert.Equal(t, []string{"shared:shared-path.md"}, docPaths(getDocs(t, s, "/api/v1/shared-links", b.token)))
}

func TestFeedsNeedOnlyReadScope(t *testing.T) {
	s := newServer(t)
	tok := s.token(t, "dev@example.com", core.ScopeRead)
	assert.Equal(t, http.StatusOK, s.do(http.MethodGet, "/api/v1/recent", tok, "").Code)
	assert.Equal(t, http.StatusOK, s.do(http.MethodGet, "/api/v1/shared-links", tok, "").Code)
}
