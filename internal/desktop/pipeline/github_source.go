package pipeline

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/colonyops/hive/internal/desktop/feed"
	"github.com/colonyops/hive/internal/desktop/pipeline/flow"
)

// githubSource is the Source that produces one flow github-source node's
// current items. It does not fetch GitHub itself: it delegates to
// feed.LiveProvider.SourceItems, the same coalesced, cached, conditional-
// request fetch path the desktop feed already uses — so the pipeline gains a
// producer without a second implementation of GitHub fetching to keep in
// sync.
type githubSource struct {
	live  *feed.LiveProvider
	def   feed.SourceDef
	topic string // "source:<flowId>/<nodeId>"
}

// searchDef exposes this source's definition for the producer's batched
// prefetch. Notifications retain their independent conditional REST fetch.
func (s *githubSource) searchDef() (feed.SourceDef, bool) {
	return s.def, s.def.Kind == "search"
}

// Produce emits one Msg per current item of the source, JSON-encoding
// feed.Item as the payload. Topic is the flow-qualified
// "source:<flowId>/<nodeId>" so a frontend graph only ingests its own source
// nodes' rows; Key is the item's stable ID, used to skip unchanged source
// values within that topic.
func (s *githubSource) Produce(ctx context.Context, emit func(Msg) error) error {
	items, err := s.live.SourceItems(ctx, s.def)
	if err != nil {
		return fmt.Errorf("pipeline: fetching source %q: %w", s.def.ID, err)
	}

	for _, item := range items {
		payload, err := json.Marshal(item)
		if err != nil {
			return fmt.Errorf("pipeline: encoding item %q from source %q: %w", item.ID, s.def.ID, err)
		}
		msg := Msg{
			Key:     item.ID,
			Topic:   s.topic,
			Payload: payload,
		}
		if err := emit(msg); err != nil {
			return err
		}
	}
	return nil
}

// FlowLister is the subset of *flow.FlowStore the source lister needs: the
// current set of loaded flows. Producer calls it once per tick — rather than
// fixing the set at construction — so a source node added to, edited in, or
// removed from any flow takes effect on the next tick without a restart.
type FlowLister interface {
	List() []flow.Flow
}

// NewFlowSourceLister returns a SourceLister over every enabled github-source
// node across all flows: one githubSource per node, keyed and topic-tagged by
// its flow-qualified id "<flowId>/<nodeId>". Two nodes with identical fetch
// config still share one GitHub request — LiveProvider keys its cache on
// kind+query+limit, not id — while producing distinct topics so each flow's
// graph ingests only its own rows.
func NewFlowSourceLister(live *feed.LiveProvider, flows FlowLister) SourceLister {
	return func(context.Context) (map[string]Source, error) {
		out := map[string]Source{}
		for _, f := range flows.List() {
			if !f.Enabled {
				continue
			}
			for _, node := range f.Nodes {
				if node.Disabled || node.Type != "github-source" {
					continue
				}
				cfg, ok := node.Config.(*flow.GithubSourceConfig)
				if !ok {
					continue
				}
				id := f.ID + "/" + node.ID
				out[id] = &githubSource{
					live:  live,
					def:   feed.SourceDef{ID: id, Kind: cfg.Kind, Query: cfg.Query, Limit: cfg.Limit},
					topic: "source:" + id,
				}
			}
		}
		return out, nil
	}
}
