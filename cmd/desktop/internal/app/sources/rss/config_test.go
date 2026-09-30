package rss

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/connector"
)

func TestValidateAcceptsAFeedURL(t *testing.T) {
	t.Parallel()

	for name, cfg := range map[string]*Config{
		"https":            {URL: "https://example.com/feed.xml"},
		"http on a LAN":    {URL: "http://nas.local:8080/feed"},
		"with every field": {URL: "https://example.com/atom", Limit: 10, Interval: connector.Duration(0), Icon: "rss"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.NoError(t, cfg.Validate())
		})
	}
}

// Each of these fails on every tick if it is not caught at load, and the two
// URL cases fail in ways a user reads as "the feed is broken" rather than "the
// node is wrong".
func TestValidateRejects(t *testing.T) {
	t.Parallel()

	for name, cfg := range map[string]*Config{
		"no url":               {},
		"a bare host":          {URL: "example.com/feed.xml"},
		"a non-http scheme":    {URL: "file:///etc/feed.xml"},
		"a url with no host":   {URL: "https:///feed.xml"},
		"embedded userinfo":    {URL: "https://user:pass@example.com/feed"},
		"a negative limit":     {URL: "https://example.com/feed", Limit: -1},
		"a limit past the max": {URL: "https://example.com/feed", Limit: maxLimit + 1},
		"a negative interval":  {URL: "https://example.com/feed", Interval: connector.Duration(-1)},
		"an unknown icon":      {URL: "https://example.com/feed", Icon: "not-an-icon"},
		"a malformed image":    {URL: "https://example.com/feed", Image: "nope"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.Error(t, cfg.Validate())
		})
	}
}

// A node that names no limit still has one. Without it the first fetch of a
// feed with a long archive ingests every entry it publishes.
func TestEntryLimitDefaults(t *testing.T) {
	t.Parallel()

	assert.Equal(t, defaultLimit, (&Config{}).EntryLimit())
	assert.Equal(t, defaultLimit, (&Config{Limit: 0}).EntryLimit())
	assert.Equal(t, 5, (&Config{Limit: 5}).EntryLimit())
}

// The descriptor is the whole declaration, and the two fields this connector
// deliberately leaves empty are the ones that would change its semantics: a
// Provider would claim it authenticates, and either capability would make an
// entry that scrolled out of the window look resolved.
func TestDescriptorDeclaresNoCredentialAndNoCapabilities(t *testing.T) {
	t.Parallel()

	require.Equal(t, "sources.rss", Descriptor.Type)
	assert.Empty(t, Descriptor.Provider)
	assert.Zero(t, Descriptor.Capabilities)
	assert.Equal(t, connector.ModePull, Descriptor.Mode)
}
