// Package migrate is the one-time converter that turns a legacy desktop
// profiles.yaml (top-level sources: + profiles:/feeds:/filters:) into
// per-profile flows/*.yaml graphs. It is run via the desktop binary's
// --migrate-profiles flag and nothing in the running app depends on it — so
// it carries its own private reader of the old config shape, letting the
// legacy feed package's config schema be deleted independently.
package migrate

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/colonyops/hive/internal/desktop/pipeline/flow"
)

// Node type discriminators, matching flow's registry keys.
const (
	typeGithubSource = "github-source"
	typeGithubFilter = "github-filter"
	typeFeed         = "feed"
)

// Layout columns: sources on the left, filters in the middle, feeds on the
// right; rows stack within a column.
const (
	colGap = 300
	rowGap = 96
	colPad = 48
)

// --- self-contained legacy reader (a private copy of the old profiles.yaml
// shape, decoded laxly so an old field the app no longer knows about is
// ignored rather than fatal) ---

type legacySource struct {
	ID    string `yaml:"id"`
	Kind  string `yaml:"kind"`
	Query string `yaml:"query"`
	Limit int    `yaml:"limit"`
}

type legacyFilter struct {
	Repos          []string `yaml:"repos"`
	ExcludeRepos   []string `yaml:"exclude_repos"`
	Authors        []string `yaml:"authors"`
	ExcludeAuthors []string `yaml:"exclude_authors"`
	Labels         []string `yaml:"labels"`
	ExcludeLabels  []string `yaml:"exclude_labels"`
	Types          []string `yaml:"types"`
	Reasons        []string `yaml:"reasons"`
}

func (f legacyFilter) isZero() bool {
	return len(f.Repos) == 0 && len(f.ExcludeRepos) == 0 &&
		len(f.Authors) == 0 && len(f.ExcludeAuthors) == 0 &&
		len(f.Labels) == 0 && len(f.ExcludeLabels) == 0 &&
		len(f.Types) == 0 && len(f.Reasons) == 0
}

type legacyFeed struct {
	ID      string       `yaml:"id"`
	Name    string       `yaml:"name"`
	Sources []string     `yaml:"sources"`
	Filters legacyFilter `yaml:"filters"`
}

type legacyProfile struct {
	ID    string       `yaml:"id"`
	Name  string       `yaml:"name"`
	Feeds []legacyFeed `yaml:"feeds"`
}

type legacyConfig struct {
	Sources  []legacySource  `yaml:"sources"`
	Profiles []legacyProfile `yaml:"profiles"`
}

// Options controls a Convert run.
type Options struct {
	// Write commits the flows to disk; false is a dry-run that renders the
	// YAML into the Report and touches nothing.
	Write bool
	// Force overwrites an existing flows/<id>.yaml; without it, an existing
	// file is skipped so a re-run never clobbers hand edits.
	Force bool
}

// FlowResult is the per-profile outcome of a conversion.
type FlowResult struct {
	ProfileID string
	Path      string
	YAML      string
	Written   bool
	Skipped   bool
	Reason    string
	Warnings  []string
}

// Report is the whole conversion outcome, suitable for printing.
type Report struct {
	ProfilesPath string
	FlowsDir     string
	BackupPath   string
	DryRun       bool
	Flows        []FlowResult
}

// Format renders the report as human-readable CLI output.
func (r Report) Format() string {
	var b strings.Builder
	mode := "DRY RUN — no files written (pass --migrate-profiles=write to apply)"
	if !r.DryRun {
		mode = "WRITE"
	}
	fmt.Fprintf(&b, "Profiles → flows migration [%s]\n", mode)
	fmt.Fprintf(&b, "  source: %s\n", r.ProfilesPath)
	fmt.Fprintf(&b, "  flows:  %s\n", r.FlowsDir)
	if r.BackupPath != "" {
		fmt.Fprintf(&b, "  backup: %s\n", r.BackupPath)
	}
	b.WriteString("\n")
	if len(r.Flows) == 0 {
		b.WriteString("No profiles found — nothing to convert.\n")
		return b.String()
	}
	for _, fr := range r.Flows {
		status := "converted"
		switch {
		case fr.Skipped:
			status = "SKIPPED: " + fr.Reason
		case fr.Written:
			status = "written " + fr.Path
		}
		fmt.Fprintf(&b, "▸ %s — %s\n", fr.ProfileID, status)
		for _, w := range fr.Warnings {
			fmt.Fprintf(&b, "    ⚠ %s\n", w)
		}
		if r.DryRun && fr.YAML != "" {
			for _, line := range strings.Split(strings.TrimRight(fr.YAML, "\n"), "\n") {
				fmt.Fprintf(&b, "    | %s\n", line)
			}
		}
	}
	return b.String()
}

// Convert reads profilesPath and produces one flow per profile under
// flowsDir. In dry-run mode (opts.Write == false) nothing is written; the
// rendered YAML rides the Report so a caller can print it for review.
func Convert(profilesPath, flowsDir string, opts Options) (Report, error) {
	data, err := os.ReadFile(profilesPath)
	if err != nil {
		return Report{}, fmt.Errorf("read profiles config %q: %w", profilesPath, err)
	}
	var cfg legacyConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Report{}, fmt.Errorf("parse profiles config: %w", err)
	}

	sourcesByID := make(map[string]legacySource, len(cfg.Sources))
	for _, s := range cfg.Sources {
		sourcesByID[s.ID] = s
	}

	report := Report{ProfilesPath: profilesPath, FlowsDir: flowsDir, DryRun: !opts.Write}

	if opts.Write {
		if err := os.MkdirAll(flowsDir, 0o700); err != nil {
			return report, fmt.Errorf("create flows dir %q: %w", flowsDir, err)
		}
		backup, err := backupOnce(profilesPath)
		if err != nil {
			return report, err
		}
		report.BackupPath = backup
	}

	for _, p := range cfg.Profiles {
		report.Flows = append(report.Flows, convertProfile(p, sourcesByID, flowsDir, opts))
	}
	return report, nil
}

// convertProfile builds, renders, validates, and (in write mode) persists one
// profile's flow.
func convertProfile(p legacyProfile, sourcesByID map[string]legacySource, flowsDir string, opts Options) FlowResult {
	f, layout, warnings := buildFlow(p, sourcesByID)
	res := FlowResult{ProfileID: p.ID, Warnings: warnings}

	yamlBytes, err := flow.MarshalFlow(f)
	if err != nil {
		res.Skipped, res.Reason = true, fmt.Sprintf("render flow: %v", err)
		return res
	}
	res.YAML = string(yamlBytes)

	if _, verr := flow.Validate(f, flow.MapRefs{}); verr != nil {
		res.Skipped, res.Reason = true, fmt.Sprintf("produced flow is invalid: %v", verr)
		return res
	}

	res.Path = filepath.Join(flowsDir, f.ID+".yaml")
	if !opts.Write {
		return res
	}

	if _, err := os.Stat(res.Path); err == nil && !opts.Force {
		res.Skipped, res.Reason = true, "flow file already exists (use --force to overwrite)"
		return res
	}

	if err := flow.SaveFlow(res.Path, f); err != nil {
		res.Skipped, res.Reason = true, fmt.Sprintf("write flow: %v", err)
		return res
	}
	uiPath := filepath.Join(flowsDir, f.ID+".ui.yaml")
	if err := flow.SaveUI(uiPath, layout); err != nil {
		res.Warnings = append(res.Warnings, fmt.Sprintf("wrote flow but not its layout: %v", err))
	}
	res.Written = true
	return res
}

// buildFlow turns one legacy profile into a flow graph plus a columnar
// layout, collecting soft warnings for anything it had to route around.
func buildFlow(p legacyProfile, sourcesByID map[string]legacySource) (flow.Flow, flow.Layout, []string) {
	b := &flowBuilder{
		usedIDs:      map[string]bool{},
		sourceNodeID: map[string]string{},
		colRows:      map[int]int{},
		layout:       flow.Layout{Nodes: map[string]flow.NodePosition{}},
		sourcesByID:  sourcesByID,
	}

	for _, feed := range p.Feeds {
		b.addFeed(feed)
	}

	name := p.Name
	if name == "" {
		name = p.ID
	}
	f := flow.Flow{ID: slugify(p.ID), Name: name, Enabled: true, Nodes: b.nodes, Wires: b.wires}
	return f, b.layout, b.warnings
}

type flowBuilder struct {
	sourcesByID  map[string]legacySource
	nodes        []flow.Node
	wires        []flow.Wire
	warnings     []string
	usedIDs      map[string]bool
	sourceNodeID map[string]string // original source id -> node id (dedup)
	colRows      map[int]int       // column -> next free row
	layout       flow.Layout
}

func (b *flowBuilder) addFeed(fd legacyFeed) {
	var filterID string
	if !fd.Filters.isZero() {
		filterID = b.addNode(slugify(fd.ID+"-filter"), typeGithubFilter, "", filterConfig(fd.Filters), 1)
	}

	feedName := fd.Name
	if feedName == "" {
		feedName = fd.ID
	}
	feedID := b.addNode(slugify(fd.ID), typeFeed, feedName, &flow.FeedConfig{}, 2)

	// A filter sits between the sources and the feed; without one the sources
	// wire straight into the feed.
	target := feedID
	if filterID != "" {
		target = filterID
		b.wires = append(b.wires, flow.Wire{From: filterID, Out: 0, To: feedID})
	}

	for _, srcID := range dedup(fd.Sources) {
		nodeID, ok := b.ensureSource(srcID)
		if !ok {
			continue
		}
		b.wires = append(b.wires, flow.Wire{From: nodeID, To: target})
	}
}

// ensureSource creates (once) the github-source node for a legacy source id,
// returning its node id.
func (b *flowBuilder) ensureSource(origID string) (string, bool) {
	if id, ok := b.sourceNodeID[origID]; ok {
		return id, true
	}
	src, ok := b.sourcesByID[origID]
	if !ok {
		b.warnings = append(b.warnings, fmt.Sprintf("feed references unknown source %q — skipped", origID))
		return "", false
	}
	cfg := &flow.GithubSourceConfig{Kind: src.Kind, Query: src.Query, Limit: src.Limit}
	id := b.addNode(slugify(src.ID), typeGithubSource, src.ID, cfg, 0)
	b.sourceNodeID[origID] = id
	return id, true
}

// addNode appends a node with a flow-unique id and records its layout slot in
// the given column.
func (b *flowBuilder) addNode(baseID, nodeType, name string, cfg flow.NodeConfig, col int) string {
	id := b.uniqueID(baseID)
	// A source node's name is redundant with its type label; only carry a
	// display name for feeds (their human title in the sidebar).
	node := flow.Node{ID: id, Type: nodeType, Config: cfg}
	if nodeType == typeFeed {
		node.Name = name
	}
	b.nodes = append(b.nodes, node)
	row := b.colRows[col]
	b.colRows[col] = row + 1
	b.layout.Nodes[id] = flow.NodePosition{X: colPad + col*colGap, Y: colPad + row*rowGap}
	return id
}

func (b *flowBuilder) uniqueID(base string) string {
	if base == "" {
		base = "node"
	}
	id := base
	for i := 2; b.usedIDs[id]; i++ {
		id = fmt.Sprintf("%s-%d", base, i)
	}
	b.usedIDs[id] = true
	return id
}

func filterConfig(f legacyFilter) *flow.GithubFilterConfig {
	return &flow.GithubFilterConfig{
		Repos:          f.Repos,
		ExcludeRepos:   f.ExcludeRepos,
		Authors:        f.Authors,
		ExcludeAuthors: f.ExcludeAuthors,
		Labels:         f.Labels,
		ExcludeLabels:  f.ExcludeLabels,
		Types:          f.Types,
		Reasons:        f.Reasons,
	}
}

// backupOnce copies profilesPath to profilesPath+".bak", leaving an existing
// backup untouched so a second run never clobbers the pristine original.
func backupOnce(profilesPath string) (string, error) {
	backup := profilesPath + ".bak"
	if _, err := os.Stat(backup); err == nil {
		return backup, nil // keep the first backup taken
	}
	data, err := os.ReadFile(profilesPath)
	if err != nil {
		return "", fmt.Errorf("read profiles config for backup: %w", err)
	}
	if err := os.WriteFile(backup, data, 0o600); err != nil {
		return "", fmt.Errorf("write backup %q: %w", backup, err)
	}
	return backup, nil
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// slugify coerces an arbitrary id/name into the flow schema's slug rule
// (`^[a-z0-9][a-z0-9-]*$`, max 64 chars): lowercase, non-alphanumeric runs
// become single hyphens, leading/trailing hyphens are trimmed.
func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = nonSlug.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > 64 {
		s = strings.Trim(s[:64], "-")
	}
	if s == "" {
		return "flow"
	}
	return s
}

func dedup(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, v := range in {
		if seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}
