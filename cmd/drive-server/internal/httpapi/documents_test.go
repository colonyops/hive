package httpapi_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/drive-server/internal/app"
	"github.com/colonyops/hive/cmd/drive-server/internal/config"
	"github.com/colonyops/hive/cmd/drive-server/internal/core"
	"github.com/colonyops/hive/cmd/drive-server/internal/httpapi"
	"github.com/colonyops/hive/internal/domain/drive"
	"github.com/colonyops/hive/internal/platform/driveapi"
	"github.com/colonyops/hive/pkg/buildinfo"
)

const (
	testMaxDocumentSize = 64
	testBaseURL         = "http://drive.test"
)

type server struct {
	h       http.Handler
	svc     *app.Services
	dataDir string
}

func newServer(t *testing.T) *server { return newServerAt(t, testBaseURL) }

func newServerAt(t *testing.T, baseURL string) *server {
	t.Helper()
	dir := t.TempDir()
	svc, err := app.OpenServices(t.Context(), zerolog.Nop(), config.Config{
		DataDir:         dir,
		BaseURL:         baseURL,
		MaxDocumentSize: config.Size{Bytes: testMaxDocumentSize},
		UserPolicies:    []config.Policy{devPolicy(config.Size{Bytes: 1 << 20})},
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = svc.Close() })
	return &server{
		h:       httpapi.New(zerolog.Nop(), buildinfo.Info{}, svc.HTTP(baseURL, nil)),
		svc:     svc,
		dataDir: dir,
	}
}

func (s *server) token(t *testing.T, email string, scope core.Scope) string {
	t.Helper()
	secret, _, _, err := s.svc.CreateToken(t.Context(), email, scope, "test", 0)
	require.NoError(t, err)
	return secret
}

func (s *server) do(method, target, token, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, bytes.NewBufferString(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	s.h.ServeHTTP(rec, req)
	return rec
}

func errorKind(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body driveapi.ErrorBody
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body), rec.Body.String())
	return body.Kind
}

func mustSpace(t *testing.T) map[string]drive.Space {
	t.Helper()
	repo, err := drive.RepositorySpace("colonyops", "hive")
	require.NoError(t, err)
	ws, err := drive.WorkspaceSpace("my chats")
	require.NoError(t, err)
	return map[string]drive.Space{"repository": repo, "workspace": ws, "shared": drive.Shared}
}

func TestDocumentRoundTripPerSpaceKind(t *testing.T) {
	s := newServer(t)
	tok := s.token(t, "dev@example.com", core.ScopePush)

	for name, space := range mustSpace(t) {
		t.Run(name, func(t *testing.T) {
			url := driveapi.DocPath(space, "plans/plan.md")

			put := s.do(http.MethodPut, url, tok, "# hello")
			require.Equal(t, http.StatusOK, put.Code, put.Body.String())
			var doc driveapi.Document
			require.NoError(t, json.Unmarshal(put.Body.Bytes(), &doc))
			assert.Equal(t, space.Key, doc.Space)
			assert.Equal(t, drive.KindMarkdown, doc.Kind)

			got := s.do(http.MethodGet, url, tok, "")
			require.Equal(t, http.StatusOK, got.Code)
			assert.Equal(t, "# hello", got.Body.String())
			assert.Equal(t, "text/markdown; charset=utf-8", got.Header().Get("Content-Type"))
			assert.Equal(t, `"`+doc.SHA256+`"`, got.Header().Get("ETag"))

			htmlURL := driveapi.DocPath(space, "page.html")
			require.Equal(t, http.StatusOK, s.do(http.MethodPut, htmlURL, tok, "<p>hi</p>").Code)
			assert.Equal(t, "text/html; charset=utf-8", s.do(http.MethodGet, htmlURL, tok, "").Header().Get("Content-Type"))

			list := s.do(http.MethodGet, driveapi.SpacePrefix(space), tok, "")
			require.Equal(t, http.StatusOK, list.Code)
			var docs []driveapi.Document
			require.NoError(t, json.Unmarshal(list.Body.Bytes(), &docs))
			require.Len(t, docs, 2)
			assert.Equal(t, "page.html", docs[0].Path)
			assert.Equal(t, doc.SHA256, docs[1].SHA256)

			require.Equal(t, http.StatusNoContent, s.do(http.MethodDelete, url, tok, "").Code)
			assert.Equal(t, http.StatusNotFound, s.do(http.MethodGet, url, tok, "").Code)
			assert.Equal(t, http.StatusNotFound, s.do(http.MethodDelete, url, tok, "").Code)
		})
	}
}

func TestDocPathRoundTripsAwkwardNames(t *testing.T) {
	s := newServer(t)
	tok := s.token(t, "dev@example.com", core.ScopePush)

	for _, path := range []string{"my notes/a b.md", "q?.md", "h#ash.md", "100%.md", "plus+.md"} {
		url := driveapi.DocPath(drive.Shared, path)
		require.Equal(t, http.StatusOK, s.do(http.MethodPut, url, tok, "x").Code, path)
		got := s.do(http.MethodGet, url, tok, "")
		require.Equal(t, http.StatusOK, got.Code, path)
		assert.Equal(t, "x", got.Body.String())
	}
	list := s.do(http.MethodGet, "/api/v1/shared", tok, "")
	var docs []driveapi.Document
	require.NoError(t, json.Unmarshal(list.Body.Bytes(), &docs))
	paths := make([]string, len(docs))
	for i, d := range docs {
		paths[i] = d.Path
	}
	assert.ElementsMatch(t, []string{"my notes/a b.md", "q?.md", "h#ash.md", "100%.md", "plus+.md"}, paths)
}

func TestReadTokenCannotWrite(t *testing.T) {
	s := newServer(t)
	push := s.token(t, "dev@example.com", core.ScopePush)
	read := s.token(t, "dev@example.com", core.ScopeRead)
	url := driveapi.DocPath(drive.Shared, "a.md")
	require.Equal(t, http.StatusOK, s.do(http.MethodPut, url, push, "x").Code)

	put := s.do(http.MethodPut, url, read, "y")
	assert.Equal(t, http.StatusForbidden, put.Code)
	assert.Equal(t, "forbidden", errorKind(t, put))
	assert.Equal(t, http.StatusForbidden, s.do(http.MethodDelete, url, read, "").Code)

	assert.Equal(t, http.StatusOK, s.do(http.MethodGet, url, read, "").Code)
	assert.Equal(t, http.StatusOK, s.do(http.MethodGet, "/api/v1/shared", read, "").Code)
	assert.Equal(t, http.StatusOK, s.do(http.MethodGet, "/api/v1/spaces", read, "").Code)
	assert.Equal(t, http.StatusUnauthorized, s.do(http.MethodGet, url, "", "").Code)
}

func TestOversizeBodyIsTooLarge(t *testing.T) {
	s := newServer(t)
	tok := s.token(t, "dev@example.com", core.ScopePush)
	url := driveapi.DocPath(drive.Shared, "a.md")

	assert.Equal(t, http.StatusOK, s.do(http.MethodPut, url, tok, strings.Repeat("x", testMaxDocumentSize)).Code)
	for _, n := range []int{testMaxDocumentSize + 1, testMaxDocumentSize * 10} {
		rec := s.do(http.MethodPut, url, tok, strings.Repeat("x", n))
		assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code, n)
		assert.Equal(t, "too_large", errorKind(t, rec))
	}
}

func TestRejectedWritesAndSpaces(t *testing.T) {
	s := newServer(t)
	tok := s.token(t, "dev@example.com", core.ScopePush)

	assert.Equal(t, http.StatusUnprocessableEntity, s.do(http.MethodPut, "/api/v1/shared/docs/a.txt", tok, "x").Code)
	assert.Equal(t, http.StatusUnprocessableEntity, s.do(http.MethodPut, "/api/v1/shared/docs/a.canvas.json", tok, "x").Code)
	assert.Equal(t, http.StatusNotFound, s.do(http.MethodPut, "/api/v1/repos/workspaces/x/docs/a.md", tok, "x").Code)
	assert.Equal(t, http.StatusNotFound, s.do(http.MethodGet, "/api/v1/repos/shared/x", tok, "").Code)
}

func TestSpacesAndMeUsage(t *testing.T) {
	s := newServer(t)
	tok := s.token(t, "dev@example.com", core.ScopePush)
	spaces := mustSpace(t)
	require.Equal(t, http.StatusOK, s.do(http.MethodPut, driveapi.DocPath(spaces["repository"], "a.md"), tok, "12345").Code)
	require.Equal(t, http.StatusOK, s.do(http.MethodPut, driveapi.DocPath(drive.Shared, "b.md"), tok, "123").Code)

	rec := s.do(http.MethodGet, "/api/v1/spaces", tok, "")
	require.Equal(t, http.StatusOK, rec.Code)
	var got []driveapi.SpaceSummary
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, []driveapi.SpaceSummary{
		{Space: "colonyops/hive", Kind: drive.SpaceKindRepository, Documents: 1, Bytes: 5},
		{Space: "shared", Kind: drive.SpaceKindShared, Documents: 1, Bytes: 3},
	}, got)

	me := s.do(http.MethodGet, "/api/v1/me", tok, "")
	assert.JSONEq(t, `{"email":"dev@example.com","policy":"dev","quota":{"used":8,"limit":1048576}}`, me.Body.String())

	empty := s.do(http.MethodGet, "/api/v1/shared", s.token(t, "other@example.com", core.ScopeRead), "")
	assert.JSONEq(t, `[]`, empty.Body.String())
}
