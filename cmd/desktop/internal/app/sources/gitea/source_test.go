package gitea

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/credentials"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/connector"
)

// connectedFetchers wires a fetcher registry onto a running test server with
// one connected account, the way app.New wires the real one.
func connectedFetchers(t *testing.T, baseURL string) (*Fetchers, string) {
	t.Helper()
	creds := credentials.NewMemoryStore()
	instances := NewInstanceStore(filepath.Join(t.TempDir(), "gitea-instances.json"))
	ref := credentials.Ref{Provider: Provider, Account: "git.example.com-octocat"}
	require.NoError(t, creds.Set(ref, "gta_token"))
	require.NoError(t, instances.Set(ref, Binding{URL: baseURL, Login: "octocat"}))
	return NewFetchers(instances, creds, zerolog.Nop()), ref.String()
}

// produce drains one instance built from cfg, returning the messages it emitted.
func produce(t *testing.T, fetchers *Fetchers, cfg *Config) ([]models.Msg, error) {
	t.Helper()
	require.NoError(t, cfg.Validate())
	instance, err := NewFactory(fetchers).New(connector.Node{FlowID: "flow", NodeID: "src"}, cfg)
	require.NoError(t, err)

	var msgs []models.Msg
	err = instance.Pull.Produce(t.Context(), func(msg models.Msg) error {
		msgs = append(msgs, msg)
		return nil
	})
	return msgs, err
}

// parseRef turns the credential string connectedFetchers hands back into the
// ref the fetcher registry is keyed on.
func parseRef(t *testing.T, ref string) credentials.Ref {
	t.Helper()
	parsed, err := credentials.ParseRef(ref)
	require.NoError(t, err)
	return parsed
}

// issueJSON is one search result, as Gitea sends it.
func issueJSON(number int, title, state string) string {
	return fmt.Sprintf(`{"id":%d,"number":%d,"title":%q,"state":%q,
		"html_url":"https://git.example.com/acme/app/issues/%d","updated_at":"2026-08-11T10:00:00Z",
		"user":{"login":"octocat"},"labels":[],"repository":{"full_name":"acme/app"}}`,
		number, number, title, state, number)
}

func TestProduceEmitsSearchItems(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/repos/issues/search", r.URL.Path)
		_, _ = w.Write([]byte(`[` + issueJSON(1, "First", "open") + `,` + issueJSON(2, "Second", "open") + `]`))
	}))
	defer server.Close()

	fetchers, ref := connectedFetchers(t, server.URL)
	msgs, err := produce(t, fetchers, &Config{Credential: ref, Kind: KindSearch})
	require.NoError(t, err)

	require.Len(t, msgs, 2)
	assert.Equal(t, "acme/app#1", msgs[0].Key)
	assert.Equal(t, SourceKind, msgs[0].SourceKind)
	assert.Equal(t, "source:flow/src", msgs[0].Topic)

	var item Item
	require.NoError(t, json.Unmarshal(msgs[0].Payload, &item))
	assert.Equal(t, "First", item.Title)
	assert.Equal(t, OriginSearch, item.Origin)
}

// The union is several requests merged into one snapshot, so a node asking for
// two involvements makes two calls and emits each item once.
func TestProduceUnionsInvolvementsIntoOneSnapshot(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	seen := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		switch {
		case r.URL.Query().Get("review_requested") == "true":
			seen["review_requested"]++
			mu.Unlock()
			_, _ = w.Write([]byte(`[` + issueJSON(1, "Shared", "open") + `,` + issueJSON(2, "Review", "open") + `]`))
		case r.URL.Query().Get("assigned") == "true":
			seen["assigned"]++
			mu.Unlock()
			_, _ = w.Write([]byte(`[` + issueJSON(1, "Shared", "open") + `,` + issueJSON(3, "Assigned", "open") + `]`))
		default:
			mu.Unlock()
			t.Errorf("unexpected query %q", r.URL.RawQuery)
		}
	}))
	defer server.Close()

	fetchers, ref := connectedFetchers(t, server.URL)
	msgs, err := produce(t, fetchers, &Config{
		Credential: ref, Kind: KindSearch,
		Involving: []string{InvolvingReviewRequested, InvolvingAssigned},
	})
	require.NoError(t, err)

	assert.Equal(t, map[string]int{"review_requested": 1, "assigned": 1}, seen, "one request per involvement")
	assert.Len(t, msgs, 3, "the item matching both involvements is emitted once")
}

// A successful Produce is an authoritative snapshot, so a half-answered union
// must fail rather than archive everything the failed half owned.
func TestProduceFailsWhenOneSearchOfAUnionFails(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("assigned") == "true" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`[` + issueJSON(1, "Review", "open") + `]`))
	}))
	defer server.Close()

	fetchers, ref := connectedFetchers(t, server.URL)
	msgs, err := produce(t, fetchers, &Config{
		Credential: ref, Kind: KindSearch,
		Involving: []string{InvolvingReviewRequested, InvolvingAssigned},
	})

	require.Error(t, err)
	assert.Empty(t, msgs, "nothing is emitted before the whole result is in hand")
}

func TestProduceEmitsNotificationItems(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/notifications", r.URL.Path)
		_, _ = w.Write([]byte(`[
			{"id":1,"unread":true,"updated_at":"2026-08-12T08:34:18Z",
			 "repository":{"full_name":"acme/app"},
			 "subject":{"title":"A PR","type":"Pull","state":"open",
			            "url":"https://git.example.com/api/v1/repos/acme/app/issues/9",
			            "html_url":"https://git.example.com/acme/app/pulls/9"}}
		]`))
	}))
	defer server.Close()

	fetchers, ref := connectedFetchers(t, server.URL)
	msgs, err := produce(t, fetchers, &Config{Credential: ref, Kind: KindNotifications})
	require.NoError(t, err)

	require.Len(t, msgs, 1)
	assert.Equal(t, "acme/app#9", msgs[0].Key)

	var item Item
	require.NoError(t, json.Unmarshal(msgs[0].Payload, &item))
	assert.Equal(t, OriginNotifications, item.Origin)
}

// An account with no stored binding must fail rather than reach the network as
// nobody, which would surface as an empty — and therefore authoritative —
// snapshot that archived the source's whole tracked set.
func TestProduceFailsForAnUnconnectedAccount(t *testing.T) {
	t.Parallel()

	creds := credentials.NewMemoryStore()
	instances := NewInstanceStore(filepath.Join(t.TempDir(), "gitea-instances.json"))
	fetchers := NewFetchers(instances, creds, zerolog.Nop())

	_, err := produce(t, fetchers, &Config{Credential: Provider + "/git.example.com-octocat", Kind: KindSearch})
	assert.ErrorContains(t, err, "not connected")
}

// The scope is what separates two accounts' items inside one flow, and it is
// host-qualified so two instances never collide.
func TestInstanceMetadataScopesItemsToTheAccount(t *testing.T) {
	t.Parallel()

	fetchers, ref := connectedFetchers(t, "https://git.example.com")
	cfg := &Config{Credential: ref, Kind: KindSearch}
	require.NoError(t, cfg.Validate())

	instance, err := NewFactory(fetchers).New(connector.Node{FlowID: "flow", NodeID: "src"}, cfg)
	require.NoError(t, err)

	assert.Equal(t, "flow", instance.Metadata.ProfileID)
	assert.Equal(t, SourceKind, instance.Metadata.SourceKind)
	assert.Equal(t, "git.example.com-octocat", instance.Metadata.SourceScope)
}

func TestFactoryRejectsAnotherConnectorsConfig(t *testing.T) {
	t.Parallel()

	fetchers, _ := connectedFetchers(t, "https://git.example.com")
	_, err := NewFactory(fetchers).New(connector.Node{FlowID: "flow", NodeID: "src"}, &stubConfig{})
	assert.ErrorContains(t, err, "want *gitea.Config")
}

type stubConfig struct{}

func (*stubConfig) Validate() error { return nil }
