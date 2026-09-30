package gitea

import (
	"fmt"
	"strings"

	"github.com/colonyops/hive/cmd/desktop/internal/app/credentials"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/connector"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/gitea/giteaclient"
)

// Provider is the credentials provider name every Gitea credential is filed
// under. A ref is "gitea/<host>-<login>": the host is part of the account
// because a Gitea credential is only meaningful against the instance it was
// issued by, and one person routinely holds accounts on several.
const Provider = "gitea"

// The two fetch shapes a Gitea source can take. A search source runs a
// filtered issue/PR query; a notifications source drains the inbox.
const (
	KindSearch        = "search"
	KindNotifications = "notifications"
)

// What a search returns. Gitea's parameter is "type" with no all-items value —
// omitting it returns both — so the connector spells the third case rather
// than making "unset" mean something.
const (
	ItemsAll    = "all"
	ItemsIssues = "issues"
	ItemsPulls  = "pulls"
)

// Which lifecycle states a search returns.
const (
	StateOpen   = "open"
	StateClosed = "closed"
	StateAll    = "all"
)

// The relationships to the connected account a search can filter on. These are
// the connector's vocabulary, mapped onto Gitea's boolean parameters in
// searchRequests.
const (
	InvolvingCreated         = "created"
	InvolvingAssigned        = "assigned"
	InvolvingMentioned       = "mentioned"
	InvolvingReviewRequested = "review_requested"
	InvolvingReviewed        = "reviewed"
)

// involvementParams maps the config vocabulary onto the client's. Also the set
// Validate accepts, so an unknown value is rejected at load rather than
// silently dropped into an unfiltered search.
var involvementParams = map[string]string{
	InvolvingCreated:         giteaclient.InvolvementCreated,
	InvolvingAssigned:        giteaclient.InvolvementAssigned,
	InvolvingMentioned:       giteaclient.InvolvementMentioned,
	InvolvingReviewRequested: giteaclient.InvolvementReviewRequested,
	InvolvingReviewed:        giteaclient.InvolvementReviewed,
}

// Per-kind page caps. Gitea clamps the notification list to its
// api.MAX_RESPONSE_ITEMS (50 by default) but does not clamp the issue search
// at all, so the search cap is the connector's own: a source that pulls more
// than a page of items into an inbox every tick is a misconfiguration, not a
// large feed.
const (
	maxSearchLimit        = 100
	maxNotificationsLimit = 50
	defaultLimit          = 50
)

// Config is a Gitea source node's configuration.
//
// The search half is a set of typed filters rather than a query string because
// Gitea has no search DSL to carry one: its endpoint takes discrete
// parameters, and a text query is only free text over title and body.
type Config struct {
	// Credential names the account this source fetches as,
	// "gitea/<host>-<login>". A ref and never a token: flows/ is
	// dotfiles-managed, so an embedded token would be a token in a git repo.
	// The description carries no comma on purpose: the jsonschema tag parser
	// splits on one, so anything after it is dropped from the reflected schema.
	Credential string `json:"credential" yaml:"credential" jsonschema:"title=Credential,description=The connected Gitea account to fetch as - written 'gitea/<host>-<login>'."`
	// Kind selects the fetch shape.
	Kind string `json:"kind" yaml:"kind" jsonschema:"title=Kind,enum=search,enum=notifications,description=search runs a filtered issue and pull-request query; notifications drains the authenticated user's inbox."`
	// Items narrows a search to one item type. Empty means all.
	Items string `json:"items,omitempty" yaml:"items,omitempty" jsonschema:"title=Items,enum=all,enum=issues,enum=pulls,description=Which items a search returns. Only for kind search; defaults to all."`
	// State narrows a search by lifecycle state. Empty means open.
	State string `json:"state,omitempty" yaml:"state,omitempty" jsonschema:"title=State,enum=open,enum=closed,enum=all,description=Which states a search returns. Only for kind search; defaults to open."`
	// Involving filters to items the connected account has a relationship
	// with. Several are a union, not an intersection.
	Involving []string `json:"involving,omitempty" yaml:"involving,omitempty" jsonschema:"title=Involving,description=Return items the connected account is related to - created / assigned / mentioned / review_requested / reviewed. Several are combined as a union. Only for kind search."`
	// Owner limits a search to one user's or organization's repositories.
	Owner string `json:"owner,omitempty" yaml:"owner,omitempty" jsonschema:"title=Owner,description=Limit the search to one user's or organization's repositories. Only for kind search."`
	// Labels matches items carrying any of these labels.
	Labels []string `json:"labels,omitempty" yaml:"labels,omitempty" jsonschema:"title=Labels,description=Return items carrying any of these labels. Only for kind search."`
	// Text is a free-text query over title and body.
	Text string `json:"text,omitempty" yaml:"text,omitempty" jsonschema:"title=Text,description=Free-text search over title and body. Only for kind search."`
	// Limit bounds items per fetch. 0 means the per-kind default.
	Limit int `json:"limit,omitempty" yaml:"limit,omitempty" jsonschema:"title=Limit,minimum=0,maximum=100,description=Maximum items per fetch. Search caps at 100 and notifications at 50; 0 uses the default of 50."`
	// Interval is the floor between fetches, for an instance that should not
	// be searched on every tick.
	Interval connector.Duration `json:"interval,omitempty" yaml:"interval,omitempty" jsonschema:"title=Minimum interval,description=Shortest time between fetches. The source still only runs on a poll tick so the real cadence rounds up to the next one; empty fetches on every tick."`
}

// Validate rejects what Gitea would silently ignore. Its search endpoint
// answers an unknown state or type with an unfiltered page rather than an
// error, so a typo would read as "these are all my open pull requests" — which
// is why every enumerated value is checked here instead of being passed
// through.
func (c *Config) Validate() error {
	if _, err := c.CredentialRef(); err != nil {
		return err
	}
	switch c.Kind {
	case KindSearch:
		if err := c.validateSearch(); err != nil {
			return err
		}
		if c.Limit > maxSearchLimit {
			return fmt.Errorf("gitea source: limit %d exceeds the search cap of %d", c.Limit, maxSearchLimit)
		}
	case KindNotifications:
		if err := c.rejectSearchFields(); err != nil {
			return err
		}
		if c.Limit > maxNotificationsLimit {
			return fmt.Errorf("gitea source: limit %d exceeds the notifications page cap of %d", c.Limit, maxNotificationsLimit)
		}
	case "":
		return fmt.Errorf("gitea source: kind is required (want %q or %q)", KindSearch, KindNotifications)
	default:
		return fmt.Errorf("gitea source: unknown kind %q (want %q or %q)", c.Kind, KindSearch, KindNotifications)
	}
	if c.Limit < 0 {
		return fmt.Errorf("gitea source: limit must not be negative")
	}
	return nil
}

func (c *Config) validateSearch() error {
	switch c.Items {
	case "", ItemsAll, ItemsIssues, ItemsPulls:
	default:
		return fmt.Errorf("gitea source: unknown items %q (want %q, %q or %q)", c.Items, ItemsAll, ItemsIssues, ItemsPulls)
	}
	switch c.State {
	case "", StateOpen, StateClosed, StateAll:
	default:
		return fmt.Errorf("gitea source: unknown state %q (want %q, %q or %q)", c.State, StateOpen, StateClosed, StateAll)
	}
	seen := make(map[string]bool, len(c.Involving))
	for _, involving := range c.Involving {
		if _, ok := involvementParams[involving]; !ok {
			return fmt.Errorf("gitea source: unknown involving %q (want %s)", involving, involvingOptions())
		}
		if seen[involving] {
			return fmt.Errorf("gitea source: involving %q is listed twice", involving)
		}
		seen[involving] = true
	}
	for _, label := range c.Labels {
		if strings.TrimSpace(label) == "" {
			return fmt.Errorf("gitea source: labels must not contain a blank entry")
		}
		// The labels parameter is comma-joined with no escaping, so a label
		// carrying one would silently split into two nonexistent filters.
		if strings.Contains(label, ",") {
			return fmt.Errorf("gitea source: label %q contains a comma, which Gitea's search cannot express", label)
		}
	}
	return connector.ValidateInterval("gitea source", c.Interval)
}

// rejectSearchFields fails a notifications source carrying search filters.
// Ignoring them would be worse than failing: the inbox cannot be filtered
// server-side, so a node that looks filtered would quietly return everything.
func (c *Config) rejectSearchFields() error {
	set := make([]string, 0, 6)
	if strings.TrimSpace(c.Items) != "" {
		set = append(set, "items")
	}
	if strings.TrimSpace(c.State) != "" {
		set = append(set, "state")
	}
	if len(c.Involving) > 0 {
		set = append(set, "involving")
	}
	if strings.TrimSpace(c.Owner) != "" {
		set = append(set, "owner")
	}
	if len(c.Labels) > 0 {
		set = append(set, "labels")
	}
	if strings.TrimSpace(c.Text) != "" {
		set = append(set, "text")
	}
	if len(set) > 0 {
		return fmt.Errorf("gitea source: kind %q takes no search filters (remove %s); filter the inbox with a downstream node instead",
			KindNotifications, strings.Join(set, ", "))
	}
	return nil
}

func involvingOptions() string {
	return strings.Join([]string{
		InvolvingAssigned, InvolvingCreated, InvolvingMentioned,
		InvolvingReviewRequested, InvolvingReviewed,
	}, ", ")
}

// CredentialRef is the parsed credential ref. A ref naming another provider is
// rejected here rather than at fetch time: "github/octocat" on a Gitea source
// is a config mistake, and failing it at load says so, where failing it at
// fetch surfaces as an empty feed.
func (c *Config) CredentialRef() (credentials.Ref, error) {
	if strings.TrimSpace(c.Credential) == "" {
		return credentials.Ref{}, fmt.Errorf("gitea source: credential is required (e.g. %q)", Provider+"/gitea.example.com-octocat")
	}
	ref, err := credentials.ParseRef(c.Credential)
	if err != nil {
		return credentials.Ref{}, fmt.Errorf("gitea source: %w", err)
	}
	if ref.Provider != Provider {
		return credentials.Ref{}, fmt.Errorf("gitea source: credential %q is not a %s credential", c.Credential, Provider)
	}
	return ref, nil
}

// effectiveLimit resolves the configured limit against the per-kind default.
func (c *Config) effectiveLimit() int {
	if c.Limit > 0 {
		return c.Limit
	}
	return defaultLimit
}

// searchRequests is the set of API calls one search config asks for: one per
// involvement, or a single unfiltered request when none is named.
//
// Gitea ANDs its involvement parameters — created=true&review_requested=true
// returns only items that are both, which is empty in practice — so a union is
// several requests merged, not one request with several flags.
func (c *Config) searchRequests() []giteaclient.SearchRequest {
	base := giteaclient.SearchRequest{
		Type:   itemsToType(c.Items),
		State:  stateOrDefault(c.State),
		Owner:  strings.TrimSpace(c.Owner),
		Labels: c.Labels,
		Text:   strings.TrimSpace(c.Text),
		Limit:  c.effectiveLimit(),
	}
	if len(c.Involving) == 0 {
		return []giteaclient.SearchRequest{base}
	}
	requests := make([]giteaclient.SearchRequest, 0, len(c.Involving))
	for _, involving := range c.Involving {
		req := base
		req.Involvement = involvementParams[involving]
		requests = append(requests, req)
	}
	return requests
}

// itemsToType maps the config's item vocabulary onto Gitea's type parameter,
// where "both" is expressed by omitting it.
func itemsToType(items string) string {
	if items == "" || items == ItemsAll {
		return ""
	}
	return items
}

func stateOrDefault(state string) string {
	if state == "" {
		return StateOpen
	}
	return state
}
