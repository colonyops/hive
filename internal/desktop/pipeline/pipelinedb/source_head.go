package pipelinedb

import (
	"context"
	"fmt"
)

// ListSourceHeadKeys returns membership for exactly one source topic.
func (db *DB) ListSourceHeadKeys(ctx context.Context, topic string) ([]string, error) {
	keys, err := db.queries.ListSourceHeadKeys(ctx, topic)
	if err != nil {
		return nil, fmt.Errorf("listing source head keys: %w", err)
	}
	return keys, nil
}

// SourceHeadPayload returns one topic-scoped source membership payload.
func (db *DB) SourceHeadPayload(ctx context.Context, topic, key string) ([]byte, error) {
	payload, err := db.queries.GetSourceHeadPayload(ctx, GetSourceHeadPayloadParams{Topic: topic, Key: key})
	if err != nil {
		return nil, fmt.Errorf("getting source head payload: %w", err)
	}
	return payload, nil
}
