package httpapi_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/drive-server/internal/core"
	"github.com/colonyops/hive/internal/domain/drive"
	"github.com/colonyops/hive/internal/platform/driveapi"
)

// The server and driveapi.Client are written apart. This runs every client
// method against the real handler so a route or wire change breaks here.
func TestClientAgainstServer(t *testing.T) {
	s := newServer(t)
	ts := httptest.NewServer(s.h)
	t.Cleanup(ts.Close)
	tok := s.token(t, "dev@example.com", core.ScopePush)
	client, err := driveapi.NewClient(ts.URL, func() string { return tok })
	require.NoError(t, err)

	for name, space := range mustSpace(t) {
		t.Run(name, func(t *testing.T) {
			ctx := t.Context()
			path := "plans/my plan #1.md"

			put, err := client.Put(ctx, space, path, []byte("# Plan\n"))
			require.NoError(t, err)
			assert.Equal(t, path, put.Path)
			assert.Equal(t, drive.KindMarkdown, put.Kind)

			doc, body, err := client.Get(ctx, space, path)
			require.NoError(t, err)
			assert.Equal(t, "# Plan\n", string(body))
			assert.Equal(t, put.SHA256, doc.SHA256, "the client's hash matches the server's")

			docs, err := client.List(ctx, space)
			require.NoError(t, err)
			require.Len(t, docs, 1)
			assert.Equal(t, path, docs[0].Path)

			share, err := client.Share(ctx, space, path, "7d")
			require.NoError(t, err)
			assert.Contains(t, share.URL, testBaseURL+"/s/")
			require.NotNil(t, share.ExpiresAt)

			docs, err = client.List(ctx, space)
			require.NoError(t, err)
			require.NotNil(t, docs[0].Shared)
			assert.Equal(t, share.URL, docs[0].Shared.URL)
		})
	}

	t.Run("me", func(t *testing.T) {
		me, err := client.Me(t.Context())
		require.NoError(t, err)
		assert.Equal(t, "dev@example.com", me.Email)
		assert.Positive(t, me.Quota.Used)
	})

	t.Run("share link serves the document", func(t *testing.T) {
		share, err := client.Share(t.Context(), drive.Shared, "plans/my plan #1.md", "")
		require.NoError(t, err)
		resp, err := http.Get(ts.URL + share.URL[len(testBaseURL):] + "/raw")
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		data, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Equal(t, "# Plan\n", string(data))
	})

	t.Run("errors carry the server's kind", func(t *testing.T) {
		_, _, err := client.Get(t.Context(), drive.Shared, "missing.md")
		var apiErr *driveapi.APIError
		require.ErrorAs(t, err, &apiErr)
		assert.Equal(t, http.StatusNotFound, apiErr.Status)
		assert.Equal(t, "not_found", apiErr.Body.Kind)

		_, err = client.Put(t.Context(), drive.Shared, "big.md", make([]byte, testMaxDocumentSize+1))
		require.ErrorAs(t, err, &apiErr)
		assert.Equal(t, "too_large", apiErr.Body.Kind)

		_, err = client.Put(t.Context(), drive.Shared, "notes.txt", []byte("x"))
		require.ErrorAs(t, err, &apiErr)
		assert.Equal(t, http.StatusUnprocessableEntity, apiErr.Status)
	})

	t.Run("a read token cannot write", func(t *testing.T) {
		readTok := s.token(t, "dev@example.com", core.ScopeRead)
		readOnly, err := driveapi.NewClient(ts.URL, func() string { return readTok })
		require.NoError(t, err)

		_, err = readOnly.Put(t.Context(), drive.Shared, "a.md", []byte("x"))
		var apiErr *driveapi.APIError
		require.ErrorAs(t, err, &apiErr)
		assert.Equal(t, "forbidden", apiErr.Body.Kind)

		_, err = readOnly.List(t.Context(), drive.Shared)
		require.NoError(t, err)
	})
}
