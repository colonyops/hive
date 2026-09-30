package agentws

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteManifestPreservesCommentsAndUntouchedKeys(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dir := filepath.Join(root, "demo")
	require.NoError(t, os.Mkdir(dir, 0o700))
	// Current-version, because MigrateRoot runs at startup before anything
	// loads a manifest: WriteManifest only stamps a version onto a file it
	// creates, so an editable file is always already at Current.
	original := `# hand-authored: do not lose me
version: 5
name: Demo
# what runs here
command: claude
mcps:
  - hass-mcp # lights and sensors
skills:
  - pdf-tools
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, manifestFileName), []byte(original), 0o600))

	require.NoError(t, WriteManifest(root, "demo", ManifestEdit{
		Name: "Renamed", Command: "claude --permission-mode acceptEdits",
		MCPs: []string{"hass-mcp"}, Skills: []string{"pdf-tools"},
	}))

	raw, err := os.ReadFile(filepath.Join(dir, manifestFileName))
	require.NoError(t, err)
	text := string(raw)
	assert.Contains(t, text, "# hand-authored: do not lose me")
	assert.Contains(t, text, "# what runs here")

	ws, err := parseWorkspace([]byte(text))
	require.NoError(t, err)
	assert.Equal(t, "Renamed", ws.Name)
	assert.Equal(t, "claude --permission-mode acceptEdits", ws.Command)
	assert.Equal(t, []string{"hass-mcp"}, ws.MCPs)
	assert.Equal(t, []string{"pdf-tools"}, ws.Skills)
}

// TestWriteManifestOwnsTheCapabilityKeys covers both enablement lists: the
// writer replaces what it is given and removes a key it is given nothing for,
// rather than writing an empty list.
func TestWriteManifestOwnsTheCapabilityKeys(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dir := filepath.Join(root, "demo")
	require.NoError(t, os.Mkdir(dir, 0o700))
	original := "version: 5\nname: Demo\ncommand: claude\nmcps:\n  - playwright\nskills:\n  - hive\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, manifestFileName), []byte(original), 0o600))

	require.NoError(t, WriteManifest(root, "demo", ManifestEdit{
		Name: "Demo", Command: "claude",
		MCPs: []string{"playwright", "home-assistant"}, Skills: []string{"hive-mcp", "team-notes"},
	}))
	raw, err := os.ReadFile(filepath.Join(dir, manifestFileName))
	require.NoError(t, err)
	ws, err := parseWorkspace(raw)
	require.NoError(t, err)
	assert.Equal(t, []string{"playwright", "home-assistant"}, ws.MCPs)
	assert.Equal(t, []string{"hive-mcp", "team-notes"}, ws.Skills)

	require.NoError(t, WriteManifest(root, "demo", ManifestEdit{Name: "Demo", Command: "claude"}))
	raw, err = os.ReadFile(filepath.Join(dir, manifestFileName))
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "mcps", "an empty set removes the key rather than writing mcps: []")
	assert.NotContains(t, string(raw), "skills", "an empty set removes the key rather than writing skills: []")
	ws, err = parseWorkspace(raw)
	require.NoError(t, err)
	assert.Empty(t, ws.MCPs)
	assert.Empty(t, ws.Skills)
}

func TestCreateWorkspaceWritesALoadableManifest(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, CreateWorkspace(root, "fresh", ManifestEdit{
		Name: "Fresh", Command: "claude",
		MCPs: []string{"playwright"}, Skills: []string{"hive-mcp"},
	}))

	raw, err := os.ReadFile(filepath.Join(root, "fresh", manifestFileName))
	require.NoError(t, err)
	ws, err := parseWorkspace(raw)
	require.NoError(t, err)
	assert.Equal(t, "Fresh", ws.Name)
	assert.Equal(t, "claude", ws.Command)
	assert.Equal(t, []string{"playwright"}, ws.MCPs)
	assert.Equal(t, []string{"hive-mcp"}, ws.Skills)

	err = CreateWorkspace(root, "fresh", ManifestEdit{Name: "Fresh", Command: "claude"})
	require.ErrorIs(t, err, os.ErrExist)
}

func TestCreateWorkspaceScaffoldsAgentsMD(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, CreateWorkspace(root, "fresh", ManifestEdit{Name: "Paperless", Command: "claude"}))

	raw, err := os.ReadFile(filepath.Join(root, "fresh", "AGENTS.md"))
	require.NoError(t, err)
	assert.Contains(t, string(raw), "# Paperless")
	assert.Contains(t, string(raw), "agent workspace")

	// The scaffold is authored from the moment it lands: nothing regenerates
	// it, so a Generate over the workspace copies it to CLAUDE.md verbatim.
	result, err := Generate(GenerateInput{Dir: filepath.Join(root, "fresh")})
	require.NoError(t, err)
	assert.Empty(t, result.Problems)
	claude, err := os.ReadFile(filepath.Join(root, "fresh", "CLAUDE.md"))
	require.NoError(t, err)
	assert.Equal(t, string(raw), string(claude))
}

func TestRemoveWorkspaceDeletesTheWholeDirectory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, CreateWorkspace(root, "fresh", ManifestEdit{Name: "Fresh", Command: "claude"}))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "fresh", "canvases"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(root, "fresh", "canvases", "plan.json"), []byte("{}"), 0o600))

	require.NoError(t, RemoveWorkspace(root, "fresh"))
	assert.NoDirExists(t, filepath.Join(root, "fresh"))
}

func TestRemoveWorkspaceRefusesAnythingThatIsNotAWorkspace(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, manifestFileName), []byte("version: 3\n"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".shared", "skills"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(root, libraryFileName), []byte("version: 1\n"), 0o600))
	require.NoError(t, os.Symlink(outside, filepath.Join(root, "linked")))

	for _, dir := range []string{"", ".", "..", "../" + filepath.Base(outside), ".shared", libraryFileName, "linked", "never-created"} {
		require.Error(t, RemoveWorkspace(root, dir), "dir %q", dir)
	}

	assert.DirExists(t, outside)
	assert.DirExists(t, filepath.Join(root, ".shared", "skills"))
	assert.FileExists(t, filepath.Join(root, libraryFileName))
}
