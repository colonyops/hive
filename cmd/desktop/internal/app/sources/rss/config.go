package rss

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/colonyops/hive/cmd/desktop/internal/app/icons"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sourcemark"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/connector"
)

// defaultLimit is how many entries one fetch ingests when the node names no
// limit. A feed's archive can run to hundreds of entries, and every one of
// them arrives as a first observation on the tick a node is added — so the
// default bounds the burst a new source can produce.
const defaultLimit = 50

// maxLimit bounds the configurable limit. Past this the node is not a feed
// subscription any more, it is an archive import.
const maxLimit = 500

// Config is a feed source node's configuration.
type Config struct {
	// URL is the feed document. RSS, Atom and JSON Feed are all read through
	// the same field; the parser decides from the document, not the extension.
	URL string `json:"url" yaml:"url" jsonschema:"title=Feed URL,description=The RSS Atom or JSON Feed document to fetch each poll. It must be reachable without credentials."`
	// Limit caps how many of the feed's entries are ingested per fetch, most
	// recent first.
	Limit int `json:"limit,omitempty" yaml:"limit,omitempty" jsonschema:"title=Entry limit,description=How many of the feed's most recent entries to ingest per fetch. Empty is 50; at most 500."`
	// Interval is the floor between fetches. A feed is polite to poll
	// conditionally but rude to poll often, so most feeds want one.
	Interval connector.Duration `json:"interval,omitempty" yaml:"interval,omitempty" jsonschema:"title=Minimum interval,description=Shortest time between fetches. The feed still only loads on a poll tick so the real cadence rounds up to the next one; empty fetches on every tick."`
	// Icon is the glyph feed rows render for this node's items, from the
	// shared feed icon set. Purely cosmetic; empty means the default.
	Icon string `json:"icon,omitempty" yaml:"icon,omitempty" jsonschema:"title=Item icon,description=The glyph feed rows render for this node's items. Empty uses the default feed glyph."`
	// Image, when set, is the content hash of an uploaded image shown as this
	// node's feed mark instead of Icon (see cmd/desktop/internal/app/sourcemark). Empty or
	// a missing file falls back to Icon.
	Image string `json:"image,omitempty" yaml:"image,omitempty" jsonschema:"title=Image,description=Content hash of an uploaded image shown as this source's feed mark instead of the icon. Set through the node editor's image picker; empty falls back to the icon."`
}

// MarkImage and SetMarkImage read and record the feed-mark image hash, satisfying
// the flow store's image-mark interface.
func (c *Config) MarkImage() string        { return c.Image }
func (c *Config) SetMarkImage(hash string) { c.Image = hash }

// EntryLimit is the configured limit with the default applied.
func (c *Config) EntryLimit() int {
	if c.Limit <= 0 {
		return defaultLimit
	}
	return c.Limit
}

// Validate checks the URL is one an HTTP client can actually fetch and that
// the remaining fields are in range.
func (c *Config) Validate() error {
	if err := validateFeedURL(c.URL); err != nil {
		return err
	}
	if c.Limit < 0 || c.Limit > maxLimit {
		return fmt.Errorf("rss source: limit %d is out of range (1 to %d, or empty for %d)", c.Limit, maxLimit, defaultLimit)
	}
	if err := connector.ValidateInterval("rss source", c.Interval); err != nil {
		return err
	}
	if !icons.ValidFeed(c.Icon) {
		return fmt.Errorf("rss source: icon %q is not a supported feed icon", c.Icon)
	}
	if c.Image != "" && !sourcemark.ValidHash(c.Image) {
		return fmt.Errorf("rss source: image %q is not a valid mark reference", c.Image)
	}
	return nil
}

// validateFeedURL rejects at load what would otherwise fail on every tick.
// http is allowed alongside https because a self-hosted reader on a LAN is a
// real case and this connector sends no credential; userinfo is not, because
// a URL that carries one is a credential written into a file meant for a
// dotfiles repo.
func validateFeedURL(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fmt.Errorf("rss source: url is required")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("rss source: invalid url: %w", err)
	}
	switch parsed.Scheme {
	case "http", "https":
	default:
		return fmt.Errorf("rss source: url must be http or https, not %q (did you include https://?)", parsed.Scheme)
	}
	if parsed.Host == "" {
		return fmt.Errorf("rss source: url %q has no host", raw)
	}
	if parsed.User != nil {
		return fmt.Errorf("rss source: url must not contain a username or password")
	}
	return nil
}
