package agentws

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testNames() []SkillName {
	return SkillNames(
		[]ShippedSkill{{Slug: "hive-mcp"}, {Slug: "hive-flows"}, {Slug: "hive-settings"}},
		[]SharedSkill{{Slug: "terraform-plan"}, {Slug: "terraform-apply"}, {Slug: "runbook"}},
	)
}

// TestSelectSkillsUnionsPackages covers what a workspace's skills: list buys
// it: every enabled package's matches, once each, plus the packages
// skills.yml does not define.
func TestSelectSkillsUnionsPackages(t *testing.T) {
	t.Parallel()

	lib := SkillLibrary{Version: 1, Packages: map[string]SkillPackage{
		"hive":  {Include: []string{"hive-*"}},
		"infra": {Include: []string{"terraform-*", "runbook"}},
		// Overlaps infra deliberately: a skill two packages select is still
		// one skill, which is why packages need no collision rule.
		"terraform": {Include: []string{"terraform-*"}},
	}}

	selected, missing := SelectSkills(lib, []string{"infra", "terraform", "ghost"}, testNames())

	slugs := make([]string, 0, len(selected))
	for _, name := range selected {
		slugs = append(slugs, name.Slug)
	}
	assert.Equal(t, []string{"runbook", "terraform-apply", "terraform-plan"}, slugs,
		"sorted, de-duplicated across packages")
	assert.Equal(t, []string{"ghost"}, missing)
}

// TestPackageMembersRespectExclude is the carve-out: excludes apply after
// includes, so a package can take a family minus one of its members.
func TestPackageMembersRespectExclude(t *testing.T) {
	t.Parallel()

	pkg := SkillPackage{Include: []string{"hive-*"}, Exclude: []string{"hive-settings"}}
	members := pkg.Members(testNames())

	slugs := make([]string, 0, len(members))
	for _, name := range members {
		slugs = append(slugs, name.Slug)
	}
	assert.Equal(t, []string{"hive-flows", "hive-mcp"}, slugs)
}

// TestSelectSkillsCarriesProvenance keeps the two sources distinguishable
// after matching: the service renders a shipped skill per install and installs
// a shared one verbatim, so it has to be able to tell them apart.
func TestSelectSkillsCarriesProvenance(t *testing.T) {
	t.Parallel()

	lib := SkillLibrary{Version: 1, Packages: map[string]SkillPackage{
		"everything": {Include: []string{"*"}},
	}}
	selected, missing := SelectSkills(lib, []string{"everything"}, testNames())
	require.Len(t, selected, 6)
	assert.Empty(t, missing)

	shipped := map[string]bool{}
	for _, name := range selected {
		shipped[name.Slug] = name.Shipped
	}
	assert.True(t, shipped["hive-mcp"])
	assert.False(t, shipped["runbook"])
}

// TestSkillNamesPreferTheAuthoredCopy: a slug is one skill, and a shared file
// of the same name is the one the user can see and edit, so it wins.
func TestSkillNamesPreferTheAuthoredCopy(t *testing.T) {
	t.Parallel()

	names := SkillNames([]ShippedSkill{{Slug: "hive-mcp"}}, []SharedSkill{{Slug: "hive-mcp"}})
	require.Len(t, names, 1)
	assert.False(t, names[0].Shipped, "the authored copy replaces the shipped one of the same name")
}

func TestSkillLibraryValidate(t *testing.T) {
	t.Parallel()

	t.Run("RejectsAPackageThatCouldNeverSelectAnything", func(t *testing.T) {
		t.Parallel()
		lib := SkillLibrary{Version: 1, Packages: map[string]SkillPackage{"empty": {}}}
		require.ErrorContains(t, lib.Validate(), "at least one include pattern")
	})

	t.Run("RejectsAPatternThatCannotCompile", func(t *testing.T) {
		t.Parallel()
		lib := SkillLibrary{Version: 1, Packages: map[string]SkillPackage{
			"broken": {Include: []string{"hive-["}},
		}}
		require.ErrorContains(t, lib.Validate(), "pattern")
	})

	t.Run("RejectsAPackageNameThatIsNotASegment", func(t *testing.T) {
		t.Parallel()
		lib := SkillLibrary{Version: 1, Packages: map[string]SkillPackage{
			"../escape": {Include: []string{"*"}},
		}}
		require.ErrorContains(t, lib.Validate(), "path-safe")
	})
}

// TestSkillPackageCatalogueResolvesMembers is the editor's read: a package
// carries what it currently selects, and one selecting nothing still lists —
// an empty package is a pattern to fix, not a row to hide.
func TestSkillPackageCatalogueResolvesMembers(t *testing.T) {
	t.Parallel()

	lib := SkillLibrary{Version: 1, Packages: map[string]SkillPackage{
		"hive":   {Title: "Hive", Include: []string{"hive-*"}},
		"typoed": {Include: []string{"k8s-*"}},
	}}
	entries := SkillPackageCatalogue(lib, testNames())
	require.Len(t, entries, 2)

	assert.Equal(t, "hive", entries[0].Name)
	assert.Equal(t, "Hive", entries[0].Title)
	assert.Len(t, entries[0].Members, 3)

	assert.Equal(t, "typoed", entries[1].Name)
	assert.Equal(t, "typoed", entries[1].Title, "a package with no title is labelled by its name")
	assert.Empty(t, entries[1].Members)
}

func TestLoadSkillLibrary(t *testing.T) {
	t.Parallel()

	t.Run("ReadsPackages", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		writeFile(t, SkillLibraryPath(root), "version: 1\npackages:\n  infra:\n    title: Infra\n    include: [\"terraform-*\"]\n    exclude: [\"terraform-apply\"]\n")

		lib, err := LoadSkillLibrary(SkillLibraryPath(root))
		require.NoError(t, err)
		require.Contains(t, lib.Packages, "infra")
		assert.Equal(t, "Infra", lib.Packages["infra"].Title)
		assert.Equal(t, []string{"terraform-apply"}, lib.Packages["infra"].Exclude)
	})

	t.Run("RejectsAnUnknownKey", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		writeFile(t, filepath.Join(root, skillLibraryFileName), "version: 1\npackages:\n  infra:\n    includes: [\"terraform-*\"]\n")

		_, err := LoadSkillLibrary(SkillLibraryPath(root))
		require.Error(t, err, "a typo'd key must fail the load rather than silently selecting nothing")
	})
}

// TestSkillNameCatalogueNamesWhatSelectsEachSkill is the lookup the
// unresolved-name report and the workspace editor both read: a name and the
// packages that would carry it.
func TestSkillNameCatalogueNamesWhatSelectsEachSkill(t *testing.T) {
	t.Parallel()

	lib := SkillLibrary{Version: 1, Packages: map[string]SkillPackage{
		"hive":      {Include: []string{"hive-*"}, Exclude: []string{"hive-settings"}},
		"infra":     {Include: []string{"terraform-*", "runbook"}},
		"terraform": {Include: []string{"terraform-*"}},
	}}

	bySlug := make(map[string]SkillNameEntry)
	for _, entry := range SkillNameCatalogue(lib, testNames()) {
		bySlug[entry.Slug] = entry
	}

	assert.Equal(t, []string{"hive"}, bySlug["hive-mcp"].SelectedBy)
	assert.True(t, bySlug["hive-mcp"].Shipped)
	assert.Equal(t, []string{"infra", "terraform"}, bySlug["terraform-plan"].SelectedBy, "sorted, and a skill can belong to several")
	assert.Empty(t, bySlug["hive-settings"].SelectedBy, "an excluded skill is selected by nothing")
}

// TestExplainUnresolvedSeparatesASkillFromATypo is #307: both cases reach
// SelectSkills as an undefined package name, and only one of them has a fix
// the user can act on.
func TestExplainUnresolvedSeparatesASkillFromATypo(t *testing.T) {
	t.Parallel()

	lib := SkillLibrary{Version: 1, Packages: map[string]SkillPackage{"hive": {Include: []string{"hive-*"}}}}
	catalogue := SkillNameCatalogue(lib, testNames())

	assert.Equal(t, []UnresolvedName{
		{Name: "hive-mcp", Skill: true, SelectedBy: []string{"hive"}},
		{Name: "runbook", Skill: true},
		{Name: "ghost"},
	}, ExplainUnresolved(catalogue, []string{"hive-mcp", "runbook", "ghost"}),
		"a skill a package carries, a skill nothing carries, and a name that is no skill at all")
}
