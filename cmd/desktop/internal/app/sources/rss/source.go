package rss

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/connector"
)

// fetcher is the half of Fetchers one source uses, named here so a test can
// stand one in without the network.
type fetcher interface {
	Fetch(ctx context.Context, feedURL string) ([]Entry, error)
}

// source emits one feed's newest entries.
//
// What it emits is a window, not the whole feed: a publisher drops old entries
// as new ones arrive, and `limit` cuts it further.
type source struct {
	id      string
	topic   string
	url     string
	limit   int
	fetcher fetcher
}

var _ connector.PullSource = (*source)(nil)

// Produce fetches the feed and emits its newest entries.
//
// The whole document is parsed before the first emit, so a fetch that fails
// emits nothing rather than half a window.
func (s *source) Produce(ctx context.Context, emit func(models.Msg) error) error {
	entries, err := s.fetcher.Fetch(ctx, s.url)
	if err != nil {
		return fmt.Errorf("rss source %q: %w", s.id, err)
	}
	if len(entries) > s.limit {
		entries = entries[:s.limit]
	}

	// Attributes on the tick's own source span rather than a span of their
	// own: a fetch is one HTTP round trip, which otelhttp already spans, and
	// these are what explain a window that came back smaller than expected.
	// The feed reference drops the query, which is where a private feed keeps
	// its token.
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(
		attribute.String("rss.feed", feedRef(s.url)),
		attribute.Int("rss.entries", len(entries)),
		attribute.Int("rss.limit", s.limit),
	)

	for _, entry := range entries {
		if err := emit(models.Msg{Key: entry.Key, Topic: s.topic, SourceKind: SourceKind, Payload: entry.Payload}); err != nil {
			return err
		}
	}
	return nil
}
