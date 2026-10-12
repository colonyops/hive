package httpapi

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"filippo.io/age"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/relay/internal/relay"
	"github.com/colonyops/hive/cmd/relay/internal/wire"
	"github.com/colonyops/hive/internal/web"
)

func TestAPIRecipientsAndEncryption(t *testing.T) {
	h := newHarness(t, relay.DefaultLimits())
	c := client{h: h, key: h.mint(owner)}
	identity, err := age.GenerateX25519Identity()
	require.NoError(t, err)

	rec := c.call(http.MethodPatch, "/v1/me", map[string]string{"encryption": "on"})
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code, rec.Body.String())
	assert.Equal(t, "invalid", decode[web.ErrorBody](t, rec).Kind)
	assert.Equal(t, http.StatusUnprocessableEntity, c.call(http.MethodPatch, "/v1/me", map[string]string{"encryption": "yes"}).Code)

	assert.Equal(t, http.StatusUnprocessableEntity, c.call(http.MethodPost, "/v1/recipients", map[string]string{"recipient": ""}).Code)
	assert.Equal(t, http.StatusUnprocessableEntity, c.call(http.MethodPost, "/v1/recipients", map[string]string{"recipient": "age1nope"}).Code)
	assert.Equal(t, http.StatusUnprocessableEntity, c.call(http.MethodPost, "/v1/recipients", map[string]string{
		"recipient": identity.Recipient().String(), "device": strings.Repeat("d", 81),
	}).Code)

	rec = c.call(http.MethodPost, "/v1/recipients", map[string]string{"recipient": identity.Recipient().String(), "device": "laptop"})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	created := decode[wire.Recipient](t, rec)
	assert.Equal(t, identity.Recipient().String(), created.Recipient)
	assert.Equal(t, "laptop", created.Device)

	rec = c.call(http.MethodGet, "/v1/recipients", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, []wire.Recipient{created}, decode[wire.Recipients](t, rec).Recipients)

	rec = c.call(http.MethodPatch, "/v1/me", map[string]string{"encryption": "on"})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.True(t, decode[wire.Me](t, rec).Encryption)
	assert.True(t, decode[wire.Me](t, c.call(http.MethodGet, "/v1/me", nil)).Encryption)

	assert.Equal(t, http.StatusUnprocessableEntity, c.call(http.MethodDelete, "/v1/recipients/"+created.ID, nil).Code)

	src, token := h.source(owner, "")
	ingress := ingressPost(token, `{"secret":"payload"}`)
	ingress.Header.Set("Content-Type", "application/json")
	rec = h.do(ingress)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var accepted map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &accepted))
	assert.NotEmpty(t, accepted["id"])

	rec = c.call(http.MethodGet, "/v1/sources/"+src.ID+"/deliveries", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.NotContains(t, rec.Body.String(), "payload")
	var page struct {
		Deliveries []map[string]any `json:"deliveries"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &page))
	require.Len(t, page.Deliveries, 1)
	d := page.Deliveries[0]
	keys := make([]string, 0, len(d))
	for k := range d {
		keys = append(keys, k)
	}
	assert.ElementsMatch(t, []string{"id", "received_at", "content_length", "enc", "blob"}, keys)
	assert.Equal(t, accepted["id"], d["id"])
	assert.Equal(t, "age", d["enc"])

	blob, err := base64.StdEncoding.DecodeString(d["blob"].(string))
	require.NoError(t, err)
	r, err := age.Decrypt(bytes.NewReader(blob), identity)
	require.NoError(t, err)
	raw, err := io.ReadAll(r)
	require.NoError(t, err)
	var record relay.Record
	require.NoError(t, json.Unmarshal(raw, &record))
	assert.JSONEq(t, `{"secret":"payload"}`, string(record.Body))

	rec = c.call(http.MethodPatch, "/v1/me", map[string]string{"encryption": "off"})
	require.Equal(t, http.StatusOK, rec.Code)
	assert.False(t, decode[wire.Me](t, rec).Encryption)
	assert.Equal(t, http.StatusNoContent, c.call(http.MethodDelete, "/v1/recipients/"+created.ID, nil).Code)
	assert.Equal(t, http.StatusNotFound, c.call(http.MethodDelete, "/v1/recipients/"+created.ID, nil).Code)
	assert.Empty(t, decode[wire.Recipients](t, c.call(http.MethodGet, "/v1/recipients", nil)).Recipients)
}
