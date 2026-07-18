package feed

import (
	"bytes"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// SourceDef is a top-level data source: one GitHub API acquisition, shared by
// any number of feeds. Sources are the only part of the config that costs API
// requests; feeds are free client-side views over them.
type SourceDef struct {
	ID    string `json:"id"              yaml:"id"`
	Kind  string `json:"kind"            yaml:"kind"` // "search" | "notifications"
	Query string `json:"query,omitempty" yaml:"query,omitempty"`
	// Limit bounds items per fetch. Search: default 50, max 100 (API page
	// cap). Notifications: default 50, max 50 (API page cap).
	Limit int `json:"limit,omitempty" yaml:"limit,omitempty"`
}

const (
	defaultSourceLimit    = 50
	maxSearchLimit        = 100
	maxNotificationsLimit = 50
)

// effectiveLimit resolves the configured limit against per-kind defaults and
// API caps; validation rejects out-of-range values, so this only fills the
// default.
func (s SourceDef) effectiveLimit() int {
	if s.Limit > 0 {
		return s.Limit
	}
	return defaultSourceLimit
}

// FeedDef is a feed inside a profile: a client-side filtered view over one or
// more top-level sources. Feeds never cost API requests of their own.
type FeedDef struct {
	ID      string    `json:"id"      yaml:"id"`
	Name    string    `json:"name"    yaml:"name"`
	Sources []string  `json:"sources" yaml:"sources"`
	Filters FilterDef `json:"filters" yaml:"filters,omitempty"`
}

// ProfileDef is a profile ("workspace") definition.
type ProfileDef struct {
	ID    string    `json:"id"    yaml:"id"`
	Name  string    `json:"name"  yaml:"name"`
	Feeds []FeedDef `json:"feeds" yaml:"feeds"`
}

// configFile is the on-disk shape of the profiles config.
type configFile struct {
	Sources  []SourceDef  `yaml:"sources"`
	Profiles []ProfileDef `yaml:"profiles"`
}

// maxSearchSources caps sources of kind "search". Each distinct search source
// is one request per poll cycle (about once a minute) against GitHub's search
// bucket of 30 requests/min; 25 leaves headroom for manual refreshes.
// Notifications sources are uncapped: they poll the core bucket (5000/hr) with
// conditional requests whose 304 responses are free, and identical sources
// deduplicate to one request anyway.
const maxSearchSources = 25

// ConfigInfo describes the profiles config file for the frontend: where it
// lives, what it contains, and whether it currently parses.
type ConfigInfo struct {
	Path   string `json:"path"`
	Exists bool   `json:"exists"`
	// YAML is the raw file content, or the example template when the file
	// does not exist yet, so the UI and the copy-prompt always have a
	// concrete config to show.
	YAML  string `json:"yaml"`
	Valid bool   `json:"valid"`
	Error string `json:"error"`
}

// legacyFeedFieldMarkers identify KnownFields errors caused by the pre-source
// schema, where kind/query/repos/exclude_repos lived on feeds. None of these
// fields exist on FeedDef anymore, so the substrings only fire on old configs.
var legacyFeedFieldMarkers = []string{
	"field kind not found",
	"field query not found",
	"field repos not found",
	"field exclude_repos not found",
}

// parseConfig strictly decodes and validates the profiles YAML. Unknown fields
// are errors: a typo like "filter:" for "filters:" must fail loudly, not
// silently configure nothing.
func parseConfig(data []byte) (configFile, error) {
	var file configFile
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&file); err != nil {
		if err.Error() == "EOF" { // empty file: valid, no sources or profiles
			return configFile{}, nil
		}
		for _, marker := range legacyFeedFieldMarkers {
			if strings.Contains(err.Error(), marker) {
				return configFile{}, fmt.Errorf("parse profiles config: %w (hint: feeds now reference top-level sources: entries — kind/query moved to sources, and repos/exclude_repos moved under the feed's filters: block)", err)
			}
		}
		return configFile{}, fmt.Errorf("parse profiles config: %w", err)
	}
	if err := validateConfig(file); err != nil {
		return configFile{}, err
	}
	return file, nil
}

func validateConfig(file configFile) error {
	sourceIDs := make(map[string]bool, len(file.Sources))
	searchSources := 0
	for _, def := range file.Sources {
		if err := validateSource(def); err != nil {
			return err
		}
		if sourceIDs[def.ID] {
			return fmt.Errorf("source id %q is defined twice", def.ID)
		}
		sourceIDs[def.ID] = true
		if def.Kind == "search" {
			searchSources++
		}
	}
	if searchSources > maxSearchSources {
		return fmt.Errorf("%d search sources defined; the limit is %d — each search source is one request per poll against GitHub's search rate limit (30 requests/min), and %d leaves headroom for manual refreshes (notifications sources are not capped)", searchSources, maxSearchSources, maxSearchSources)
	}

	profileIDs := make(map[string]bool)
	for _, def := range file.Profiles {
		if def.ID == "" {
			return fmt.Errorf("profile %q: id is required", def.Name)
		}
		if def.Name == "" {
			return fmt.Errorf("profile %q: name is required", def.ID)
		}
		if profileIDs[def.ID] {
			return fmt.Errorf("profile id %q is defined twice", def.ID)
		}
		profileIDs[def.ID] = true

		feedIDs := make(map[string]bool)
		for _, feedDef := range def.Feeds {
			if err := validateFeed(def.ID, feedDef, sourceIDs); err != nil {
				return err
			}
			if feedIDs[feedDef.ID] {
				return fmt.Errorf("profile %q: feed id %q is defined twice", def.ID, feedDef.ID)
			}
			feedIDs[feedDef.ID] = true
		}
	}
	return nil
}

func validateSource(def SourceDef) error {
	if def.ID == "" {
		return fmt.Errorf("source: id is required")
	}
	switch def.Kind {
	case "search":
		if strings.TrimSpace(def.Query) == "" {
			return fmt.Errorf("source %q: kind \"search\" requires a query", def.ID)
		}
		if def.Limit > maxSearchLimit {
			return fmt.Errorf("source %q: limit %d exceeds the search API page cap of %d", def.ID, def.Limit, maxSearchLimit)
		}
	case "notifications":
		if def.Query != "" {
			return fmt.Errorf("source %q: kind \"notifications\" takes no query", def.ID)
		}
		if def.Limit > maxNotificationsLimit {
			return fmt.Errorf("source %q: limit %d exceeds the notifications API page cap of %d", def.ID, def.Limit, maxNotificationsLimit)
		}
	default:
		return fmt.Errorf("source %q: unknown kind %q (want \"search\" or \"notifications\")", def.ID, def.Kind)
	}
	if def.Limit < 0 {
		return fmt.Errorf("source %q: limit must not be negative", def.ID)
	}
	return nil
}

func validateFeed(profileID string, def FeedDef, sourceIDs map[string]bool) error {
	if def.ID == "" {
		return fmt.Errorf("profile %q: feed %q: id is required", profileID, def.Name)
	}
	if def.Name == "" {
		return fmt.Errorf("profile %q: feed %q: name is required", profileID, def.ID)
	}
	if len(def.Sources) == 0 {
		return fmt.Errorf("profile %q: feed %q: at least one source is required", profileID, def.ID)
	}
	for _, sourceID := range def.Sources {
		if !sourceIDs[sourceID] {
			return fmt.Errorf("profile %q: feed %q: unknown source %q (not defined under top-level sources:)", profileID, def.ID, sourceID)
		}
	}
	if err := validateFilters(def.Filters); err != nil {
		return fmt.Errorf("profile %q: feed %q: %w", profileID, def.ID, err)
	}
	return nil
}

const configHeader = `# Hive Desktop feeds — sources, workspaces, and feeds, as code.
# Edited by hand or by the app; changes apply live, no restart needed.
# Sources are the GitHub API cost (deduplicated, polled about once a
# minute); feeds are free client-side filtered views over them.
`

// appendProfileToConfig returns the config document with the profile appended
// to the top-level profiles sequence, preserving comments and formatting.
func appendProfileToConfig(data []byte, def ProfileDef) ([]byte, error) {
	return appendTopLevelEntry(data, "profiles", def)
}

// appendSourceToConfig returns the config document with the source appended
// to the top-level sources sequence, preserving comments and formatting.
func appendSourceToConfig(data []byte, def SourceDef) ([]byte, error) {
	return appendTopLevelEntry(data, "sources", def)
}

// appendTopLevelEntry appends value to the top-level sequence under key,
// creating the key when missing. It edits the YAML node tree instead of
// re-marshalling the whole document so comments and formatting in a
// hand-managed file survive an app-side edit.
func appendTopLevelEntry(data []byte, key string, value any) ([]byte, error) {
	var valueNode yaml.Node
	if err := valueNode.Encode(value); err != nil {
		return nil, fmt.Errorf("encode %s entry: %w", key, err)
	}

	if len(bytes.TrimSpace(data)) == 0 {
		doc := &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{
			Kind: yaml.MappingNode,
			Content: []*yaml.Node{
				{Kind: yaml.ScalarNode, Value: key},
				{Kind: yaml.SequenceNode, Content: []*yaml.Node{&valueNode}},
			},
		}}}
		out, err := encodeConfigNode(doc)
		if err != nil {
			return nil, err
		}
		return append([]byte(configHeader), out...), nil
	}

	doc, root, err := parseConfigNode(data)
	if err != nil {
		return nil, err
	}
	if seq := mappingValue(root, key); seq != nil {
		if seq.Kind == yaml.SequenceNode {
			seq.Content = append(seq.Content, &valueNode)
		} else { // key present with a null value
			*seq = yaml.Node{Kind: yaml.SequenceNode, Content: []*yaml.Node{&valueNode}}
		}
		return encodeConfigNode(doc)
	}

	root.Content = append(root.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: key},
		&yaml.Node{Kind: yaml.SequenceNode, Content: []*yaml.Node{&valueNode}},
	)
	return encodeConfigNode(doc)
}

// appendFeedToConfig appends the feed to the profile's feeds sequence,
// preserving comments and formatting elsewhere in the document.
func appendFeedToConfig(data []byte, profileID string, def FeedDef) ([]byte, error) {
	doc, profileNode, err := findProfileNode(data, profileID)
	if err != nil {
		return nil, err
	}

	var feedNode yaml.Node
	if err := feedNode.Encode(def); err != nil {
		return nil, fmt.Errorf("encode feed: %w", err)
	}

	if seq := mappingValue(profileNode, "feeds"); seq != nil {
		if seq.Kind == yaml.SequenceNode {
			seq.Content = append(seq.Content, &feedNode)
		} else { // "feeds:" with a null value
			*seq = yaml.Node{Kind: yaml.SequenceNode, Content: []*yaml.Node{&feedNode}}
		}
		return encodeConfigNode(doc)
	}

	profileNode.Content = append(profileNode.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: "feeds"},
		&yaml.Node{Kind: yaml.SequenceNode, Content: []*yaml.Node{&feedNode}},
	)
	return encodeConfigNode(doc)
}

// updateFeedInConfig replaces the feed's mapping node in place. Comments
// elsewhere in the document survive; comments attached to the replaced feed
// node itself are lost — the node is re-encoded wholesale.
func updateFeedInConfig(data []byte, profileID, feedID string, def FeedDef) ([]byte, error) {
	doc, profileNode, err := findProfileNode(data, profileID)
	if err != nil {
		return nil, err
	}
	feeds := mappingValue(profileNode, "feeds")
	if feeds == nil || feeds.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("profile %q has no feeds", profileID)
	}
	feedNode := sequenceEntryByID(feeds, feedID)
	if feedNode == nil {
		return nil, fmt.Errorf("feed %q not found in profile %q", feedID, profileID)
	}

	var replacement yaml.Node
	if err := replacement.Encode(def); err != nil {
		return nil, fmt.Errorf("encode feed: %w", err)
	}
	*feedNode = replacement
	return encodeConfigNode(doc)
}

// findProfileNode parses the document and locates the profile's mapping node.
func findProfileNode(data []byte, profileID string) (doc, profile *yaml.Node, err error) {
	doc, root, err := parseConfigNode(data)
	if err != nil {
		return nil, nil, err
	}
	profiles := mappingValue(root, "profiles")
	if profiles == nil || profiles.Kind != yaml.SequenceNode {
		return nil, nil, fmt.Errorf("profile %q not found: config has no profiles", profileID)
	}
	profileNode := sequenceEntryByID(profiles, profileID)
	if profileNode == nil {
		return nil, nil, fmt.Errorf("profile %q not found", profileID)
	}
	return doc, profileNode, nil
}

// parseConfigNode parses the raw document into its node tree and returns the
// document node plus the root mapping.
func parseConfigNode(data []byte) (doc, root *yaml.Node, err error) {
	doc = &yaml.Node{}
	if err := yaml.Unmarshal(data, doc); err != nil {
		return nil, nil, fmt.Errorf("parse profiles config: %w", err)
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, nil, fmt.Errorf("profiles config is not a mapping")
	}
	return doc, doc.Content[0], nil
}

// mappingValue returns the value node for key in a mapping node, or nil.
func mappingValue(mapping *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}

// sequenceEntryByID returns the sequence's mapping entry whose "id" scalar
// equals id, or nil.
func sequenceEntryByID(seq *yaml.Node, id string) *yaml.Node {
	for _, entry := range seq.Content {
		if entry.Kind != yaml.MappingNode {
			continue
		}
		if idNode := mappingValue(entry, "id"); idNode != nil && idNode.Value == id {
			return entry
		}
	}
	return nil
}

func encodeConfigNode(doc *yaml.Node) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return nil, fmt.Errorf("encode profiles config: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("encode profiles config: %w", err)
	}
	return buf.Bytes(), nil
}

// ExampleConfig is what the UI and copy-prompt show before the file exists:
// a concrete, commented starting point rather than an empty pane.
func ExampleConfig() string {
	return configHeader + `sources:
  - id: my-prs
    kind: search
    query: "is:open is:pr author:@me archived:false"
  - id: assigned
    kind: search
    query: "is:open assignee:@me archived:false"
  - id: inbox
    kind: notifications
profiles:
  - id: triage
    name: Triage
    feeds:
      - id: my-open-prs
        name: My open PRs
        sources: [my-prs]
      - id: notifications-inbox
        name: Notifications inbox
        sources: [inbox]
        # Optional client-side filters (no API cost). Groups AND together,
        # values within a group OR, excludes win over includes:
        filters:
          exclude_authors: ["dependabot[bot]"]
          # repos: ["colonyops/*"]
          # reasons: [mention, review_requested]
      - id: assigned-across-org
        name: Assigned across org
        sources: [assigned]
`
}
