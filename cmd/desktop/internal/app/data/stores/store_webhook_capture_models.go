package stores

import "github.com/colonyops/hive/cmd/desktop/internal/app/data/queries"

type WebhookCapture struct {
	Topic      string `json:"topic"`
	ReceivedAt int64  `json:"receivedAt"`
	Body       []byte `json:"body"`
}

func mapWebhookCaptureFromDB(row queries.WebhookCapture) WebhookCapture {
	return WebhookCapture{Topic: row.Topic, ReceivedAt: row.ReceivedAt, Body: row.Body}
}
