package build

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// The frontend install has to stay skippable while its lockfile is unchanged:
// every `wails3 dev` rebuild runs it as a dependency, and an npm ci there
// deletes node_modules under a concurrent vitest run (#120). `wails3 update
// build-assets` rewrites this file from the upstream template, whose
// directory-named `generates` is what broke the skip, so pin the declaration.
func TestFrontendInstallIsSkippableWhenLockfileIsUnchanged(t *testing.T) {
	var taskfile struct {
		Tasks map[string]yaml.Node `yaml:"tasks"`
	}

	raw, err := os.ReadFile("Taskfile.yml")
	require.NoError(t, err)
	require.NoError(t, yaml.Unmarshal(raw, &taskfile))

	node, ok := taskfile.Tasks["install:frontend:deps:npm"]
	require.True(t, ok, "install:frontend:deps:npm is missing from Taskfile.yml")

	var install struct {
		Sources   []string `yaml:"sources"`
		Generates []string `yaml:"generates"`
	}
	require.NoError(t, node.Decode(&install))

	assert.Equal(t, []string{"package.json", "package-lock.json"}, install.Sources,
		"npm ci must still re-run when either lockfile input changes")
	assert.Equal(t, []string{"node_modules/.package-lock.json"}, install.Generates,
		"generates must name a file npm writes, never the node_modules directory")
}
