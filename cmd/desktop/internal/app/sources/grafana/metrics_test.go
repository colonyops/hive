package grafana

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
)

func queryServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[{"metric":{"__name__":"up"},"value":[1,"1"]}]}}`))
	}))
}

func TestProduceEmitsOneKeyedMessage(t *testing.T) {
	t.Parallel()

	server := queryServer(t)
	defer server.Close()
	fx, _ := connectedFetcher(t, server.URL)

	src := &metricsSource{fetcher: fx, dsUID: "ds", expr: "up", topic: "source:flow/node", key: "node"}

	var msgs []models.Msg
	require.NoError(t, src.Produce(t.Context(), func(m models.Msg) error {
		msgs = append(msgs, m)
		return nil
	}))

	require.Len(t, msgs, 1, "a metrics poll emits exactly one message")
	msg := msgs[0]
	assert.Equal(t, "node", msg.Key, "keyed by node id, so the node maps to one durable item")
	assert.Equal(t, "source:flow/node", msg.Topic)
	assert.Equal(t, SourceKind, msg.SourceKind)

	var body struct {
		Title  string          `json:"title"`
		Result json.RawMessage `json:"result"`
	}
	require.NoError(t, json.Unmarshal(msg.Payload, &body))
	assert.Equal(t, "Grafana: up", body.Title, "an empty title falls back to the query, never a bare node id")
	assert.Contains(t, string(body.Result), "resultType")
}

func TestProduceUsesConfiguredTitle(t *testing.T) {
	t.Parallel()

	server := queryServer(t)
	defer server.Close()
	fx, _ := connectedFetcher(t, server.URL)

	src := &metricsSource{fetcher: fx, dsUID: "ds", expr: "up", title: "Prod uptime", topic: "source:flow/node", key: "node"}

	var got models.Msg
	require.NoError(t, src.Produce(t.Context(), func(m models.Msg) error {
		got = m
		return nil
	}))

	var body struct {
		Title string `json:"title"`
	}
	require.NoError(t, json.Unmarshal(got.Payload, &body))
	assert.Equal(t, "Prod uptime", body.Title)
}

func TestProduceReturnsFetchError(t *testing.T) {
	t.Parallel()

	// A stack with no stored URL is not connected, so Produce surfaces the
	// error rather than emitting an empty snapshot that would clear the feed.
	fx, _ := connectedFetcher(t, "")
	src := &metricsSource{fetcher: fx, dsUID: "ds", expr: "up", topic: "source:flow/node", key: "node"}

	emitted := false
	err := src.Produce(t.Context(), func(models.Msg) error {
		emitted = true
		return nil
	})
	require.Error(t, err)
	assert.False(t, emitted, "a failed fetch emits nothing")
}
