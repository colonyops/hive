package grafana

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/connector"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/grafana/client"
)

// metricsSource polls one node's PromQL query, emitting one message per tick
// keyed by the node id, carrying the whole query result. A downstream function
// node can split that result into one durable feed item per series under a key
// it mints (ADR function-node-per-entity-feed-items), or leave it as the single node-level item this emits.
type metricsSource struct {
	fetcher *fetcher
	dsUID   string
	expr    string
	title   string
	topic   string
	key     string
}

var _ connector.PullSource = (*metricsSource)(nil)

// payload is what a metrics poll emits. title is promoted onto the inbox item at
// ingest; result is the opaque query response a function node reads. There is no
// url — a metric has no canonical link.
type payload struct {
	Title  string             `json:"title"`
	Result client.QueryResult `json:"result"`
}

// Produce runs the query and emits one message. A fetch error returns without
// emitting, leaving the previous snapshot in place rather than clearing it.
func (s *metricsSource) Produce(ctx context.Context, emit func(models.Msg) error) error {
	result, err := s.fetcher.Query(ctx, s.dsUID, s.expr)
	if err != nil {
		return fmt.Errorf("grafana metrics %q: %w", s.key, err)
	}
	body, err := json.Marshal(payload{Title: s.itemTitle(), Result: result})
	if err != nil {
		return fmt.Errorf("grafana metrics %q: encoding payload: %w", s.key, err)
	}
	return emit(models.Msg{Key: s.key, Topic: s.topic, SourceKind: SourceKind, Payload: body})
}

func (s *metricsSource) itemTitle() string {
	if t := strings.TrimSpace(s.title); t != "" {
		return t
	}
	return "Grafana: " + s.expr
}
