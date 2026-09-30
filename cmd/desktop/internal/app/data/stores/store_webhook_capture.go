package stores

import (
	"context"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/queries"
)

type WebhookCaptureStore struct {
	q *queries.DB
}

func NewWebhookCaptureStore(q *queries.DB, _ Options) *WebhookCaptureStore {
	return &WebhookCaptureStore{q: q}
}

func (s *WebhookCaptureStore) Upsert(ctx context.Context, topic string, receivedAt int64, body []byte) error {
	return wrap("upserting webhook capture", s.q.Ctx(ctx).UpsertWebhookCapture(ctx, queries.UpsertWebhookCaptureParams{
		Topic: topic, ReceivedAt: receivedAt, Body: body,
	}))
}

func (s *WebhookCaptureStore) Get(ctx context.Context, topic string) (WebhookCapture, error) {
	row, err := s.q.Ctx(ctx).GetWebhookCapture(ctx, topic)
	if err != nil {
		return WebhookCapture{}, errTransformQueryOne("webhook_capture", topic, err)
	}
	return mapWebhookCaptureFromDB(row), nil
}
