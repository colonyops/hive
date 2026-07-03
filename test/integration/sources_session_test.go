//go:build integration

package integration

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// referenceSourceConfig returns a hive config.yaml with the bundled
// reference source registered as an external source at binPath, plus
// a batch_spawn rule that writes the rendered prompt to promptFile so tests
// can prove prompt injection reaches the spawn path without needing tmux
// (SpawnWith runs batch_spawn commands directly via `sh -c`, not through
// tmux).
func referenceSourceConfig(binPath, promptFile string) string {
	return fmt.Sprintf(`version: "0.2.4"
git_path: git
agents:
  default: testbash
  testbash:
    command: bash
rules:
  - batch_spawn:
      - "printf '%%s' {{ .Prompt | shq }} > %s"
sources:
  external:
    - id: reference
      command: [%q]
      templates:
        name: "ref-{{ .ID }}"
        prompt: "{{ .Detail }}"
        tags:
          - "status-{{ .Fields.status }}"
`, promptFile, binPath)
}

func TestCreateSessionFromSourceItem(t *testing.T) {
	binPath := buildReferenceSource(t)
	promptFile := filepath.Join(t.TempDir(), "prompt.txt")
	repo := createBareRepo(t, "source-repo")

	h := NewHarness(t).WithConfig(referenceSourceConfig(binPath, promptFile))

	out, err := h.RunStdout("source", "open", "reference", "--pick", "ref-1", "--remote", repo, "--json")
	require.NoError(t, err, "output: %s", out)

	sessions, err := h.RunJSONLines("ls", "--json")
	require.NoError(t, err)
	require.Len(t, sessions, 1)

	sess := sessions[0]
	assert.Equal(t, "ref-ref-1", sess["name"])

	tags, ok := sess["tags"].([]any)
	require.True(t, ok, "tags field missing or wrong type: %#v", sess["tags"])
	require.Len(t, tags, 1)
	assert.Equal(t, "status-open", tags[0])
}

func TestCreateSessionFromSourceItem_PromptReachesSpawn(t *testing.T) {
	binPath := buildReferenceSource(t)
	promptFile := filepath.Join(t.TempDir(), "prompt.txt")
	repo := createBareRepo(t, "source-repo")

	h := NewHarness(t).WithConfig(referenceSourceConfig(binPath, promptFile))

	out, err := h.RunStdout("source", "open", "reference", "--pick", "ref-2", "--remote", repo, "--json")
	require.NoError(t, err, "output: %s", out)

	content, err := os.ReadFile(promptFile)
	require.NoError(t, err, "batch_spawn did not write the prompt file")
	assert.Contains(t, string(content), "Canned detail body for item `ref-2`")
}

func TestSourceOpen_UnknownItemID(t *testing.T) {
	binPath := buildReferenceSource(t)
	promptFile := filepath.Join(t.TempDir(), "prompt.txt")
	repo := createBareRepo(t, "source-repo")

	h := NewHarness(t).WithConfig(referenceSourceConfig(binPath, promptFile))

	out, err := h.Run("source", "open", "reference", "--pick", "does-not-exist", "--remote", repo)
	require.Error(t, err, "output: %s", out)
	assert.Contains(t, out, "no item with id")

	sessions, err := h.RunJSONLines("ls", "--json")
	require.NoError(t, err)
	assert.Empty(t, sessions, "no session should be created for an unknown item id")
}

func TestSourceOpen_UnknownSourceID(t *testing.T) {
	binPath := buildReferenceSource(t)
	promptFile := filepath.Join(t.TempDir(), "prompt.txt")

	h := NewHarness(t).WithConfig(referenceSourceConfig(binPath, promptFile))

	out, err := h.Run("source", "open", "does-not-exist", "--pick", "ref-1")
	require.Error(t, err, "output: %s", out)
	assert.Contains(t, out, "unknown source")
}

func TestSourceOpen_MissingSourceID(t *testing.T) {
	binPath := buildReferenceSource(t)
	promptFile := filepath.Join(t.TempDir(), "prompt.txt")

	h := NewHarness(t).WithConfig(referenceSourceConfig(binPath, promptFile))

	out, err := h.Run("source", "open", "--pick", "ref-1")
	require.Error(t, err, "output: %s", out)
	assert.Contains(t, out, "source id is required")
}

// badTemplateSourceConfig registers the reference source with a name
// template referencing a Fields key the source never emits, which is a
// documented render-time error (missingkey=error for .Fields lookups).
func badTemplateSourceConfig(binPath string) string {
	return fmt.Sprintf(`version: "0.2.4"
git_path: git
agents:
  default: testbash
  testbash:
    command: bash
sources:
  external:
    - id: reference
      command: [%q]
      templates:
        name: "{{ .Fields.does_not_exist }}"
`, binPath)
}

func TestSourceOpen_TemplateRenderFailureCreatesNoSession(t *testing.T) {
	binPath := buildReferenceSource(t)
	repo := createBareRepo(t, "source-repo")

	h := NewHarness(t).WithConfig(badTemplateSourceConfig(binPath))

	out, err := h.Run("source", "open", "reference", "--pick", "ref-1", "--remote", repo)
	require.Error(t, err, "output: %s", out)

	sessions, err := h.RunJSONLines("ls", "--json")
	require.NoError(t, err)
	assert.Empty(t, sessions, "no session should be created when template rendering fails")
}
