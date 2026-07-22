package github

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetIssue(t *testing.T) {
	t.Parallel()

	updatedAt := time.Date(2026, time.July, 22, 13, 45, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/acme/widgets/issues/42", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"number":42,"state":"CLOSED","merged":true,"updated_at":"2026-07-22T13:45:00Z"}`))
	}))
	defer server.Close()

	issue, err := NewClient(WithAPIBase(server.URL)).GetIssue(t.Context(), "acme", "widgets", 42)
	require.NoError(t, err)
	assert.Equal(t, 42, issue.Number)
	assert.Equal(t, "closed", issue.State)
	assert.False(t, issue.Merged, "the issues endpoint does not report PR merge state")
	assert.Equal(t, updatedAt, issue.UpdatedAt)
}

func TestGetPullRequest(t *testing.T) {
	t.Parallel()

	updatedAt := time.Date(2026, time.July, 22, 13, 46, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/acme/widgets/pulls/43", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"number":43,"state":"CLOSED","merged":true,"updated_at":"2026-07-22T13:46:00Z"}`))
	}))
	defer server.Close()

	issue, err := NewClient(WithAPIBase(server.URL)).GetPullRequest(t.Context(), "acme", "widgets", 43)
	require.NoError(t, err)
	assert.Equal(t, 43, issue.Number)
	assert.Equal(t, "closed", issue.State)
	assert.True(t, issue.Merged)
	assert.Equal(t, updatedAt, issue.UpdatedAt)
}
