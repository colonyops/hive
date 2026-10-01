package agentws

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/mcpcatalog"
)

func testWorkspace() Workspace {
	return Workspace{Dir: "demo", Version: 1, Name: "Demo", Command: "claude"}
}

func testServers() map[string]mcpcatalog.Server {
	return map[string]mcpcatalog.Server{
		"zulu-remote": {Transport: mcpcatalog.TransportHttp, URL: "https://example.com/mcp", Headers: map[string]string{"Authorization": "Bearer xyz"}},
		"alpha-local": {Transport: mcpcatalog.TransportStdio, Command: "npx", Args: []string{"-y", "@playwright/mcp@latest"}},
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}

type snapshotEntry struct {
	data []byte
	mode fs.FileMode
}

// snapshotTree reads every regular file under dir, keyed by its path relative
// to dir, for comparing two generated trees against each other.
func snapshotTree(t *testing.T, dir string) map[string]snapshotEntry {
	t.Helper()
	out := map[string]snapshotEntry{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		require.NoError(t, err)
		if d.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(dir, path)
		require.NoError(t, relErr)
		data, readErr := os.ReadFile(path)
		require.NoError(t, readErr)
		info, infoErr := d.Info()
		require.NoError(t, infoErr)
		out[rel] = snapshotEntry{data: data, mode: info.Mode()}
		return nil
	})
	require.NoError(t, err)
	return out
}

func mtimes(t *testing.T, dir string, paths ...string) map[string]time.Time {
	t.Helper()
	out := make(map[string]time.Time, len(paths))
	for _, p := range paths {
		info, err := os.Stat(filepath.Join(dir, p))
		require.NoError(t, err)
		out[p] = info.ModTime()
	}
	return out
}

// TestGenerateIsDeterministic generates the same workspace into two different
// Dirs under two different roots — the cross-machine form spec §4.4 means —
// and asserts the whole generated tree is byte-identical, mode-identical, and
// carries no generator-minted absolute path from either root.
func TestGenerateIsDeterministic(t *testing.T) {
	t.Parallel()

	rootA := t.TempDir()
	rootB := t.TempDir()
	dirA := filepath.Join(rootA, "demo")
	dirB := filepath.Join(rootB, "demo")
	require.NoError(t, os.MkdirAll(dirA, 0o700))
	require.NoError(t, os.MkdirAll(dirB, 0o700))

	writeFile(t, filepath.Join(dirA, "AGENTS.md"), "# Demo\n")
	writeFile(t, filepath.Join(dirB, "AGENTS.md"), "# Demo\n")

	ws := testWorkspace()
	ws.MCPs = []string{"alpha-local", "zulu-remote"}
	ws.Skills = []string{"hive", "notes"}
	skills := []RenderedSkill{{Slug: "hive-mcp", Body: "# MCP\n"}, {Slug: "team-notes", Body: "# Team notes\n"}}

	inA := GenerateInput{Dir: dirA, Workspace: ws, Servers: testServers(), Skills: skills}
	inB := GenerateInput{Dir: dirB, Workspace: ws, Servers: testServers(), Skills: skills}

	resA, err := Generate(inA)
	require.NoError(t, err)
	resB, err := Generate(inB)
	require.NoError(t, err)
	assert.Equal(t, resA, resB)
	assert.Empty(t, resA.MissingMCPs)
	assert.Empty(t, resA.Problems)

	snapA := snapshotTree(t, dirA)
	snapB := snapshotTree(t, dirB)
	require.Len(t, snapB, len(snapA), "the two generated trees must have the same file set")

	for rel, a := range snapA {
		b, ok := snapB[rel]
		require.True(t, ok, "path %q missing from the second generation", rel)
		assert.Equal(t, a.mode, b.mode, "mode for %q", rel)
		assert.Equal(t, string(a.data), string(b.data), "content for %q must be byte-identical", rel)
		assert.NotContains(t, string(a.data), rootA, "output %q must not embed a generator-minted absolute path", rel)
		assert.NotContains(t, string(b.data), rootB, "output %q must not embed a generator-minted absolute path", rel)
	}

	mcpJSON := string(snapA[".mcp.json"].data)
	require.Contains(t, mcpJSON, "alpha-local")
	require.Contains(t, mcpJSON, "zulu-remote")
	assert.Less(t, strings.Index(mcpJSON, "alpha-local"), strings.Index(mcpJSON, "zulu-remote"), ".mcp.json must be key-sorted")

	codexTOML := string(snapA[filepath.Join(".codex", "config.toml")].data)
	require.Contains(t, codexTOML, "mcp_servers.alpha-local")
	require.Contains(t, codexTOML, "mcp_servers.zulu-remote")
	assert.Less(t, strings.Index(codexTOML, "mcp_servers.alpha-local"), strings.Index(codexTOML, "mcp_servers.zulu-remote"), ".codex/config.toml must be key-sorted")
}

// TestGenerateDoesNotRewriteUnchangedFiles is the production-observable half
// of determinism: a second generate over identical inputs must not touch any
// file's mtime, or every workspace open would re-sync the whole tree to
// iCloud regardless of whether anything changed.
func TestGenerateDoesNotRewriteUnchangedFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "AGENTS.md"), "# Demo\n")

	ws := testWorkspace()
	skills := []RenderedSkill{{Slug: "hive-mcp", Body: "# MCP\n"}}
	in := GenerateInput{Dir: dir, Workspace: ws, Servers: testServers(), Skills: skills}

	_, err := Generate(in)
	require.NoError(t, err)

	paths := []string{
		"CLAUDE.md",
		".mcp.json",
		filepath.Join(".codex", "config.toml"),
		filepath.Join(".claude", "skills", "hive-mcp", "SKILL.md"),
		filepath.Join(".agents", "skills", "hive-mcp", "SKILL.md"),
	}
	before := mtimes(t, dir, paths...)

	time.Sleep(10 * time.Millisecond)
	_, err = Generate(in)
	require.NoError(t, err)

	after := mtimes(t, dir, paths...)
	assert.Equal(t, before, after)
}

// TestGenerateReplacesGeneratedAndLeavesAuthored hand-edits every generated
// file and confirms a reopen replaces it, while AGENTS.md,
// agent-workspace.yaml and an agent-written file under docs/ survive
// untouched.
func TestGenerateReplacesGeneratedAndLeavesAuthored(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "AGENTS.md"), "# Demo\n")
	writeFile(t, filepath.Join(dir, manifestFileName), "version: 2\nname: Demo\nagent: claude\ncommand: claude\n")
	writeFile(t, filepath.Join(dir, "docs", "notes.md"), "agent notes\n")

	in := GenerateInput{Dir: dir, Workspace: testWorkspace(), Servers: testServers()}
	_, err := Generate(in)
	require.NoError(t, err)

	writeFile(t, filepath.Join(dir, "CLAUDE.md"), "hand edited\n")
	writeFile(t, filepath.Join(dir, ".mcp.json"), "{}")
	writeFile(t, filepath.Join(dir, ".codex", "config.toml"), "# edited\n")

	_, err = Generate(in)
	require.NoError(t, err)

	claude, err := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	require.NoError(t, err)
	assert.Equal(t, "# Demo\n", string(claude), "CLAUDE.md is regenerated from AGENTS.md")

	mcpJSON, err := os.ReadFile(filepath.Join(dir, ".mcp.json"))
	require.NoError(t, err)
	assert.NotEqual(t, "{}", string(mcpJSON))

	codexTOML, err := os.ReadFile(filepath.Join(dir, ".codex", "config.toml"))
	require.NoError(t, err)
	assert.NotEqual(t, "# edited\n", string(codexTOML))

	agentsMD, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	require.NoError(t, err)
	assert.Equal(t, "# Demo\n", string(agentsMD))

	manifest, err := os.ReadFile(filepath.Join(dir, manifestFileName))
	require.NoError(t, err)
	assert.Contains(t, string(manifest), "name: Demo")

	notes, err := os.ReadFile(filepath.Join(dir, "docs", "notes.md"))
	require.NoError(t, err)
	assert.Equal(t, "agent notes\n", string(notes))
}

// TestGenerateRemovesASkillNoLongerDeclared confirms a skill dropped from the
// declared set disappears from both generated trees, not just stops being
// rewritten.
func TestGenerateRemovesASkillNoLongerDeclared(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	in := GenerateInput{
		Dir: dir, Workspace: testWorkspace(),
		Servers: map[string]mcpcatalog.Server{},
		Skills:  []RenderedSkill{{Slug: "hive-mcp", Body: "# MCP\n"}},
	}
	_, err := Generate(in)
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(dir, ".claude", "skills", "hive-mcp", "SKILL.md"))
	assert.FileExists(t, filepath.Join(dir, ".agents", "skills", "hive-mcp", "SKILL.md"))

	in.Skills = nil
	_, err = Generate(in)
	require.NoError(t, err)

	assert.NoFileExists(t, filepath.Join(dir, ".claude", "skills", "hive-mcp", "SKILL.md"))
	assert.NoDirExists(t, filepath.Join(dir, ".claude", "skills", "hive-mcp"))
	assert.NoFileExists(t, filepath.Join(dir, ".agents", "skills", "hive-mcp", "SKILL.md"))
	assert.NoDirExists(t, filepath.Join(dir, ".agents", "skills", "hive-mcp"))
}

// TestGenerateInstallsExactlyWhatItIsHanded is the generator's half of the
// package contract: it expands no pattern and reads no directory of its own,
// so a shared skill no enabled package selected never lands in the tree.
func TestGenerateInstallsExactlyWhatItIsHanded(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dir := filepath.Join(root, "demo")
	require.NoError(t, os.MkdirAll(dir, 0o700))
	writeFile(t, filepath.Join(SharedSkillsDir(root), "team-notes", skillFileName), "# team notes\n")

	ws := testWorkspace()
	ws.Skills = []string{"hive"}
	in := GenerateInput{
		Dir: dir, Workspace: ws,
		Servers: map[string]mcpcatalog.Server{},
		Skills:  []RenderedSkill{{Slug: "hive-mcp", Body: "# MCP\n"}},
	}
	_, err := Generate(in)
	require.NoError(t, err)

	assert.FileExists(t, filepath.Join(dir, ".claude", "skills", "hive-mcp", "SKILL.md"))
	assert.FileExists(t, filepath.Join(dir, ".agents", "skills", "hive-mcp", "SKILL.md"))
	assert.NoDirExists(t, filepath.Join(dir, ".claude", "skills", "team-notes"),
		"a shared skill no enabled package selected must not install")
}

// TestCLAUDEMDIsACopyOfAGENTSMD covers the write, the update-on-edit, and the
// removal-on-delete cases in one pass.
func TestCLAUDEMDIsACopyOfAGENTSMD(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "AGENTS.md"), "# Original\n")

	in := GenerateInput{Dir: dir, Workspace: testWorkspace(), Servers: map[string]mcpcatalog.Server{}}
	_, err := Generate(in)
	require.NoError(t, err)
	data, err := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	require.NoError(t, err)
	assert.Equal(t, "# Original\n", string(data))

	writeFile(t, filepath.Join(dir, "AGENTS.md"), "# Updated\n")
	_, err = Generate(in)
	require.NoError(t, err)
	data, err = os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	require.NoError(t, err)
	assert.Equal(t, "# Updated\n", string(data))

	require.NoError(t, os.Remove(filepath.Join(dir, "AGENTS.md")))
	_, err = Generate(in)
	require.NoError(t, err)
	assert.NoFileExists(t, filepath.Join(dir, "CLAUDE.md"))
}

// TestGenerateReportsAnEvictedAuthoredFile confirms an .icloud placeholder
// standing in for AGENTS.md lands a Problem instead of silently generating an
// empty CLAUDE.md (spec §4.4) — the recoverable state must stay visible.
func TestGenerateReportsAnEvictedAuthoredFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".AGENTS.md.icloud"), "")

	in := GenerateInput{Dir: dir, Workspace: testWorkspace(), Servers: map[string]mcpcatalog.Server{}}
	res, err := Generate(in)
	require.NoError(t, err)
	require.Len(t, res.Problems, 1)
	assert.Contains(t, res.Problems[0], "AGENTS.md")
	assert.NoFileExists(t, filepath.Join(dir, "CLAUDE.md"))
}

// TestGenerateReportsAnUnwritableWorkspace: a read-only workspace directory
// must surface the write failure rather than half-generating.
// Precedent: TestSeedDefaultsIfMissingReportsDirectoryFailure
// (cmd/desktop/internal/app/actions/seed_test.go).
func TestGenerateReportsAnUnwritableWorkspace(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: chmod 0o500 does not block writes")
	}
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "AGENTS.md"), "# Demo\n")
	require.NoError(t, os.Chmod(dir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	in := GenerateInput{Dir: dir, Workspace: testWorkspace(), Servers: map[string]mcpcatalog.Server{}}
	_, err := Generate(in)
	require.Error(t, err)
}

// TestGenerateRejectsATraversingSkillSlug: a slug that is not a direct child
// of the skills tree is refused rather than written. spec §8 hands the
// workspace manifest to an agent to author, so this is validated even though
// only known prompt ids reach it today.
func TestGenerateRejectsATraversingSkillSlug(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		slug string
	}{
		{"ParentTraversal", "../outside"},
		{"AbsolutePath", "/etc/passwd"},
		{"Empty", ""},
		{"DotDot", ".."},
		{"Dot", "."},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			in := GenerateInput{
				Dir: dir, Workspace: testWorkspace(),
				Servers: map[string]mcpcatalog.Server{},
				Skills:  []RenderedSkill{{Slug: c.slug, Body: "x"}},
			}
			_, err := Generate(in)
			require.ErrorIs(t, err, ErrInvalidSkillSlug)
			assert.NoFileExists(t, filepath.Join(dir, ".claude", "skills"))
		})
	}
}

// TestDocsIsSeededEmptyAndUntouched: docs/ is created if missing, and a file
// an agent writes there survives a reopen with its mtime intact — no
// rendering, no indexing, no seeded content (D-E).
func TestDocsIsSeededEmptyAndUntouched(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	in := GenerateInput{Dir: dir, Workspace: testWorkspace(), Servers: map[string]mcpcatalog.Server{}}

	_, err := Generate(in)
	require.NoError(t, err)
	info, err := os.Stat(filepath.Join(dir, "docs"))
	require.NoError(t, err)
	assert.True(t, info.IsDir())
	entries, err := os.ReadDir(filepath.Join(dir, "docs"))
	require.NoError(t, err)
	assert.Empty(t, entries)

	writeFile(t, filepath.Join(dir, "docs", "notes.md"), "written by the agent\n")
	before, err := os.Stat(filepath.Join(dir, "docs", "notes.md"))
	require.NoError(t, err)

	time.Sleep(10 * time.Millisecond)
	_, err = Generate(in)
	require.NoError(t, err)

	after, err := os.Stat(filepath.Join(dir, "docs", "notes.md"))
	require.NoError(t, err)
	assert.Equal(t, before.ModTime(), after.ModTime())
	data, err := os.ReadFile(filepath.Join(dir, "docs", "notes.md"))
	require.NoError(t, err)
	assert.Equal(t, "written by the agent\n", string(data))
}
