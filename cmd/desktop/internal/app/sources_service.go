package app

import (
	"context"

	"github.com/colonyops/hive/cmd/desktop/internal/app/ingest"
	ghsource "github.com/colonyops/hive/cmd/desktop/internal/app/sources/github"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/rss"
)

// SourcesService is ingestion on demand: a producer tick someone asked for
// rather than the next one settings.polling.interval would have run. Both
// entry points force the tick, so a source that declared an hourly cadence
// still runs.
//
// They differ in what happens to the fetch caches. A user pressing refresh is
// asserting that something upstream changed, so Refresh drops the caches and
// pays for the refetch. A flow edit asserts nothing about upstream — an added
// or retyped source node has nothing cached under its query anyway — so Run
// keeps them and stays cheap enough to fire on every deploy.
type SourcesService struct {
	producer *ingest.Producer
	fetchers *ghsource.Fetchers
	feeds    *rss.Fetchers
}

func newSourcesService(producer *ingest.Producer, fetchers *ghsource.Fetchers, feeds *rss.Fetchers) *SourcesService {
	return &SourcesService{producer: producer, fetchers: fetchers, feeds: feeds}
}

// Refresh clears the fetch caches and drains every source now. Engine commits
// are asynchronous, so a caller reading items back should retry briefly. Mock
// modes have no producer and return KindUnavailable.
func (s *SourcesService) Refresh(ctx context.Context) (ingest.TickSummary, error) {
	if s.fetchers != nil {
		s.fetchers.InvalidateAll()
	}
	// A feed's cache holds its ETag as well as its window, so dropping it is
	// what makes refresh refetch rather than answer 304 from the last window.
	if s.feeds != nil {
		s.feeds.InvalidateAll()
	}
	return s.Run(ctx)
}

// Run drains every source now against whatever the fetch caches already hold.
func (s *SourcesService) Run(ctx context.Context) (ingest.TickSummary, error) {
	if s.producer == nil {
		return ingest.TickSummary{}, Errorf(KindUnavailable, "source refresh is unavailable in this mode")
	}
	return s.producer.Refresh(ctx), nil
}
