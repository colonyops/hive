package agentws

import "sort"

// ShippedSkill is one skill this build ships, as the catalogue needs it. The
// shipped half arrives as data rather than being read here: the bodies are
// rendered per install by the prompt engine, and internal/app/prompts imports
// this package, so the dependency cannot run the other way.
type ShippedSkill struct {
	Slug        string
	Title       string
	Description string
}

// SkillPackageEntry is one row of the package catalogue — a package as the
// workspace editor shows it, with the names it currently resolves to. Members
// are resolved for display rather than declared: a package is patterns, so
// what it contains is a question only the current name-space can answer.
type SkillPackageEntry struct {
	Name        string
	Title       string
	Description string
	Members     []SkillName
}

// SkillNames is the name-space packages glob over: every shipped skill plus
// every skill authored in the shared directory, sorted, with a shared skill
// of the same name winning — a slug is one skill, and the authored copy is
// the one the user can see and edit.
func SkillNames(shipped []ShippedSkill, sharedSkills []SharedSkill) []SkillName {
	names := make(map[string]SkillName, len(shipped)+len(sharedSkills))
	for _, s := range shipped {
		names[s.Slug] = SkillName{Slug: s.Slug, Shipped: true}
	}
	for _, s := range sharedSkills {
		names[s.Slug] = SkillName{Slug: s.Slug}
	}

	out := make([]SkillName, 0, len(names))
	for _, name := range names {
		out = append(out, name)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out
}

// SkillNameEntry is one name in the skills name-space with the packages that
// currently select it, sorted. SelectedBy is what makes an enabled name
// skills.yml does not define actionable: when that name is a skill, the fix
// is enabling a package that already selects it, not defining a package
// under the skill's own name.
type SkillNameEntry struct {
	Slug       string
	Shipped    bool
	SelectedBy []string
}

// UnresolvedName explains one enabled name SelectSkills could not resolve.
// A manifest written before packages were the enablement unit
// (ADR skill-packages-are-the-unit-a-workspace-enables) enumerates skill
// slugs, which now read as undefined packages — the same report as a typo
// from an entirely different cause, so the two are separated here rather
// than left for copy to guess at.
type UnresolvedName struct {
	Name string
	// Skill is set when the name is a skill rather than a package.
	Skill bool
	// SelectedBy names the packages that already select that skill.
	SelectedBy []string
}

// SkillNameCatalogue pairs every name with the packages selecting it, sorted
// by slug.
func SkillNameCatalogue(lib SkillLibrary, names []SkillName) []SkillNameEntry {
	selectedBy := make(map[string][]string, len(names))
	for id, pkg := range lib.Packages {
		for _, member := range pkg.Members(names) {
			selectedBy[member.Slug] = append(selectedBy[member.Slug], id)
		}
	}
	out := make([]SkillNameEntry, 0, len(names))
	for _, name := range names {
		packages := selectedBy[name.Slug]
		sort.Strings(packages)
		out = append(out, SkillNameEntry{Slug: name.Slug, Shipped: name.Shipped, SelectedBy: packages})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out
}

// ExplainUnresolved classifies each name a workspace enables that skills.yml
// does not define, in the order given.
func ExplainUnresolved(catalogue []SkillNameEntry, unresolved []string) []UnresolvedName {
	bySlug := make(map[string]SkillNameEntry, len(catalogue))
	for _, entry := range catalogue {
		bySlug[entry.Slug] = entry
	}
	out := make([]UnresolvedName, 0, len(unresolved))
	for _, name := range unresolved {
		entry, isSkill := bySlug[name]
		out = append(out, UnresolvedName{Name: name, Skill: isSkill, SelectedBy: entry.SelectedBy})
	}
	return out
}

// SkillPackageCatalogue lists every defined package with the names it
// resolves to, sorted by package name. A package matching nothing is listed
// with no members rather than hidden: an empty package is a pattern to fix,
// and hiding it would make skills.yml and the editor disagree.
func SkillPackageCatalogue(lib SkillLibrary, names []SkillName) []SkillPackageEntry {
	out := make([]SkillPackageEntry, 0, len(lib.Packages))
	for id, pkg := range lib.Packages {
		title := pkg.Title
		if title == "" {
			title = id
		}
		out = append(out, SkillPackageEntry{
			Name:        id,
			Title:       title,
			Description: pkg.Description,
			Members:     pkg.Members(names),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
