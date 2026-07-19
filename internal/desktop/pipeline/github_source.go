package pipeline

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/colonyops/hive/internal/desktop/feed"
)

// githubSource is the Source that produces one configured feed.SourceDef's
// current items. It does not fetch GitHub itself: it delegates to
// feed.LiveProvider.SourceItems, which is the same coalesced, cached,
// conditional-request fetch path (client/auth/singleflight) the feed
// service already uses — so the pipeline gains a producer without a second
// implementation of GitHub fetching to keep in sync.
type githubSource struct {
	live *feed.LiveProvider
	def  feed.SourceDef
}

// Produce emits one Msg per current item of the source, JSON-encoding
// feed.Item as the payload. Topic is "source:<sourceID>" and Key is the
// item's stable ID, so pipelinedb's per-key compaction collapses repeated
// appends of the same item down to its latest value.
func (s *githubSource) Produce(ctx context.Context, emit func(Msg) error) error {
	items, err := s.live.SourceItems(ctx, s.def)
	if err != nil {
		return fmt.Errorf("pipeline: fetching source %q: %w", s.def.ID, err)
	}

	topic := "source:" + s.def.ID
	for _, item := range items {
		payload, err := json.Marshal(item)
		if err != nil {
			return fmt.Errorf("pipeline: encoding item %q from source %q: %w", item.ID, s.def.ID, err)
		}
		msg := Msg{
			Key:     item.ID,
			Topic:   topic,
			Payload: payload,
			Meta: map[string]any{
				"source": s.def.ID,
				"kind":   item.Kind,
				"repo":   item.Repo,
			},
		}
		if err := emit(msg); err != nil {
			return err
		}
	}
	return nil
}

// NewGithubSourceLister returns a SourceLister over the live provider's
// currently configured sources, one githubSource per feed.SourceDef, keyed
// by the source's ID. It re-reads live.Sources on every call (Producer
// calls it once per tick), so a source added to or removed from the
// profiles config takes effect on the next tick without a restart.
func NewGithubSourceLister(live *feed.LiveProvider) SourceLister {
	return func(ctx context.Context) (map[string]Source, error) {
		defs, err := live.Sources(ctx)
		if err != nil {
			return nil, err
		}
		out := make(map[string]Source, len(defs))
		for _, def := range defs {
			out[def.ID] = &githubSource{live: live, def: def}
		}
		return out, nil
	}
}
