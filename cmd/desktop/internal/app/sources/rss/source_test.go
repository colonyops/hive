package rss

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/connector"
)

type stubFetcher struct {
	entries []Entry
	err     error
	urls    []string
}

func (f *stubFetcher) Fetch(_ context.Context, feedURL string) ([]Entry, error) {
	f.urls = append(f.urls, feedURL)
	return f.entries, f.err
}

func window(n int) []Entry {
	entries := make([]Entry, n)
	for i := range entries {
		entries[i] = Entry{Key: strconv.Itoa(i), Payload: []byte(`{"title":"` + strconv.Itoa(i) + `"}`)}
	}
	return entries
}

func drain(t *testing.T, src *source) []models.Msg {
	t.Helper()
	var got []models.Msg
	require.NoError(t, src.Produce(t.Context(), func(msg models.Msg) error {
		got = append(got, msg)
		return nil
	}))
	return got
}

func TestProduceEmitsOneMessagePerEntry(t *testing.T) {
	t.Parallel()

	fetcher := &stubFetcher{entries: window(2)}
	src := &source{id: "flow/node", topic: "source:flow/node", url: "https://example.com/feed", limit: 10, fetcher: fetcher}

	got := drain(t, src)

	require.Len(t, got, 2)
	assert.Equal(t, []string{"https://example.com/feed"}, fetcher.urls)
	assert.Equal(t, "0", got[0].Key)
	assert.Equal(t, "source:flow/node", got[0].Topic)
	assert.Equal(t, SourceKind, got[0].SourceKind)
}

// Entries arrive newest first, so the limit cuts the old end of the window.
func TestProduceCutsTheWindowToTheLimit(t *testing.T) {
	t.Parallel()

	src := &source{topic: "t", limit: 2, fetcher: &stubFetcher{entries: window(5)}}

	got := drain(t, src)

	require.Len(t, got, 2)
	assert.Equal(t, "0", got[0].Key)
	assert.Equal(t, "1", got[1].Key)
}

// A failed fetch must emit nothing: the producer treats a successful Produce as
// the source's current set, so half a window would archive the rest.
func TestProduceEmitsNothingWhenTheFetchFails(t *testing.T) {
	t.Parallel()

	src := &source{id: "flow/node", topic: "t", limit: 10, fetcher: &stubFetcher{err: errors.New("boom")}}

	var emitted int
	err := src.Produce(t.Context(), func(models.Msg) error {
		emitted++
		return nil
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "flow/node")
	assert.Zero(t, emitted)
}

func TestNewFactoryBuildsAPullInstance(t *testing.T) {
	t.Parallel()

	factory := NewFactory(NewFetchers(zerolog.Nop()))
	node := connector.Node{FlowID: "flow", NodeID: "node", Policy: models.ResurfacePolicyStateChanges}

	instance, err := factory.New(node, &Config{URL: "https://example.com/feed", Limit: 7})
	require.NoError(t, err)

	assert.Equal(t, Descriptor.Type, instance.Type)
	assert.NotNil(t, instance.Pull)
	assert.Equal(t, SourceKind, instance.Metadata.SourceKind)
	assert.Equal(t, "node", instance.Metadata.SourceScope, "several feeds in one flow are separated by node id")
	assert.Equal(t, models.ResurfacePolicyStateChanges, instance.Metadata.Policy)

	// Declared capabilities are wired, and this connector declares none: an
	// entry that scrolled out of the window is old, not resolved.
	assert.Nil(t, instance.Classifier)
	assert.Nil(t, instance.Absence)
}

func TestNewFactoryRejectsAnotherConnectorsConfig(t *testing.T) {
	t.Parallel()

	_, err := NewFactory(NewFetchers(zerolog.Nop())).New(connector.Node{FlowID: "f", NodeID: "n"}, nil)
	require.Error(t, err)
}
