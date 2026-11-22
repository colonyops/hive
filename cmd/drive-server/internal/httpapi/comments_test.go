package httpapi_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/drive-server/internal/core"
	"github.com/colonyops/hive/internal/domain/drive"
	"github.com/colonyops/hive/internal/platform/driveapi"
)

const commentDoc = "# T\n\nKeep the old ones.\n"

// sharedMarkdown pushes a markdown document as token's owner and shares it.
func sharedMarkdown(t *testing.T, s *server, token, path string) (driveapi.Document, string) {
	t.Helper()
	rec := s.do(http.MethodPut, driveapi.DocPath(drive.Shared, path), token, commentDoc)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	doc := decode[driveapi.Document](t, rec)
	rec = s.do(http.MethodPost, driveapi.SpacePrefix(drive.Shared)+"/shares/"+path, token, "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	slug, ok := strings.CutPrefix(decode[driveapi.Share](t, rec).URL, testBaseURL+"/s/")
	require.True(t, ok)
	return doc, slug
}

func jsonBody(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return string(b)
}

func createThread(t *testing.T, s *server, target, token string, in driveapi.CreateThread) driveapi.Thread {
	t.Helper()
	rec := s.do(http.MethodPost, target, token, jsonBody(t, in))
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	return decode[driveapi.Thread](t, rec)
}

func TestCommentRoundTripOverLinkAndOwnerRoutes(t *testing.T) {
	s := newServer(t)
	owner := s.token(t, "a@example.com", core.ScopePush)
	reader := s.token(t, "b@example.com", core.ScopePush)
	doc, slug := sharedMarkdown(t, s, owner, "plan.md")

	th := createThread(t, s, "/api/v1/links/"+slug+"/threads", reader, driveapi.CreateThread{
		DocumentSHA256: doc.SHA256,
		Anchor:         &driveapi.Anchor{StartLine: 3, EndLine: 3, Quote: "Keep the old ones"},
		Body:           "Why?",
	})
	assert.Equal(t, "b", th.Author.Name)
	assert.Equal(t, "test", th.Author.Agent, "a token-written comment names its token")

	rec := s.do(http.MethodPost, "/api/v1/threads/"+th.ID+"/comments", owner, `{"body":"Because."}`)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	reply := decode[driveapi.Comment](t, rec)

	rec = s.do(http.MethodPatch, "/api/v1/threads/"+th.ID, owner, `{"resolved":true}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.NotNil(t, decode[driveapi.Thread](t, rec).ResolvedAt)

	rec = s.do(http.MethodPatch, "/api/v1/comments/"+reply.ID, owner, `{"body":"Because, really."}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	rec = s.do(http.MethodGet, "/api/v1/shared/threads/plan.md", owner, "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	list := decode[driveapi.ThreadList](t, rec)
	require.Len(t, list.Threads, 1)
	assert.Equal(t, "Because, really.", list.Threads[0].Comments[1].Body)
	assert.True(t, list.Viewer.IsOwner)

	rec = s.do(http.MethodDelete, "/api/v1/links/"+slug+"/comments/"+reply.ID, reader, "")
	assert.Equal(t, http.StatusForbidden, rec.Code, "a reader cannot delete the owner's comment")
	rec = s.do(http.MethodDelete, "/api/v1/comments/"+reply.ID, owner, "")
	assert.Equal(t, http.StatusNoContent, rec.Code)
}

func TestPublicThreadsServeAnonymousReaders(t *testing.T) {
	s := newServer(t)
	owner := s.token(t, "a@example.com", core.ScopePush)
	doc, slug := sharedMarkdown(t, s, owner, "plan.md")
	createThread(t, s, "/api/v1/shared/threads/plan.md", owner, driveapi.CreateThread{DocumentSHA256: doc.SHA256, Body: "note"})

	rec := s.do(http.MethodGet, "/api/v1/public/"+slug+"/threads", "", "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	list := decode[driveapi.ThreadList](t, rec)
	require.Len(t, list.Threads, 1)
	assert.Equal(t, driveapi.Author{Name: "a", Agent: "test"}, list.Threads[0].Author)
	assert.False(t, list.Viewer.CanComment)
	assert.NotContains(t, rec.Body.String(), "a@example.com")

	rec = s.do(http.MethodGet, "/api/v1/public/"+slug+"/threads", "hds_bogus", "")
	assert.Equal(t, http.StatusOK, rec.Code, "a bad credential reads as anonymous")

	rec = s.do(http.MethodPost, "/api/v1/links/"+slug+"/threads", "", jsonBody(t, driveapi.CreateThread{DocumentSHA256: doc.SHA256, Body: "x"}))
	assert.Equal(t, http.StatusUnauthorized, rec.Code, "writing needs a signed-in principal")
}

func TestCreateThreadOnAStaleVersionIsAConflict(t *testing.T) {
	s := newServer(t)
	owner := s.token(t, "a@example.com", core.ScopePush)
	_, slug := sharedMarkdown(t, s, owner, "plan.md")

	rec := s.do(http.MethodPost, "/api/v1/links/"+slug+"/threads", owner, jsonBody(t, driveapi.CreateThread{DocumentSHA256: "old", Body: "x"}))
	assert.Equal(t, http.StatusConflict, rec.Code)
	assert.Equal(t, "conflict", errorKind(t, rec))
}

func TestMeReportsCommentsAndSetsDisplayName(t *testing.T) {
	s := newAuthServer(t, testBaseURL)
	cookie := s.signIn(t, "dev@example.com")

	rec := s.send(call{method: http.MethodGet, target: "/api/v1/me", cookie: cookie})
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, decode[driveapi.Me](t, rec).Features, driveapi.FeatureComments)

	rec = s.send(call{method: http.MethodPatch, target: "/api/v1/me", cookie: cookie, body: `{"display_name":" Dev Person "}`})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, "Dev Person", decode[driveapi.Me](t, rec).DisplayName)

	tok := s.token(t, "dev@example.com", core.ScopePush)
	rec = s.do(http.MethodPatch, "/api/v1/me", tok, `{"display_name":"x"}`)
	assert.Equal(t, http.StatusForbidden, rec.Code, "a token cannot rename the account")
}

func TestPresenceWithUnknownTokenIsNotFound(t *testing.T) {
	s := newServer(t)
	rec := s.do(http.MethodPost, "/api/v1/presence", "", `{"token":"made-up","color":0}`)
	assert.Equal(t, http.StatusNotFound, rec.Code)
	rec = s.do(http.MethodPost, "/api/v1/presence", "", `{"color":0}`)
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
}

func TestCommentWritesAreRateLimited(t *testing.T) {
	s := newServer(t)
	owner := s.token(t, "a@example.com", core.ScopePush)
	doc, _ := sharedMarkdown(t, s, owner, "plan.md")
	th := createThread(t, s, "/api/v1/shared/threads/plan.md", owner, driveapi.CreateThread{DocumentSHA256: doc.SHA256, Body: "x"})

	var last *http.Response
	for range core.CommentWritesPerUser {
		rec := s.do(http.MethodPatch, "/api/v1/threads/"+th.ID, owner, `{"resolved":true}`)
		last = rec.Result()
	}
	require.Equal(t, http.StatusTooManyRequests, last.StatusCode)
	assert.NotEmpty(t, last.Header.Get("Retry-After"))
}
