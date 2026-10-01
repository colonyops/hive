package webhook

import (
	"fmt"
	"strings"

	"github.com/colonyops/hive/cmd/desktop/internal/app/icons"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sourcemark"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/connector"
)

// SourceKind is the inbox source_kind webhook observations carry.
const SourceKind = "webhook"

// PathPrefix is the URL prefix every webhook source node's path is served
// under, leaving the listener's root free for future control routes.
const PathPrefix = "/hooks/"

// SecretHeader carries a webhook source node's shared secret. The header is
// compared in constant time against the node's configured value.
const SecretHeader = "X-Hive-Secret"

// Path/secret bounds. Paths are URL segments under the local listener's
// /hooks/ prefix, so they stay slug-shaped; the secret is an opaque header
// value with no shape beyond printable-ASCII sanity.
const (
	maxPathLen   = 128
	maxSecretLen = 128
)

// Descriptor declares the connector. It is push: deliveries arrive on the
// local listener rather than being drained on a tick, which is why the two
// modes are separate — a push connector has no blocking read to fake.
//
// It declares CapClassify only. There is no absence to confirm (a sender that
// stops sending says nothing about the item's fate) and nothing to batch.
var Descriptor = connector.Descriptor{
	Type:  "sources.webhook",
	Title: "Webhook source",
	// No Provider: the listener is local ingress. There is nothing to
	// authenticate as, so there is no account to connect and no card action.
	Mode:         connector.ModePush,
	Stability:    connector.Stable,
	Capabilities: connector.CapClassify,
	NewConfig:    func() connector.Config { return &Config{} },
}

// Config is a webhook source node's configuration. The node declares a path
// on the desktop's local webhook listener; JSON POSTed to /hooks/<path> is
// ingested as this node's observations — the same delivery contract as the
// GitHub source, minus the poll loop.
//
// Several nodes, across flows, may declare the same path: each enabled one
// receives every request, so one sender can fan into multiple flows.
type Config struct {
	// Path is the endpoint under /hooks/.
	Path string `json:"path" yaml:"path" jsonschema:"title=Path,description=The endpoint under /hooks/ such as 'ci-alerts' or 'ci/deploys'. One or more lowercase slug segments separated by '/' with no leading or trailing slash."`
	// Secret, when set, requires senders to present the same value in the
	// X-Hive-Secret request header. Requests without it are rejected 401.
	Secret string `json:"secret,omitempty" yaml:"secret,omitempty" jsonschema:"title=Secret,description=When set senders must present this value in the X-Hive-Secret header. Requests without it are rejected."`
	// Icon is the glyph feed rows render for this node's items, from the
	// shared feed icon set. Purely cosmetic; empty means the default.
	Icon string `json:"icon,omitempty" yaml:"icon,omitempty" jsonschema:"title=Icon,description=The glyph feed rows render for this node's items. Empty uses the default webhook glyph."`
	// Image, when set, is the content hash of an uploaded image shown as this
	// node's feed mark instead of Icon (see cmd/desktop/internal/app/sourcemark). Empty or a
	// missing file falls back to Icon.
	Image string `json:"image,omitempty" yaml:"image,omitempty" jsonschema:"title=Image,description=Content hash of an uploaded image shown as this source's feed mark instead of the icon. Set through the node editor's image picker; empty falls back to the icon."`
}

// MarkImage and SetMarkImage read and record the feed-mark image hash, satisfying
// the flow store's image-mark interface.
func (c *Config) MarkImage() string        { return c.Image }
func (c *Config) SetMarkImage(hash string) { c.Image = hash }

// Validate checks the path is slug-shaped, the secret is a usable header
// value, and the icon is one the feed renders.
func (c *Config) Validate() error {
	if strings.TrimSpace(c.Path) == "" {
		return fmt.Errorf("webhook source: path is required")
	}
	if len(c.Path) > maxPathLen {
		return fmt.Errorf("webhook source: path exceeds %d characters", maxPathLen)
	}
	for segment := range strings.SplitSeq(c.Path, "/") {
		if !validPathSegment(segment) {
			return fmt.Errorf("webhook source: invalid path %q (want lowercase slug segments like \"ci-alerts\" or \"ci/deploys\")", c.Path)
		}
	}
	if len(c.Secret) > maxSecretLen {
		return fmt.Errorf("webhook source: secret exceeds %d characters", maxSecretLen)
	}
	for _, r := range c.Secret {
		if r < '!' || r > '~' {
			return fmt.Errorf("webhook source: secret must be printable ASCII without spaces")
		}
	}
	if !icons.ValidFeed(c.Icon) {
		return fmt.Errorf("webhook source: icon %q is not a supported feed icon", c.Icon)
	}
	if c.Image != "" && !sourcemark.ValidHash(c.Image) {
		return fmt.Errorf("webhook source: image %q is not a valid mark reference", c.Image)
	}
	return nil
}

// validPathSegment reports whether one "/"-separated path segment is a slug:
// a lowercase letter or digit followed by lowercase letters, digits, "-" or
// "_".
func validPathSegment(segment string) bool {
	if segment == "" {
		return false
	}
	for i, r := range segment {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
		case i > 0 && (r == '-' || r == '_'):
		default:
			return false
		}
	}
	return true
}
