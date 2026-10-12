package relay

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/connector"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/relayformat"
	"github.com/colonyops/hive/internal/platform/credentials"
)

const pageSize = 200

type source struct {
	id, topic string
	cfg       *Config
	ref       credentials.Ref
	fetchers  *Fetchers
}

var _ connector.PullSource = (*source)(nil)

// Produce reads every delivery after the node's cursor, merges the parsed
// items into the node's set, and emits the whole set. Every page is fetched
// before the set changes, so a failed tick leaves the cursor and the set as
// they were and emits nothing.
func (s *source) Produce(ctx context.Context, emit func(models.Msg) error) error {
	client, err := s.fetchers.For(s.ref)
	if err != nil {
		return fmt.Errorf("relay source %q: %w", s.id, err)
	}

	st := s.fetchers.state(s.id)
	st.mu.Lock()
	defer st.mu.Unlock()

	cursor := st.cursor
	var fetched []DeliveryView
	for {
		page, err := client.Deliveries(ctx, s.cfg.Source, cursor, pageSize)
		if err != nil {
			return fmt.Errorf("relay source %q: %w", s.id, err)
		}
		fetched = append(fetched, page.Deliveries...)
		if page.Next != "" {
			cursor = page.Next
		}
		if !page.More || len(page.Deliveries) == 0 {
			break
		}
	}

	type parsed struct {
		key string
		it  item
	}
	var batch []parsed
	for _, view := range fetched {
		d, ok := s.delivery(view)
		if !ok {
			continue
		}
		items, _, err := relayformat.Parse(s.cfg.dialect(), d)
		if err != nil {
			return fmt.Errorf("relay source %q: delivery %s: %w", s.id, view.ID, err)
		}
		for _, parsedItem := range items {
			mapped := Apply(s.cfg.Map, parsedItem)
			batch = append(batch, parsed{key: mapped.Key, it: item{payload: mapped.Payload, receivedAt: d.ReceivedAt}})
		}
	}

	st.retention, st.maxKeys = s.fetchers.window(ctx, s.ref, client)
	for _, p := range batch {
		st.merge(p.key, p.it)
	}
	st.cursor = cursor
	st.prune(time.Now())

	trace.SpanFromContext(ctx).SetAttributes(
		attribute.String("relay.source", s.cfg.Source),
		attribute.Int("relay.deliveries", len(fetched)),
		attribute.Int("relay.items", len(st.order)),
	)

	for _, key := range st.order {
		payload, err := json.Marshal(st.items[key].payload)
		if err != nil {
			return fmt.Errorf("relay source %q: encode item %q: %w", s.id, key, err)
		}
		if err := emit(models.Msg{Key: key, Topic: s.topic, SourceKind: SourceKind, Payload: payload}); err != nil {
			return err
		}
	}
	return nil
}

// delivery decodes a wire row. A sealed row needs the account's age identity,
// which arrives with encryption in a later release, so it is skipped.
func (s *source) delivery(view DeliveryView) (relayformat.Delivery, bool) {
	if view.Enc != "" {
		s.fetchers.log.Debug().Str("node", s.id).Str("delivery", view.ID).Msg("relay: skipping a sealed delivery")
		return relayformat.Delivery{}, false
	}
	d, err := toDelivery(view)
	if err != nil {
		s.fetchers.log.Warn().Err(err).Str("node", s.id).Str("delivery", view.ID).Msg("relay: skipping an undecodable delivery")
		return relayformat.Delivery{}, false
	}
	return d, true
}

func toDelivery(view DeliveryView) (relayformat.Delivery, error) {
	body := []byte(view.Body)
	if view.BodyBase64 != "" {
		decoded, err := base64.StdEncoding.DecodeString(view.BodyBase64)
		if err != nil {
			return relayformat.Delivery{}, fmt.Errorf("decode body_b64: %w", err)
		}
		body = decoded
	}
	return relayformat.Delivery{
		ID:          view.ID,
		ReceivedAt:  view.ReceivedAt,
		Method:      view.Method,
		Path:        view.Path,
		Query:       view.Query,
		Headers:     view.Headers,
		Body:        body,
		ContentType: view.ContentType,
	}, nil
}
