package relay

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/colonyops/hive/cmd/desktop/internal/app/icons"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sourcemark"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/connector"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/relayformat"
	"github.com/colonyops/hive/internal/platform/credentials"
	"github.com/colonyops/hive/pkg/duration"
)

// Config is a relay source node's configuration.
type Config struct {
	Credential string            `json:"credential"         yaml:"credential"         jsonschema:"title=Credential,description=The connected relay account to fetch as - written 'relay/<host>-<email>'."`
	Source     string            `json:"source"             yaml:"source"             jsonschema:"title=Relay source,description=The relay source id this node reads."`
	Dialect    string            `json:"dialect,omitempty"  yaml:"dialect,omitempty"  jsonschema:"title=Format,description=How a delivery is parsed into items. Empty or auto detects it; generic is the local webhook contract.,enum=auto,enum=generic,enum=ntfy,enum=gotify,enum=healthchecks"`
	Map        *Map              `json:"map,omitempty"      yaml:"map,omitempty"      jsonschema:"title=Map,description=JSON Pointers that overlay the parsed item's key and fields."`
	Checks     map[string]Check  `json:"checks,omitempty"   yaml:"checks,omitempty"   jsonschema:"title=Checks,description=Healthchecks period and grace per slug. A check with no successful ping for period plus grace goes down."`
	Interval   duration.Duration `json:"interval,omitempty" yaml:"interval,omitempty" jsonschema:"title=Minimum interval,description=Shortest time between fetches. Empty fetches on every poll tick."`
	Icon       string            `json:"icon,omitempty"     yaml:"icon,omitempty"     jsonschema:"title=Item icon,description=The glyph feed rows render for this node's items. Empty uses the default feed glyph."`
	Image      string            `json:"image,omitempty"    yaml:"image,omitempty"    jsonschema:"title=Image,description=Content hash of an uploaded image shown as this source's feed mark instead of the icon. Empty falls back to the icon."`
}

// Map overlays a parsed item. Each field is an RFC 6901 JSON Pointer over the
// item payload (canonical fields plus raw); empty means unset.
type Map struct {
	Key    string            `json:"key,omitempty"    yaml:"key,omitempty"    jsonschema:"title=Key pointer,description=JSON Pointer to the value that identifies the item."`
	Title  string            `json:"title,omitempty"  yaml:"title,omitempty"  jsonschema:"title=Title pointer,description=JSON Pointer to the item title."`
	URL    string            `json:"url,omitempty"    yaml:"url,omitempty"    jsonschema:"title=URL pointer,description=JSON Pointer to the item link."`
	Kind   string            `json:"kind,omitempty"   yaml:"kind,omitempty"   jsonschema:"title=Kind pointer,description=JSON Pointer to the item kind."`
	State  string            `json:"state,omitempty"  yaml:"state,omitempty"  jsonschema:"title=State pointer,description=JSON Pointer to the item state."`
	States map[string]string `json:"states,omitempty" yaml:"states,omitempty" jsonschema:"title=State values,description=Maps a resolved state value to the canonical state it means."`
}

// Check is one healthchecks slug's schedule.
type Check struct {
	Period duration.Duration `json:"period" yaml:"period" jsonschema:"title=Period,description=How often the check is expected to ping."`
	Grace  duration.Duration `json:"grace"  yaml:"grace"  jsonschema:"title=Grace,description=How long past the period a ping may be late before the check is down."`
}

func (c *Config) MarkImage() string        { return c.Image }
func (c *Config) SetMarkImage(hash string) { c.Image = hash }

func (c *Config) dialect() string {
	if c.Dialect == "" {
		return relayformat.DialectAuto
	}
	return c.Dialect
}

func (c *Config) Validate() error {
	if _, err := c.CredentialRef(); err != nil {
		return err
	}
	if strings.TrimSpace(c.Source) == "" {
		return errors.New("relay source: source is required")
	}
	if err := validateDialect(c.Dialect); err != nil {
		return err
	}
	if err := c.Map.validate(); err != nil {
		return err
	}
	for slug, check := range c.Checks {
		if check.Period <= 0 {
			return fmt.Errorf("relay source: check %q needs a period greater than zero", slug)
		}
		if check.Grace < 0 {
			return fmt.Errorf("relay source: check %q has a negative grace", slug)
		}
	}
	if err := connector.ValidateInterval("relay source", c.Interval); err != nil {
		return err
	}
	if !icons.ValidFeed(c.Icon) {
		return fmt.Errorf("relay source: icon %q is not a supported feed icon", c.Icon)
	}
	if c.Image != "" && !sourcemark.ValidHash(c.Image) {
		return fmt.Errorf("relay source: image %q is not a valid mark reference", c.Image)
	}
	return nil
}

func validateDialect(dialect string) error {
	if dialect == "" || slices.Contains(relayformat.Dialects(), dialect) {
		return nil
	}
	return fmt.Errorf("relay source: format %q is not one of %s", dialect, strings.Join(relayformat.Dialects(), ", "))
}

func (m *Map) validate() error {
	if m == nil {
		return nil
	}
	for _, f := range m.fields() {
		if _, err := parsePointer(f.pointer); err != nil {
			return fmt.Errorf("relay source: map.%s: %w", f.name, err)
		}
	}
	return nil
}

// CredentialRef rejects a ref naming another provider at load, where it reads
// as a config mistake rather than an empty feed.
func (c *Config) CredentialRef() (credentials.Ref, error) {
	if strings.TrimSpace(c.Credential) == "" {
		return credentials.Ref{}, fmt.Errorf("relay source: credential is required (e.g. %q)", Provider+"/relay.hivedesktop.com-you@example.com")
	}
	ref, err := credentials.ParseRef(c.Credential)
	if err != nil {
		return credentials.Ref{}, fmt.Errorf("relay source: %w", err)
	}
	if ref.Provider != Provider {
		return credentials.Ref{}, fmt.Errorf("relay source: credential %q is not a %s credential", c.Credential, Provider)
	}
	return ref, nil
}
