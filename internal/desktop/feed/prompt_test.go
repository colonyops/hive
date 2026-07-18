package feed

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildConfigPrompt(t *testing.T) {
	t.Parallel()

	prompt := BuildConfigPrompt(ConfigInfo{
		Path:   "/home/u/.config/hive/desktop/profiles.yaml",
		Exists: true,
		YAML:   "profiles:\n  - id: work\n",
		Valid:  true,
	})

	assert.Contains(t, prompt, "/home/u/.config/hive/desktop/profiles.yaml")
	assert.Contains(t, prompt, "- id: work")
	assert.Contains(t, prompt, "hot-reloads")
	assert.NotContains(t, prompt, "does not exist")

	// The schema contract: sources vs feeds, the filter vocabulary, and the
	// rate-limit constraints the agent must respect.
	assert.Contains(t, prompt, "sources:")
	assert.Contains(t, prompt, "exclude_repos")
	assert.Contains(t, prompt, "exclude_authors")
	assert.Contains(t, prompt, "exclude_labels")
	assert.Contains(t, prompt, "types:")
	assert.Contains(t, prompt, "reasons:")
	assert.Contains(t, prompt, "a reasons filter excludes them")
	assert.Contains(t, prompt, "25 sources of kind \"search\"")
	assert.Contains(t, prompt, "30 requests/min")
	assert.Contains(t, prompt, "Feeds are unlimited and free")
	assert.Contains(t, prompt, "304")
}

func TestBuildConfigPromptMissingFile(t *testing.T) {
	t.Parallel()

	prompt := BuildConfigPrompt(ConfigInfo{
		Path: "/home/u/.config/hive/desktop/profiles.yaml",
		YAML: ExampleConfig(),
	})

	assert.Contains(t, prompt, "does not exist yet")
	assert.Contains(t, prompt, "kind: notifications")
}
