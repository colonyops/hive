package relay

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"testing"
	"time"

	"filippo.io/age"
	"filippo.io/age/armor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seal(t *testing.T, to age.Recipient, plaintext []byte, armored bool) []byte {
	t.Helper()
	var buf bytes.Buffer
	var out io.Writer = &buf
	var armorWriter io.WriteCloser
	if armored {
		armorWriter = armor.NewWriter(&buf)
		out = armorWriter
	}
	enc, err := age.Encrypt(out, to)
	require.NoError(t, err)
	_, err = enc.Write(plaintext)
	require.NoError(t, err)
	require.NoError(t, enc.Close())
	if armorWriter != nil {
		require.NoError(t, armorWriter.Close())
	}
	return buf.Bytes()
}

func newIdentity(t *testing.T) *age.X25519Identity {
	t.Helper()
	id, err := age.GenerateX25519Identity()
	require.NoError(t, err)
	return id
}

// sourceWithIdentity is a source whose account holds an age identity, as a
// connect leaves it.
func sourceWithIdentity(t *testing.T, relay *fakeRelay) (*source, *age.X25519Identity) {
	t.Helper()
	s := newSource(t, relay)
	id := newIdentity(t)
	require.NoError(t, s.fetchers.creds.Set(ageRef(s.ref.Account), id.String()))
	return s, id
}

func sealedRecord(t *testing.T, n int, to age.Recipient, body string) DeliveryView {
	t.Helper()
	plain, err := json.Marshal(record{Method: "POST", Path: "/hook", Body: []byte(body), ContentType: "application/json"})
	require.NoError(t, err)
	blob := seal(t, to, plain, false)
	return DeliveryView{ID: deliveryID(n), ReceivedAt: time.Now(), Enc: "age", Blob: base64.StdEncoding.EncodeToString(blob), ContentLength: int64(len(blob))}
}

func TestProduceOpensASealedRecord(t *testing.T) {
	t.Parallel()
	relay := newFakeRelay(t)
	s, id := sourceWithIdentity(t, relay)
	relay.add(sealedRecord(t, 1, id.Recipient(), `{"id":"sealed","title":"Sealed"}`))

	msgs, err := produce(t, s)
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Equal(t, "sealed", msgs[0].Key)
	p := payloadOf(t, msgs[0])
	assert.Equal(t, "Sealed", p["title"])
	assert.Equal(t, "Webhook", p["kind"])
}

func TestProduceOpensASenderSealedBody(t *testing.T) {
	t.Parallel()
	relay := newFakeRelay(t)
	s, id := sourceWithIdentity(t, relay)
	binary := seal(t, id.Recipient(), []byte(`{"id":"binary"}`), false)
	armored := seal(t, id.Recipient(), []byte(`{"id":"armored"}`), true)
	relay.add(
		DeliveryView{ID: deliveryID(1), ReceivedAt: time.Now(), Method: "POST", BodyBase64: base64.StdEncoding.EncodeToString(binary)},
		DeliveryView{ID: deliveryID(2), ReceivedAt: time.Now(), Method: "POST", Body: string(armored)},
	)

	msgs, err := produce(t, s)
	require.NoError(t, err)
	require.Len(t, msgs, 2)
	assert.Equal(t, "binary", msgs[0].Key)
	assert.Equal(t, "armored", msgs[1].Key)
}

func TestProduceShowsAnUnopenableDeliveryAsAPlaceholder(t *testing.T) {
	t.Parallel()
	relay := newFakeRelay(t)
	s, _ := sourceWithIdentity(t, relay)
	other := newIdentity(t)
	relay.add(
		sealedRecord(t, 1, other.Recipient(), `{"id":"hidden"}`),
		DeliveryView{ID: deliveryID(2), ReceivedAt: time.Now(), Body: string(seal(t, other.Recipient(), []byte(`{"id":"x"}`), true))},
	)

	msgs, err := produce(t, s)
	require.NoError(t, err)
	require.Len(t, msgs, 2)
	for i, msg := range msgs {
		assert.Equal(t, deliveryID(i+1), msg.Key)
		assert.Equal(t, map[string]any{
			"id":    deliveryID(i + 1),
			"title": "Encrypted delivery, no key on this device",
			"kind":  "Encrypted",
		}, payloadOf(t, msg))
	}
}

func TestRecentDeliveriesOpensSealedRows(t *testing.T) {
	t.Parallel()
	relay := newFakeRelay(t)
	s, id := sourceWithIdentity(t, relay)
	relay.add(
		sealedRecord(t, 1, id.Recipient(), `{"id":"sealed"}`),
		sealedRecord(t, 2, newIdentity(t).Recipient(), `{"id":"other"}`),
	)

	recent, err := s.fetchers.RecentDeliveries(t.Context(), s.ref, "src", 5)
	require.NoError(t, err)
	require.Len(t, recent, 2)
	assert.Equal(t, "sealed", recent[0].Key)
	assert.Equal(t, "/hook", recent[0].Path)
	assert.Empty(t, recent[0].Error)
	assert.Contains(t, recent[1].Error, "encrypted")
}
