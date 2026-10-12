package relay

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/sourcehttp"
)

func newSource(t *testing.T, relay *fakeRelay) *source {
	t.Helper()
	fetchers, ref := connected(t, relay)
	cfg := validConfig()
	cfg.Source = "src"
	return &source{id: "f/n", topic: "source:f/n", cfg: cfg, ref: ref, fetchers: fetchers}
}

func produce(t *testing.T, s *source) ([]models.Msg, error) {
	t.Helper()
	var out []models.Msg
	err := s.Produce(t.Context(), func(m models.Msg) error {
		out = append(out, m)
		return nil
	})
	return out, err
}

func payloadOf(t *testing.T, m models.Msg) map[string]any {
	t.Helper()
	var p map[string]any
	require.NoError(t, json.Unmarshal(m.Payload, &p))
	return p
}

func TestProduceAdvancesTheCursorAcrossPages(t *testing.T) {
	t.Parallel()
	relay := newFakeRelay(t)
	now := time.Now()
	for i := 1; i <= 250; i++ {
		relay.add(jsonDelivery(i, fmt.Sprintf(`{"id":"k%d"}`, i), now))
	}
	s := newSource(t, relay)

	msgs, err := produce(t, s)
	require.NoError(t, err)
	require.Len(t, msgs, 250)
	assert.Equal(t, "k1", msgs[0].Key)
	assert.Equal(t, "source:f/n", msgs[0].Topic)
	assert.Equal(t, SourceKind, msgs[0].SourceKind)
	assert.Equal(t, []string{"", deliveryID(200)}, relay.afters)

	_, err = produce(t, s)
	require.NoError(t, err)
	assert.Equal(t, deliveryID(250), relay.afters[2])
}

func TestProduceReemitsTheSetOnAnEmptyPage(t *testing.T) {
	t.Parallel()
	relay := newFakeRelay(t)
	relay.add(jsonDelivery(1, `{"id":"a","title":"A"}`, time.Now()))
	s := newSource(t, relay)

	first, err := produce(t, s)
	require.NoError(t, err)
	second, err := produce(t, s)
	require.NoError(t, err)
	assert.Equal(t, first, second)

	p := payloadOf(t, second[0])
	assert.Equal(t, "a", p["id"])
	assert.Equal(t, "Webhook", p["kind"])
	assert.Equal(t, "A", p["title"])
}

func TestProduceCollapsesAKeyToItsNewestDelivery(t *testing.T) {
	t.Parallel()
	relay := newFakeRelay(t)
	now := time.Now()
	relay.add(
		jsonDelivery(1, `{"id":"a","title":"old"}`, now.Add(-time.Minute)),
		jsonDelivery(2, `{"id":"b"}`, now.Add(-time.Minute)),
		jsonDelivery(3, `{"id":"a","title":"new"}`, now),
	)
	msgs, err := produce(t, newSource(t, relay))
	require.NoError(t, err)
	require.Len(t, msgs, 2)
	assert.Equal(t, "a", msgs[0].Key, "first-seen order is kept")
	assert.Equal(t, "new", payloadOf(t, msgs[0])["title"])
}

func TestProducePrunesPastRetention(t *testing.T) {
	t.Parallel()
	relay := newFakeRelay(t)
	relay.limits = Limits{RetentionSeconds: 3600, DeliveriesPerSource: 1000}
	now := time.Now()
	relay.add(
		jsonDelivery(1, `{"id":"stale"}`, now.Add(-2*time.Hour)),
		jsonDelivery(2, `{"id":"fresh"}`, now),
	)
	msgs, err := produce(t, newSource(t, relay))
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Equal(t, "fresh", msgs[0].Key)
}

func TestProducePrunesPastTheCountCap(t *testing.T) {
	t.Parallel()
	relay := newFakeRelay(t)
	relay.limits = Limits{RetentionSeconds: 3600, DeliveriesPerSource: 2}
	now := time.Now()
	relay.add(
		jsonDelivery(1, `{"id":"a"}`, now.Add(-3*time.Minute)),
		jsonDelivery(2, `{"id":"b"}`, now.Add(-2*time.Minute)),
		jsonDelivery(3, `{"id":"c"}`, now.Add(-time.Minute)),
	)
	msgs, err := produce(t, newSource(t, relay))
	require.NoError(t, err)
	require.Len(t, msgs, 2)
	assert.Equal(t, []string{"b", "c"}, []string{msgs[0].Key, msgs[1].Key})
}

func TestProduceUnauthorizedEmitsNothing(t *testing.T) {
	t.Parallel()
	relay := newFakeRelay(t)
	relay.add(jsonDelivery(1, `{"id":"a"}`, time.Now()))
	relay.status = 401
	msgs, err := produce(t, newSource(t, relay))
	require.ErrorIs(t, err, sourcehttp.ErrUnauthorized)
	assert.Empty(t, msgs)
}

func TestProduceNetworkErrorKeepsTheCursor(t *testing.T) {
	t.Parallel()
	relay := newFakeRelay(t)
	relay.add(jsonDelivery(1, `{"id":"a"}`, time.Now()))
	s := newSource(t, relay)
	_, err := produce(t, s)
	require.NoError(t, err)

	relay.Close()
	msgs, err := produce(t, s)
	require.ErrorIs(t, err, sourcehttp.ErrUnreachable)
	assert.Empty(t, msgs)
	st := s.fetchers.state(s.id)
	assert.Equal(t, deliveryID(1), st.cursor)
	assert.Len(t, st.items, 1)
}

func TestProduceSendsTheUserAgent(t *testing.T) {
	t.Parallel()
	relay := newFakeRelay(t)
	_, err := produce(t, newSource(t, relay))
	require.NoError(t, err)
	require.NotEmpty(t, relay.userAgents)
	assert.Equal(t, "hive-desktop/1.2.3", relay.userAgents[0])
}

func TestProduceDecodesBase64AndSkipsSealedRows(t *testing.T) {
	t.Parallel()
	relay := newFakeRelay(t)
	relay.add(
		DeliveryView{ID: deliveryID(1), ReceivedAt: time.Now(), BodyBase64: base64.StdEncoding.EncodeToString([]byte(`{"id":"bin"}`))},
		DeliveryView{ID: deliveryID(2), ReceivedAt: time.Now(), Enc: "age", Blob: "c2VhbGVk"},
	)
	msgs, err := produce(t, newSource(t, relay))
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Equal(t, "bin", msgs[0].Key)
}

func TestProduceAppliesTheMap(t *testing.T) {
	t.Parallel()
	relay := newFakeRelay(t)
	relay.add(jsonDelivery(1, `{"monitor":{"id":7,"name":"api"},"heartbeat":{"status":0}}`, time.Now()))
	s := newSource(t, relay)
	s.cfg.Map = &Map{Key: "/raw/body/monitor/id", Title: "/raw/body/monitor/name", State: "/raw/body/heartbeat/status", States: map[string]string{"0": "down"}}

	msgs, err := produce(t, s)
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Equal(t, "7", msgs[0].Key)
	p := payloadOf(t, msgs[0])
	assert.Equal(t, "7", p["id"])
	assert.Equal(t, "api", p["title"])
	assert.Equal(t, "down", p["state"])
}

func TestRecentDeliveriesKeepsTheTail(t *testing.T) {
	t.Parallel()
	relay := newFakeRelay(t)
	for i := 1; i <= 600; i++ {
		relay.add(jsonDelivery(i, fmt.Sprintf(`{"id":"k%d"}`, i), time.Now()))
	}
	relay.add(DeliveryView{ID: deliveryID(601), Enc: "age"})
	fetchers, ref := connected(t, relay)
	client, err := fetchers.For(ref)
	require.NoError(t, err)

	recent, err := client.RecentDeliveries(t.Context(), "src", 3)
	require.NoError(t, err)
	require.Len(t, recent, 3)
	assert.Equal(t, "k599", recent[0].Key)
	assert.Equal(t, "generic", recent[0].Dialect)
	assert.Equal(t, "k599", recent[0].Item["id"])
	assert.Contains(t, recent[2].Error, "encrypted")
}
