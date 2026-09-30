package github

import (
	"fmt"
	"strings"

	"github.com/colonyops/hive/cmd/desktop/internal/app/credentials"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/connector"
)

// Provider is the credentials provider name every GitHub credential is filed
// under. A ref is "github/<login>": the account half is the authenticated
// user's login, which the vendored single-slot token store — pinned to one
// constant keychain account — had no room to express.
const Provider = "github"

// The two fetch shapes a GitHub source can take. A search source runs a
// query; a notifications source drains the authenticated user's inbox.
const (
	KindSearch        = "search"
	KindNotifications = "notifications"
)

// Per-kind page caps. These are GitHub API facts and belong beside the code
// that calls it — they used to live in the flow package, whose own doc
// comment forbids it from importing this one, so the rules sat apart from
// what they constrain.
const (
	maxSearchLimit        = 100
	maxNotificationsLimit = 50
)

// Config is a GitHub source node's configuration. The framework decodes it —
// strictly, so an unknown key is an error — and calls Validate for the
// cross-field rules a schema cannot state.
type Config struct {
	// Credential names the account this source fetches as, "github/<login>".
	// A ref and never a token: flows/ is dotfiles-managed, so an embedded
	// token would be a token in a git repo.
	Credential string `json:"credential" yaml:"credential" jsonschema:"title=Credential,description=The connected GitHub account to fetch as, as 'github/<login>'."`
	// Kind selects the fetch shape.
	Kind string `json:"kind" yaml:"kind" jsonschema:"title=Kind,enum=search,enum=notifications,description=search runs a GitHub search query; notifications drains the authenticated user's inbox."`
	// Query is the GitHub search query. Required for kind "search"; a
	// notifications source takes none.
	Query string `json:"query,omitempty" yaml:"query,omitempty" jsonschema:"title=Query,description=A GitHub search query such as 'is:open is:pr archived:false'. Only for kind search."`
	// Limit bounds items per fetch. 0 means the per-kind default.
	Limit int `json:"limit,omitempty" yaml:"limit,omitempty" jsonschema:"title=Limit,minimum=0,maximum=100,description=Maximum items per fetch. Search caps at 100 and notifications at 50; 0 uses the default of 50."`
	// Interval is the floor between fetches, for a query that spends more
	// rate-limit budget than its freshness is worth.
	Interval connector.Duration `json:"interval,omitempty" yaml:"interval,omitempty" jsonschema:"title=Minimum interval,description=Shortest time between fetches. The source still only runs on a poll tick so the real cadence rounds up to the next one; empty fetches on every tick."`
}

// Validate mirrors the GitHub API's own constraints: search needs a query and
// caps at one page of 100, notifications takes no query and caps at 50.
func (c *Config) Validate() error {
	if _, err := c.CredentialRef(); err != nil {
		return err
	}
	switch c.Kind {
	case KindSearch:
		if strings.TrimSpace(c.Query) == "" {
			return fmt.Errorf("github source: kind %q requires a query", KindSearch)
		}
		if c.Limit > maxSearchLimit {
			return fmt.Errorf("github source: limit %d exceeds the search API page cap of %d", c.Limit, maxSearchLimit)
		}
	case KindNotifications:
		if strings.TrimSpace(c.Query) != "" {
			return fmt.Errorf("github source: kind %q takes no query", KindNotifications)
		}
		if c.Limit > maxNotificationsLimit {
			return fmt.Errorf("github source: limit %d exceeds the notifications API page cap of %d", c.Limit, maxNotificationsLimit)
		}
	case "":
		return fmt.Errorf("github source: kind is required (want %q or %q)", KindSearch, KindNotifications)
	default:
		return fmt.Errorf("github source: unknown kind %q (want %q or %q)", c.Kind, KindSearch, KindNotifications)
	}
	if c.Limit < 0 {
		return fmt.Errorf("github source: limit must not be negative")
	}
	return connector.ValidateInterval("github source", c.Interval)
}

// CredentialRef is the parsed credential ref. A ref naming another provider
// is rejected here rather than at fetch time: "grafana/prod" on a GitHub
// source is a config mistake, and failing it at load says so, where failing
// it at fetch surfaces as an empty feed.
func (c *Config) CredentialRef() (credentials.Ref, error) {
	if strings.TrimSpace(c.Credential) == "" {
		return credentials.Ref{}, fmt.Errorf("github source: credential is required (e.g. %q)", Provider+"/octocat")
	}
	ref, err := credentials.ParseRef(c.Credential)
	if err != nil {
		return credentials.Ref{}, fmt.Errorf("github source: %w", err)
	}
	if ref.Provider != Provider {
		return credentials.Ref{}, fmt.Errorf("github source: credential %q is not a %s credential", c.Credential, Provider)
	}
	return ref, nil
}
