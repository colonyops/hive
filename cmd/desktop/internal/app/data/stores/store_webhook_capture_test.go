package stores

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWebhookCaptureStore_UpsertAndGet(t *testing.T) {
	st, _ := openTestStores(t)
	ctx := t.Context()

	_, err := st.WebhookCaptures.Get(ctx, "source:flow-1/hook")
	assert.True(t, IsNotFound(err), "nothing captured yet is not-found")

	require.NoError(t, st.WebhookCaptures.Upsert(ctx, "source:flow-1/hook", 100, []byte(`{"n":1}`)))
	capture, err := st.WebhookCaptures.Get(ctx, "source:flow-1/hook")
	require.NoError(t, err)
	assert.Equal(t, "source:flow-1/hook", capture.Topic)
	assert.Equal(t, int64(100), capture.ReceivedAt)
	assert.JSONEq(t, `{"n":1}`, string(capture.Body))

	require.NoError(t, st.WebhookCaptures.Upsert(ctx, "source:flow-1/hook", 200, []byte(`{"n":2}`)))
	capture, err = st.WebhookCaptures.Get(ctx, "source:flow-1/hook")
	require.NoError(t, err)
	assert.Equal(t, int64(200), capture.ReceivedAt)
	assert.JSONEq(t, `{"n":2}`, string(capture.Body))
}
