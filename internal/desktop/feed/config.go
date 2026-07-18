package feed

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"gopkg.in/yaml.v3"
)

// FeedDef is a feed definition inside a profile. The vocabulary (kind +
// query) deliberately mirrors the saved-view schema proposed for TUI sources
// (issue #370) so the two can converge on one definition later.
type FeedDef struct {
	ID    string `json:"id"              yaml:"id"`
	Name  string `json:"name"            yaml:"name"`
	Kind  string `json:"kind"            yaml:"kind"` // "search" | "notifications"
	Query string `json:"query,omitempty" yaml:"query,omitempty"`
	// Repos/ExcludeRepos are doublestar globs matched against "owner/repo"
	// (e.g. "colonyops/*"). They filter fetched results client-side, so
	// they never add API requests — one request per feed per poll holds
	// regardless of how the filters are written.
	Repos        []string `json:"repos,omitempty"         yaml:"repos,omitempty"`
	ExcludeRepos []string `json:"exclude_repos,omitempty" yaml:"exclude_repos,omitempty"`
}

// ProfileDef is a profile ("workspace") definition.
type ProfileDef struct {
	ID    string    `json:"id"    yaml:"id"`
	Name  string    `json:"name"  yaml:"name"`
	Feeds []FeedDef `json:"feeds" yaml:"feeds"`
}

// configFile is the on-disk shape of the profiles config.
type configFile struct {
	Profiles []ProfileDef `yaml:"profiles"`
}

// maxFeeds caps the total feed count across all profiles. Every feed costs
// one GitHub API request per poll cycle (once a minute), and authenticated
// search allows 30 requests/min — a config past that ceiling would sit in
// permanent rate-limit backoff, so reject it outright with an explanation.
const maxFeeds = 30

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

// parseConfig strictly decodes and validates profiles YAML. Unknown fields
// are errors: a typo like "filter:" for "repos:" must fail loudly, not
// silently configure nothing.
func parseConfig(data []byte) ([]ProfileDef, error) {
	var file configFile
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&file); err != nil {
		if err.Error() == "EOF" { // empty file: valid, no profiles
			return nil, nil
		}
		return nil, fmt.Errorf("parse profiles config: %w", err)
	}
	if err := validateProfiles(file.Profiles); err != nil {
		return nil, err
	}
	return file.Profiles, nil
}

func validateProfiles(defs []ProfileDef) error {
	profileIDs := make(map[string]bool)
	feeds := 0
	for _, def := range defs {
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
			feeds++
			if err := validateFeed(def.ID, feedDef); err != nil {
				return err
			}
			if feedIDs[feedDef.ID] {
				return fmt.Errorf("profile %q: feed id %q is defined twice", def.ID, feedDef.ID)
			}
			feedIDs[feedDef.ID] = true
		}
	}
	if feeds > maxFeeds {
		return fmt.Errorf("%d feeds defined; the limit is %d — each feed is one GitHub API request per poll, and more than %d would exceed the search rate limit (30 requests/min)", feeds, maxFeeds, maxFeeds)
	}
	return nil
}

func validateFeed(profileID string, def FeedDef) error {
	if def.ID == "" {
		return fmt.Errorf("profile %q: feed %q: id is required", profileID, def.Name)
	}
	if def.Name == "" {
		return fmt.Errorf("profile %q: feed %q: name is required", profileID, def.ID)
	}
	switch def.Kind {
	case "search":
		if strings.TrimSpace(def.Query) == "" {
			return fmt.Errorf("profile %q: feed %q: kind \"search\" requires a query", profileID, def.ID)
		}
	case "notifications":
		if def.Query != "" {
			return fmt.Errorf("profile %q: feed %q: kind \"notifications\" takes no query", profileID, def.ID)
		}
	default:
		return fmt.Errorf("profile %q: feed %q: unknown kind %q (want \"search\" or \"notifications\")", profileID, def.ID, def.Kind)
	}
	for _, pattern := range append(append([]string{}, def.Repos...), def.ExcludeRepos...) {
		if !doublestar.ValidatePattern(pattern) {
			return fmt.Errorf("profile %q: feed %q: invalid repo glob %q", profileID, def.ID, pattern)
		}
	}
	return nil
}

// matchesRepo reports whether an item's repo ("owner/name") passes the
// feed's include/exclude globs. Excludes win; an empty include list means
// include everything.
func (d FeedDef) matchesRepo(repo string) bool {
	if matchAnyGlob(d.ExcludeRepos, repo) {
		return false
	}
	if len(d.Repos) == 0 {
		return true
	}
	return matchAnyGlob(d.Repos, repo)
}

func matchAnyGlob(patterns []string, repo string) bool {
	for _, pattern := range patterns {
		// Match errors only on malformed patterns, which validation
		// rejected at load; a malformed pattern here just doesn't match.
		if ok, err := doublestar.Match(pattern, repo); err == nil && ok {
			return true
		}
	}
	return false
}

const configHeader = `# Hive Desktop profiles — workspaces and their feeds, as code.
# Edited by hand or by the app; changes apply live, no restart needed.
# Each feed is one GitHub API request per poll (about once a minute).
`

// appendProfileToConfig returns the config document with the profile
// appended. It edits the YAML node tree instead of re-marshalling the whole
// document so comments and formatting in a hand-managed file survive an
// app-side profile creation.
func appendProfileToConfig(data []byte, def ProfileDef) ([]byte, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		out, err := yaml.Marshal(configFile{Profiles: []ProfileDef{def}})
		if err != nil {
			return nil, fmt.Errorf("encode profiles config: %w", err)
		}
		return append([]byte(configHeader), out...), nil
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse profiles config: %w", err)
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("profiles config is not a mapping")
	}

	var profileNode yaml.Node
	if err := profileNode.Encode(def); err != nil {
		return nil, fmt.Errorf("encode profile: %w", err)
	}

	root := doc.Content[0]
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value != "profiles" {
			continue
		}
		seq := root.Content[i+1]
		if seq.Kind == yaml.SequenceNode {
			seq.Content = append(seq.Content, &profileNode)
		} else { // "profiles:" with a null value
			*seq = yaml.Node{Kind: yaml.SequenceNode, Content: []*yaml.Node{&profileNode}}
		}
		return encodeConfigNode(&doc)
	}

	root.Content = append(root.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: "profiles"},
		&yaml.Node{Kind: yaml.SequenceNode, Content: []*yaml.Node{&profileNode}},
	)
	return encodeConfigNode(&doc)
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
	return configHeader + `profiles:
  - id: triage
    name: Triage
    feeds:
      - id: my-open-prs
        name: My open PRs
        kind: search
        query: "is:open is:pr author:@me archived:false"
      - id: notifications-inbox
        name: Notifications inbox
        kind: notifications
        # Optional owner/repo globs, matched client-side (no extra API cost):
        # repos: ["colonyops/*"]
        # exclude_repos: ["colonyops/noisy-repo"]
`
}
