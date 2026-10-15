package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/drive-server/internal/core"
	"github.com/colonyops/hive/internal/platform/driveapi"
)

type tenant struct {
	id    string
	token string
}

func setupTenants(t *testing.T, s *server) (a, b tenant) {
	t.Helper()
	mk := func(email string) tenant {
		acct, err := s.svc.Accounts.Ensure(t.Context(), email)
		require.NoError(t, err)
		return tenant{id: acct.ID, token: s.token(t, email, core.ScopePush)}
	}
	a, b = mk("a@example.com"), mk("b@example.com")

	for _, space := range mustSpace(t) {
		for _, who := range []struct {
			tn   tenant
			name string
		}{{a, "A"}, {b, "B"}} {
			rec := s.do(http.MethodPut, driveapi.DocPath(space, "shared-path.md"), who.tn.token, "body of "+who.name)
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		}
		rec := s.do(http.MethodPut, driveapi.DocPath(space, "only-a.md"), a.token, "only A has this")
		require.Equal(t, http.StatusOK, rec.Code)
	}
	return a, b
}

func TestIsolationSamePathReturnsOwnBody(t *testing.T) {
	s := newServer(t)
	a, b := setupTenants(t, s)

	for name, space := range mustSpace(t) {
		url := driveapi.DocPath(space, "shared-path.md")
		assert.Equal(t, "body of B", s.do(http.MethodGet, url, b.token, "").Body.String(), name)
		assert.Equal(t, "body of A", s.do(http.MethodGet, url, a.token, "").Body.String(), name)
		assert.Equal(t, http.StatusNotFound, s.do(http.MethodGet, driveapi.DocPath(space, "only-a.md"), b.token, "").Code, name)
	}
}

func TestIsolationListingsHoldOnlyOwnDocuments(t *testing.T) {
	s := newServer(t)
	_, b := setupTenants(t, s)

	var spaces []driveapi.SpaceSummary
	rec := s.do(http.MethodGet, "/api/v1/spaces", b.token, "")
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &spaces))
	require.Len(t, spaces, 3)
	for _, sp := range spaces {
		assert.Equal(t, 1, sp.Documents, sp.Space)
		assert.EqualValues(t, len("body of B"), sp.Bytes, sp.Space)
	}

	for name, space := range mustSpace(t) {
		var docs []driveapi.Document
		rec := s.do(http.MethodGet, driveapi.SpacePrefix(space), b.token, "")
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &docs))
		require.Len(t, docs, 1, name)
		assert.Equal(t, "shared-path.md", docs[0].Path)
	}
}

func TestIsolationDeleteOfForeignPathIs404AndKeepsOwnersCopy(t *testing.T) {
	s := newServer(t)
	a, b := setupTenants(t, s)

	for name, space := range mustSpace(t) {
		url := driveapi.DocPath(space, "only-a.md")
		assert.Equal(t, http.StatusNotFound, s.do(http.MethodDelete, url, b.token, "").Code, name)
		got := s.do(http.MethodGet, url, a.token, "")
		assert.Equal(t, http.StatusOK, got.Code, name)
		assert.Equal(t, "only A has this", got.Body.String())
	}
}

func TestIsolationQuotaUsageExcludesOtherUsers(t *testing.T) {
	s := newServer(t)
	_, b := setupTenants(t, s)

	rec := s.do(http.MethodGet, "/api/v1/me", b.token, "")
	var me driveapi.Me
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &me))
	assert.EqualValues(t, 3*len("body of B"), me.Quota.Used)
}

func TestIsolationHostilePathsAndNoUserIDRoutes(t *testing.T) {
	s := newServer(t)
	a, b := setupTenants(t, s)

	for _, path := range []string{"..", "%2e%2e", "%2F..", "/abs", ".hidden", "a/../only-a.md", "%2e%2e/only-a.md"} {
		for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
			rec := s.do(method, "/api/v1/shared/docs/"+path, b.token, "x")
			// The mux redirects a path it can clean; following it lands on a
			// route that names no document.
			assert.Contains(t, []int{http.StatusMovedPermanently, http.StatusTemporaryRedirect, http.StatusNotFound, http.StatusUnprocessableEntity}, rec.Code, "%s %s", method, path)
			assert.NotContains(t, rec.Body.String(), "only A", "%s %s", method, path)
		}
	}
	assert.Equal(t, "only A has this", s.do(http.MethodGet, "/api/v1/shared/docs/only-a.md", a.token, "").Body.String())

	for _, target := range []string{
		"/api/v1/users/" + a.id + "/spaces",
		"/api/v1/users/" + a.id + "/shared",
		"/api/v1/" + a.id + "/shared",
		"/api/v1/shared/" + a.id + "/docs/only-a.md",
	} {
		assert.Equal(t, http.StatusNotFound, s.do(http.MethodGet, target, b.token, "").Code, target)
	}

	// A user id in a query or header must change nothing.
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/spaces?user_id="+a.id, nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+b.token)
	req.Header.Set("X-User-Id", a.id)
	rec := httptest.NewRecorder()
	s.h.ServeHTTP(rec, req)
	var spaces []driveapi.SpaceSummary
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &spaces))
	for _, sp := range spaces {
		assert.EqualValues(t, len("body of B"), sp.Bytes)
	}

	// Rejected requests must not have created files for anyone else.
	entries, err := os.ReadDir(filepath.Join(s.dataDir, "users"))
	require.NoError(t, err)
	dbs := map[string]bool{}
	for _, e := range entries {
		name := e.Name()
		base := strings.TrimSuffix(strings.TrimSuffix(name, "-wal"), "-shm")
		require.True(t, strings.HasSuffix(base, ".db"), name)
		dbs[base] = true
	}
	assert.Equal(t, map[string]bool{a.id + ".db": true, b.id + ".db": true}, dbs)
}
