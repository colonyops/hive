package stores

import (
	"context"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/queries"
)

type SourceHeadStore struct {
	q *queries.DB
}

func NewSourceHeadStore(q *queries.DB, _ Options) *SourceHeadStore {
	return &SourceHeadStore{q: q}
}

// ListActiveKeys returns membership for one source topic, limited to keys
// whose inbox row still exists and is not archived.
func (s *SourceHeadStore) ListActiveKeys(ctx context.Context, id SourceIdentity) ([]string, error) {
	keys, err := s.q.Ctx(ctx).ListActiveSourceHeadKeys(ctx, queries.ListActiveSourceHeadKeysParams{
		Topic:       id.Topic,
		ProfileID:   id.ProfileID,
		SourceKind:  id.SourceKind,
		SourceScope: id.SourceScope,
	})
	return keys, wrap("listing active source head keys", err)
}

func (s *SourceHeadStore) Payload(ctx context.Context, topic, key string) ([]byte, error) {
	payload, err := s.q.Ctx(ctx).GetSourceHeadPayload(ctx, queries.GetSourceHeadPayloadParams{Topic: topic, Key: key})
	return payload, wrap("getting source head payload", err)
}

func (s *SourceHeadStore) Upsert(ctx context.Context, topic, key string, payload []byte) error {
	return wrap("upserting source head", s.q.Ctx(ctx).UpsertSourceHead(ctx, queries.UpsertSourceHeadParams{
		Topic: topic, Key: key, Payload: payload,
	}))
}

func (s *SourceHeadStore) Delete(ctx context.Context, topic, key string) error {
	return wrap("deleting source head", s.q.Ctx(ctx).DeleteSourceHead(ctx, queries.DeleteSourceHeadParams{Topic: topic, Key: key}))
}

// topicPrefix is matched literally.
func (s *SourceHeadStore) DeleteByTopicPrefix(ctx context.Context, topicPrefix string) error {
	return wrap("deleting source heads by topic prefix", s.q.Ctx(ctx).DeleteSourceHeadByTopicPrefix(ctx, likePrefix(topicPrefix)))
}
