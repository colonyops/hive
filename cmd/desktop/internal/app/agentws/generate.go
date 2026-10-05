package agentws

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/colonyops/hive/cmd/desktop/internal/app/mcpcatalog"
)

// ErrInvalidSkillSlug reports a skill slug that does not resolve to a direct
// child of the directory it belongs under: a traversal attempt (a "..",
// an absolute path), the tree root itself ("."), or an empty string. Slugs
// arrive from agent-workspace.yaml and from skill library directory names,
// and spec §8 hands the workspace manifest to an agent to write, so this is
// validated rather than trusted.
var ErrInvalidSkillSlug = errors.New("agentws: invalid skill slug")

// GenerateInput is Generate's whole world. Nothing in Generate reads the
// clock, the environment, or the filesystem beyond what this struct names —
// that purity is what makes TestGenerateIsDeterministic possible.
type GenerateInput struct {
	// Dir is the absolute workspace directory. Everything the generator
	// writes lives under it — there is no side tree anywhere else on the
	// machine.
	Dir string

	Workspace Workspace
	// Servers is the enabled MCP set, already resolved through the catalogue
	// with user-shadows-shipped applied.
	Servers map[string]mcpcatalog.Server
	// Skills is what the workspace's enabled packages select, already
	// resolved and rendered. Packages are not a concept here: the generator
	// installs the skills it is handed and never asks where they came from.
	Skills []RenderedSkill
}

// RenderedSkill is one skill body ready to install, keyed by the slug it
// installs under (e.g. "hive-mcp").
type RenderedSkill struct {
	Slug string
	Body string
}

// Result is what an open reports back to the UI.
type Result struct {
	// MissingMCPs are enabled ids that are no longer in the catalogue. The
	// workspace still opens and they are omitted from .mcp.json (spec §14).
	MissingMCPs []string
	// Problems are conditions that do not stop the open but that the user
	// must see — an .icloud placeholder standing in for an authored file,
	// say.
	Problems []string
}

// Generate writes everything a workspace needs to run an agent against: a
// CLAUDE.md copy of AGENTS.md, one generated MCP config per known agent, the
// enabled skill set at both tree locations, and an empty docs/. It reconciles
// rather than clears-and-rewrites: .codex/ and every skill Hive installed are
// Hive-owned, so a file no longer in the target set is removed and a file
// that is stays untouched unless its bytes actually differ (spec §4.3, §4.4).
// A skill directory Hive did not install is left as it is.
func Generate(in GenerateInput) (Result, error) {
	var res Result

	for _, id := range in.Workspace.MCPs {
		if _, ok := in.Servers[id]; !ok {
			res.MissingMCPs = append(res.MissingMCPs, id)
		}
	}
	sort.Strings(res.MissingMCPs)

	problem, err := generateClaudeMD(in.Dir)
	if err != nil {
		return Result{}, err
	}
	if problem != "" {
		res.Problems = append(res.Problems, problem)
	}

	if err := generateMCPFiles(in.Dir, in.Servers); err != nil {
		return Result{}, err
	}

	skillFiles, err := skillTree(in.Skills)
	if err != nil {
		return Result{}, err
	}

	if err := reconcileSkills(filepath.Join(in.Dir, ".claude", "skills"), skillFiles); err != nil {
		return Result{}, err
	}
	if err := reconcileSkills(filepath.Join(in.Dir, ".agents", "skills"), skillFiles); err != nil {
		return Result{}, err
	}

	if err := os.MkdirAll(filepath.Join(in.Dir, "docs"), 0o700); err != nil {
		return Result{}, fmt.Errorf("agentws: create docs dir: %w", err)
	}

	return res, nil
}

// generateClaudeMD keeps CLAUDE.md a byte copy of AGENTS.md. A missing
// AGENTS.md removes any previously-generated CLAUDE.md — unless AGENTS.md is
// merely evicted to an iCloud placeholder, in which case that would convert a
// recoverable state into data loss the user cannot see (spec §4.4): this
// reports a problem and leaves CLAUDE.md exactly as it is instead.
func generateClaudeMD(dir string) (problem string, err error) {
	agentsPath := filepath.Join(dir, "AGENTS.md")
	claudePath := filepath.Join(dir, "CLAUDE.md")

	data, err := os.ReadFile(agentsPath)
	if err == nil {
		return "", writeIfDifferent(claudePath, data)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("agentws: read AGENTS.md: %w", err)
	}

	evicted, err := fileExists(filepath.Join(dir, ".AGENTS.md.icloud"))
	if err != nil {
		return "", fmt.Errorf("agentws: stat .AGENTS.md.icloud: %w", err)
	}
	if evicted {
		return "AGENTS.md is evicted to iCloud and cannot be read; CLAUDE.md was left untouched", nil
	}

	if err := os.Remove(claudePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("agentws: remove stale CLAUDE.md: %w", err)
	}
	return "", nil
}

// generateMCPFiles writes every known MCP config format into the workspace,
// regardless of which agent the manifest names. That is deliberate: a command
// template points at whichever file its CLI reads (LaunchData.MCPConfig), so
// generating all of them is what lets an agent this build has never heard of
// still receive the workspace's declared servers.
func generateMCPFiles(dir string, servers map[string]mcpcatalog.Server) error {
	keys := make([]string, 0, len(agentWirings))
	for k := range agentWirings {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		wiring := agentWirings[k]
		content, err := wiring.Render(servers)
		if err != nil {
			return fmt.Errorf("agentws: render %s MCP config: %w", k, err)
		}

		// A file nested under a subdirectory (.codex/config.toml) owns that
		// whole subdirectory, which is reconciled like the skills trees; a
		// top-level file (.mcp.json) has nothing else in its "tree" to
		// remove.
		if treeDir := filepath.Dir(wiring.File); treeDir != "." {
			target := map[string][]byte{filepath.Base(wiring.File): content}
			if err := reconcileTree(filepath.Join(dir, treeDir), target); err != nil {
				return err
			}
			continue
		}
		if err := writeIfDifferent(filepath.Join(dir, wiring.File), content); err != nil {
			return err
		}
	}
	return nil
}

// skillTree maps each skill the workspace's packages selected to its
// SKILL.md body, keyed by slug. The generator reads no library and expands no
// pattern of its own: resolution happens before Generate is called, which is
// what keeps the generator pure (spec §4.4).
func skillTree(enabled []RenderedSkill) (target map[string][]byte, err error) {
	target = make(map[string][]byte, len(enabled))
	for _, rs := range enabled {
		if !validSlug(rs.Slug) {
			return nil, fmt.Errorf("%w: %q", ErrInvalidSkillSlug, rs.Slug)
		}
		target[rs.Slug] = []byte(rs.Body)
	}
	return target, nil
}

// reconcileSkills makes dir hold exactly the target skills among the ones
// Hive installed, and leaves every other skill directory alone: an agent in
// the workspace may write its own skills here
// (ADR agents-write-their-own-skills-into-a-workspace). installedListName
// records the slugs Hive wrote, so a skill dropped from the target set is
// removed and one Hive never wrote is not. A directory with no record yet
// removes nothing, because it cannot tell its own skills from an agent's.
// A target slug an agent also used is overwritten: the package wins a
// name both claim.
func reconcileSkills(dir string, target map[string][]byte) error {
	installed, err := readInstalledSkills(dir)
	if err != nil {
		return err
	}
	for _, slug := range installed {
		if _, ok := target[slug]; ok || !validSlug(slug) {
			continue
		}
		if err := os.RemoveAll(filepath.Join(dir, slug)); err != nil {
			return fmt.Errorf("agentws: remove skill %s: %w", slug, err)
		}
	}

	slugs := make([]string, 0, len(target))
	for slug := range target {
		slugs = append(slugs, slug)
	}
	sort.Strings(slugs)

	// The list is written before the skills so that a write failing partway
	// cannot leave a skill Hive wrote off the list, where it would pass for an
	// agent's and never be removed. Claiming a slug whose write then fails is
	// harmless: the next open rewrites or removes it.
	listPath := filepath.Join(dir, installedListName)
	if len(slugs) == 0 {
		if err := os.Remove(listPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("agentws: remove %s: %w", listPath, err)
		}
	} else if err := writeIfDifferent(listPath, []byte(strings.Join(slugs, "\n")+"\n")); err != nil {
		return err
	}

	for _, slug := range slugs {
		if err := reconcileTree(filepath.Join(dir, slug), map[string][]byte{skillFileName: target[slug]}); err != nil {
			return err
		}
	}
	return nil
}

func readInstalledSkills(dir string) ([]string, error) {
	data, err := os.ReadFile(filepath.Join(dir, installedListName))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("agentws: read %s: %w", installedListName, err)
	}
	var slugs []string
	for line := range strings.SplitSeq(string(data), "\n") {
		if line != "" {
			slugs = append(slugs, line)
		}
	}
	return slugs, nil
}

// validSlug reports whether slug resolves to a direct child of the directory
// it belongs under: non-empty, not "." or "..", and a single path component.
// A line break is refused because .hive-installed holds one slug per line.
func validSlug(slug string) bool {
	if slug == "" || slug == "." || strings.ContainsAny(slug, "\r\n") {
		return false
	}
	return filepath.Base(slug) == slug && filepath.IsLocal(slug)
}

// reconcileTree makes dir contain exactly the files in target — each key a
// path relative to dir. dir is a Hive-owned subtree (one installed skill or
// .codex/): a file already there but not in target is removed, a directory
// left empty by that removal is pruned, and a file in target is written only
// when its bytes differ from what is already on disk.
func reconcileTree(dir string, target map[string][]byte) error {
	if err := removeStale(dir, target); err != nil {
		return err
	}

	names := make([]string, 0, len(target))
	for rel := range target {
		names = append(names, rel)
	}
	sort.Strings(names)
	for _, rel := range names {
		if err := writeIfDifferent(filepath.Join(dir, rel), target[rel]); err != nil {
			return err
		}
	}
	return nil
}

type treeEntry struct {
	rel   string
	isDir bool
}

// removeStale deletes every file under dir not present in target, then
// prunes directories a removal left empty. A dir that does not exist yet is
// not an error — there is nothing to reconcile.
func removeStale(dir string, target map[string][]byte) error {
	entries, err := listTree(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}

	for _, e := range entries {
		if e.isDir {
			continue
		}
		if _, ok := target[e.rel]; ok {
			continue
		}
		if err := os.Remove(filepath.Join(dir, e.rel)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("agentws: remove %s: %w", e.rel, err)
		}
	}

	// Prune directories left empty by the removals above, deepest first, so
	// a parent whose only child was just emptied is checked after its child.
	sort.Slice(entries, func(i, j int) bool { return len(entries[i].rel) > len(entries[j].rel) })
	for _, e := range entries {
		if !e.isDir {
			continue
		}
		full := filepath.Join(dir, e.rel)
		children, err := os.ReadDir(full)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return fmt.Errorf("agentws: read %s: %w", e.rel, err)
		}
		if len(children) > 0 {
			continue
		}
		if err := os.Remove(full); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("agentws: remove empty dir %s: %w", e.rel, err)
		}
	}
	return nil
}

// listTree lists every entry under dir (not dir itself), relative to dir.
func listTree(dir string) ([]treeEntry, error) {
	var out []treeEntry
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == dir {
			return nil
		}
		rel, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			return relErr
		}
		out = append(out, treeEntry{rel: rel, isDir: d.IsDir()})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// writeIfDifferent writes data to path only when its current bytes differ,
// preserving mtime otherwise. This is what makes determinism observable in
// production (spec §4.4): reopening a workspace whose generated output has
// not changed touches no file the OS — or iCloud — would need to re-sync.
func writeIfDifferent(path string, data []byte) error {
	if existing, err := os.ReadFile(path); err == nil && bytes.Equal(existing, data) {
		return nil
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("agentws: read %s: %w", path, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("agentws: create %s: %w", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("agentws: write %s: %w", path, err)
	}
	return nil
}

func fileExists(path string) (bool, error) {
	if _, err := os.Lstat(path); err == nil {
		return true, nil
	} else if errors.Is(err, os.ErrNotExist) {
		return false, nil
	} else {
		return false, err
	}
}
