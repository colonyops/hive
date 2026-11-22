package httpapi_test

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/drive-server/internal/core"
	"github.com/colonyops/hive/internal/domain/drive"
	"github.com/colonyops/hive/internal/platform/driveapi"
)

type sseEvent struct {
	name string
	data string
}

// openStream reads a server-sent event stream from target until the test
// ends. A missing event within a few seconds fails the test.
func openStream(t *testing.T, srv *httptest.Server, target, token string) <-chan sseEvent {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+target, nil)
	require.NoError(t, err)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := srv.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))

	out := make(chan sseEvent, 64)
	go func() {
		defer close(out)
		sc := bufio.NewScanner(resp.Body)
		var e sseEvent
		for sc.Scan() {
			line := sc.Text()
			switch {
			case line == "":
				if e.name != "" {
					out <- e
				}
				e = sseEvent{}
			case strings.HasPrefix(line, "event: "):
				e.name = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				e.data = strings.TrimPrefix(line, "data: ")
			}
		}
	}()
	return out
}

// next returns the next event named name, skipping others.
func next(t *testing.T, events <-chan sseEvent, name string) sseEvent {
	t.Helper()
	timeout := time.After(5 * time.Second)
	for {
		select {
		case e, ok := <-events:
			require.True(t, ok, "stream ended before %s", name)
			if e.name == name {
				return e
			}
		case <-timeout:
			require.FailNow(t, "no "+name+" event")
		}
	}
}

func TestLinkStreamSignalsPresenceThreadsAndRevoke(t *testing.T) {
	s := newServer(t)
	srv := httptest.NewServer(s.h)
	t.Cleanup(srv.Close)
	owner := s.token(t, "a@example.com", core.ScopePush)
	doc, slug := sharedMarkdown(t, s, owner, "plan.md")

	ownerEvents := openStream(t, srv, "/api/v1/shared/events/plan.md", owner)
	next(t, ownerEvents, "hello")
	anon := openStream(t, srv, "/api/v1/public/"+slug+"/events", "")

	var hello driveapi.StreamHello
	require.NoError(t, json.Unmarshal([]byte(next(t, anon, "hello").data), &hello))
	require.NotEmpty(t, hello.Token)

	var snap driveapi.PresenceSnapshot
	require.NoError(t, json.Unmarshal([]byte(next(t, anon, "presence").data), &snap))
	require.Len(t, snap.Viewers, 2, "the owner and the anonymous reader")
	assert.Equal(t, "a", snap.Viewers[0].Name)
	assert.True(t, snap.Viewers[0].SignedIn)
	assert.True(t, snap.Viewers[1].Self)

	rec := s.do(http.MethodPost, "/api/v1/presence", "", jsonBody(t, driveapi.PresenceUpdate{Token: hello.Token, Name: "Ana", Color: 4, Selection: &driveapi.LineRange{StartLine: 3, EndLine: 3}}))
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	for {
		var got driveapi.PresenceSnapshot
		require.NoError(t, json.Unmarshal([]byte(next(t, ownerEvents, "presence").data), &got))
		if len(got.Viewers) == 2 && got.Viewers[1].Name == "Ana" {
			assert.Equal(t, &driveapi.LineRange{StartLine: 3, EndLine: 3}, got.Viewers[1].Selection)
			break
		}
	}

	createThread(t, s, "/api/v1/links/"+slug+"/threads", owner, driveapi.CreateThread{DocumentSHA256: doc.SHA256, Body: "x"})
	next(t, anon, "threads")

	rec = s.do(http.MethodPut, driveapi.DocPath(drive.Shared, "plan.md"), owner, commentDoc+"More.\n")
	require.Equal(t, http.StatusOK, rec.Code)
	next(t, anon, "document")

	rec = s.do(http.MethodDelete, driveapi.SpacePrefix(drive.Shared)+"/shares/plan.md", owner, "")
	require.Equal(t, http.StatusNoContent, rec.Code)
	next(t, anon, "closed")

	rec = s.do(http.MethodPut, driveapi.DocPath(drive.Shared, "plan.md"), owner, commentDoc)
	require.Equal(t, http.StatusOK, rec.Code)
	next(t, ownerEvents, "document")
}

func TestStreamOfAMissingDocumentIsNotFound(t *testing.T) {
	s := newServer(t)
	owner := s.token(t, "a@example.com", core.ScopePush)
	assert.Equal(t, http.StatusNotFound, s.do(http.MethodGet, "/api/v1/shared/events/none.md", owner, "").Code)
	assert.Equal(t, http.StatusNotFound, s.do(http.MethodGet, "/api/v1/public/nope/events", "", "").Code)
}
