package agentws

import (
	"fmt"
	"path"
	"sort"
)

// SkillLibrary is skills.yml: the packages a workspace can enable. A package
// is a named set of glob patterns over skill names, never a copy of the
// skills themselves — which is what lets one skill belong to several packages
// and what makes adding a skill to every workspace that wants it a one-line
// edit here.
type SkillLibrary struct {
	Version  int                     `yaml:"version"`
	Packages map[string]SkillPackage `yaml:"packages,omitempty"`
}

// SkillPackage selects skills by name. Include patterns admit, Exclude
// patterns then carve out, and a pattern with no wildcard is simply an exact
// name — so naming one skill needs no syntax of its own.
type SkillPackage struct {
	Title       string   `yaml:"title,omitempty"`
	Description string   `yaml:"description,omitempty"`
	Include     []string `yaml:"include,omitempty"`
	Exclude     []string `yaml:"exclude,omitempty"`
}

// SkillName is one entry in the name-space packages glob over: a skill this
// build ships (rendered per install) or one authored in the shared skills
// directory. Packages match names, not sources, so a single pattern can
// select both.
type SkillName struct {
	Slug    string
	Shipped bool
}

// Validate checks every package's id and patterns.
func (l SkillLibrary) Validate() error {
	for id, pkg := range l.Packages {
		if id == "" {
			return fmt.Errorf("skills.yml: package name must not be empty")
		}
		if !validSlug(id) {
			return fmt.Errorf("skills.yml: package name %q must be a single path-safe segment", id)
		}
		if err := pkg.Validate(); err != nil {
			return fmt.Errorf("skills.yml: package %q: %w", id, err)
		}
	}
	return nil
}

// Validate rejects a package that could never select anything and patterns
// path.Match cannot compile — both are silent no-ops at generation time, and
// a package that quietly selects nothing is worse than one that fails to
// load.
func (p SkillPackage) Validate() error {
	if len(p.Include) == 0 {
		return fmt.Errorf("at least one include pattern is required")
	}
	for _, patterns := range [][]string{p.Include, p.Exclude} {
		for _, pattern := range patterns {
			if pattern == "" {
				return fmt.Errorf("patterns must not be empty")
			}
			if _, err := path.Match(pattern, ""); err != nil {
				return fmt.Errorf("pattern %q: %w", pattern, err)
			}
		}
	}
	return nil
}

// Members returns the names this package selects, sorted by slug.
func (p SkillPackage) Members(names []SkillName) []SkillName {
	out := make([]SkillName, 0, len(names))
	for _, name := range names {
		if matchesAny(p.Include, name.Slug) && !matchesAny(p.Exclude, name.Slug) {
			out = append(out, name)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out
}

// SelectSkills returns the union of the enabled packages' members, sorted by
// slug, alongside the enabled names skills.yml does not define. A name
// selected by two packages appears once: the identity is the skill, and the
// same skill from two packages is the same file either way, which is why
// packages need no collision rule of their own.
func SelectSkills(lib SkillLibrary, enabled []string, names []SkillName) (selected []SkillName, missing []string) {
	seen := make(map[string]bool, len(names))
	for _, id := range enabled {
		pkg, ok := lib.Packages[id]
		if !ok {
			missing = append(missing, id)
			continue
		}
		for _, member := range pkg.Members(names) {
			if seen[member.Slug] {
				continue
			}
			seen[member.Slug] = true
			selected = append(selected, member)
		}
	}
	sort.Slice(selected, func(i, j int) bool { return selected[i].Slug < selected[j].Slug })
	sort.Strings(missing)
	return selected, missing
}

// matchesAny reports whether slug matches any pattern. A pattern that does
// not compile cannot match; Validate is what reports it, so matching stays
// total and callers need no error path.
func matchesAny(patterns []string, slug string) bool {
	for _, pattern := range patterns {
		if ok, err := path.Match(pattern, slug); err == nil && ok {
			return true
		}
	}
	return false
}
