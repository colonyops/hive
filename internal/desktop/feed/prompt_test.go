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
	assert.Contains(t, prompt, "exclude_repos")
	assert.Contains(t, prompt, "hot-reloads")
	assert.NotContains(t, prompt, "does not exist")
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
