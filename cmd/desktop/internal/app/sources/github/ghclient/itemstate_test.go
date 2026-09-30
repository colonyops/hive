package ghclient

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/sourcehttp"
)

func TestBuildItemStateQuery(t *testing.T) {
	t.Parallel()

	metacharacters := `o/r" { injected }
second-line`
	tests := []struct {
		name string
		refs []ItemRef
	}{
		{name: "one ref", refs: []ItemRef{{Owner: "colonyops", Name: "hive", Number: 42}}},
		{name: "two refs", refs: []ItemRef{{Owner: "colonyops", Name: "hive", Number: 42}, {Owner: "hay-kot", Name: "hive-desktop", Number: 7}}},
		{name: "metacharacters stay in variables", refs: []ItemRef{{Owner: metacharacters, Name: metacharacters, Number: 1}, {Owner: "o", Name: "r", Number: 2}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, variables := buildItemStateQuery(tt.refs)

			for i, ref := range tt.refs {
				index := strconv.Itoa(i)
				assert.Contains(t, doc, "$o"+index+": String!")
				assert.Contains(t, doc, "$n"+index+": String!")
				assert.Contains(t, doc, "$i"+index+": Int!")
				assert.Contains(t, doc, "r"+index+": repository(owner: $o"+index+", name: $n"+index+")")
				assert.Contains(t, doc, "issueOrPullRequest(number: $i"+index+")")
				assert.Equal(t, ref.Owner, variables["o"+index])
				assert.Equal(t, ref.Name, variables["n"+index])
				assert.Equal(t, ref.Number, variables["i"+index])
			}
			assert.Contains(t, doc, "... on Issue {")
			assert.Contains(t, doc, "... on PullRequest {")
			assert.Contains(t, doc, "nameWithOwner")
			assert.NotContains(t, doc, metacharacters)
			assert.NotContains(t, doc, "injected")
			assert.Equal(t, len(tt.refs), strings.Count(doc, "repository(owner:"))
		})
	}
}

// itemStateHandler builds an httptest handler that answers a batched
// ItemStates request by echoing each requested number back with the given
// state, and records the alias count of every request it serves.
func itemStateHandler(t *testing.T, state string, chunkSizes *[]int, mu *sync.Mutex) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/graphql", r.URL.Path)

		var request struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&request))

		aliasCount := len(request.Variables) / 3
		mu.Lock()
		*chunkSizes = append(*chunkSizes, aliasCount)
		mu.Unlock()

		data := make(map[string]any, aliasCount)
		for i := range aliasCount {
			num, ok := request.Variables[fmt.Sprintf("i%d", i)].(float64)
			assert.True(t, ok)
			data[fmt.Sprintf("r%d", i)] = map[string]any{
				"issueOrPullRequest": map[string]any{
					"__typename": "Issue",
					"number":     int(num),
					"state":      state,
					"updatedAt":  "2026-07-18T09:00:00Z",
					"repository": map[string]any{"nameWithOwner": "o/x"},
				},
			}
		}
		w.Header().Set("Content-Type", "application/json")
		body, err := json.Marshal(map[string]any{"data": data})
		assert.NoError(t, err)
		_, _ = w.Write(body)
	}
}

func TestItemStates_ChunksAtHundred(t *testing.T) {
	t.Parallel()

	refs := make([]ItemRef, 250)
	for i := range refs {
		refs[i] = ItemRef{Owner: "o", Name: fmt.Sprintf("r%d", i), Number: i + 1}
	}

	var mu sync.Mutex
	var chunkSizes []int
	server := httptest.NewServer(itemStateHandler(t, "OPEN", &chunkSizes, &mu))
	defer server.Close()

	out, err := NewClient(WithAPIBase(server.URL)).ItemStates(t.Context(), refs)
	require.NoError(t, err)
	require.Len(t, out, 250)
	assert.Equal(t, []int{100, 100, 50}, chunkSizes)

	for i, ref := range refs {
		require.True(t, out[i].Found, "ref %d", i)
		assert.Equal(t, ref.Number, out[i].Number)
		assert.Equal(t, "open", out[i].State)
	}
}

func TestItemStates_NullAliasIsNotFound(t *testing.T) {
	t.Parallel()

	refs := []ItemRef{
		{Owner: "o", Name: "gone", Number: 1},
		{Owner: "o", Name: "r", Number: 999},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"r0":null,"r1":{"issueOrPullRequest":null}}}`))
	}))
	defer server.Close()

	out, err := NewClient(WithAPIBase(server.URL)).ItemStates(t.Context(), refs)
	require.NoError(t, err)
	require.Len(t, out, 2)
	assert.Equal(t, ItemState{}, out[0])
	assert.Equal(t, ItemState{}, out[1])
	assert.False(t, out[0].Found)
	assert.False(t, out[1].Found)
}

// GitHub answers a deleted or private repository with a null alias AND a
// top-level NOT_FOUND error carrying partial data. The found alias in the same
// response must still resolve rather than be discarded with the error.
func TestItemStates_NotFoundErrorWithPartialData(t *testing.T) {
	t.Parallel()

	refs := []ItemRef{
		{Owner: "o", Name: "live", Number: 42},
		{Owner: "o", Name: "gone", Number: 7},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"data":{"r0":{"issueOrPullRequest":{"__typename":"Issue","number":42,"state":"CLOSED","updatedAt":"2026-07-18T09:00:00Z","repository":{"nameWithOwner":"o/live"}}},"r1":null},
			"errors":[{"type":"NOT_FOUND","path":["r1"],"message":"Could not resolve to a Repository with the name 'o/gone'."}]
		}`))
	}))
	defer server.Close()

	out, err := NewClient(WithAPIBase(server.URL)).ItemStates(t.Context(), refs)
	require.NoError(t, err, "a NOT_FOUND alias must not fail the batch")
	require.Len(t, out, 2)
	assert.True(t, out[0].Found, "the resolved alias survives the NOT_FOUND on its neighbour")
	assert.Equal(t, "closed", out[0].State)
	assert.Equal(t, 42, out[0].Number)
	assert.False(t, out[1].Found, "the gone repo resolves to not-found")
}

// A non-NOT_FOUND GraphQL error alongside partial data is still fatal: the
// tolerance is scoped to NOT_FOUND, so a real query error is never swallowed.
func TestItemStates_FatalErrorAlongsideNotFoundStillFails(t *testing.T) {
	t.Parallel()

	refs := []ItemRef{{Owner: "o", Name: "r", Number: 1}}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"r0":null},"errors":[
			{"type":"NOT_FOUND","path":["r0"],"message":"gone"},
			{"type":"FORBIDDEN","message":"resource not accessible"}
		]}`))
	}))
	defer server.Close()

	_, err := NewClient(WithAPIBase(server.URL)).ItemStates(t.Context(), refs)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resource not accessible")
}

func TestItemStates_PartialChunkFailure(t *testing.T) {
	t.Parallel()

	refs := make([]ItemRef, 250)
	for i := range refs {
		refs[i] = ItemRef{Owner: "o", Name: fmt.Sprintf("r%d", i), Number: i + 1}
	}

	var mu sync.Mutex
	var chunkSizes []int
	handler := itemStateHandler(t, "OPEN", &chunkSizes, &mu)

	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		handler(w, r)
	}))
	defer server.Close()

	out, err := NewClient(WithAPIBase(server.URL)).ItemStates(t.Context(), refs)
	require.Error(t, err)
	require.Len(t, out, 250)
	assert.Equal(t, 2, calls, "the third chunk must not be requested after a failure")

	for i := range 100 {
		assert.True(t, out[i].Found, "ref %d should have resolved", i)
	}
	for i := 100; i < 250; i++ {
		assert.Equal(t, ItemState{}, out[i], "ref %d should be the zero value", i)
	}
}

func TestItemStates_RateLimitedChunk(t *testing.T) {
	t.Parallel()

	refs := make([]ItemRef, 150)
	for i := range refs {
		refs[i] = ItemRef{Owner: "o", Name: fmt.Sprintf("r%d", i), Number: i + 1}
	}

	var mu sync.Mutex
	var chunkSizes []int
	handler := itemStateHandler(t, "CLOSED", &chunkSizes, &mu)

	const resetEpoch = 1_780_000_000
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 2 {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(resetEpoch, 10))
			_, _ = w.Write([]byte(`{"errors":[{"type":"RATE_LIMITED","message":"rate limit exceeded"}]}`))
			return
		}
		handler(w, r)
	}))
	defer server.Close()

	out, err := NewClient(WithAPIBase(server.URL)).ItemStates(t.Context(), refs)
	require.ErrorIs(t, err, sourcehttp.ErrRateLimited)
	var rateErr *sourcehttp.RateLimitError
	require.ErrorAs(t, err, &rateErr)
	assert.Equal(t, time.Unix(resetEpoch, 0), rateErr.ResetAt)

	require.Len(t, out, 150)
	for i := range 100 {
		assert.True(t, out[i].Found, "ref %d should have resolved", i)
		assert.Equal(t, "closed", out[i].State)
	}
	for i := 100; i < 150; i++ {
		assert.Equal(t, ItemState{}, out[i], "ref %d should be the zero value", i)
	}
}

func TestItemStates_Empty(t *testing.T) {
	t.Parallel()

	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		calls++
	}))
	defer server.Close()

	out, err := NewClient(WithAPIBase(server.URL)).ItemStates(t.Context(), nil)
	require.NoError(t, err)
	assert.NotNil(t, out)
	assert.Empty(t, out)
	assert.Zero(t, calls)
}
