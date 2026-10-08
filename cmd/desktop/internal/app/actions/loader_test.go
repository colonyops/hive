package actions

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/configmigrate"
)

func writeActionsFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

// multiActionYAML is a valid actions.yml with one of each supported type.
const multiActionYAML = `version: 1
actions:
  - id: spawn-review
    label: Spawn review agent
    type: launch-session
    show_in_detail: true
    applies_to: [pr]
    prompt_template: "Review {{ .Payload.title }}"
    agent: claude
    repo_template: "{{ .Payload.repo }}"
  - id: run-lint
    label: Run lint
    type: shell
    show_in_detail: true
    command_template: "lint {{ .Payload.path | shq }}"
    cwd: /tmp
    timeout: "30s"
    env:
      FOO: bar
  - id: notify
    label: Notify
    type: publish-message
    show_in_detail: true
    message_template: "{{ .Payload.title }}"
    topic: pipeline.notify
`

func TestLoadActions_MultiActionFile(t *testing.T) {
	dir := t.TempDir()
	path := writeActionsFile(t, dir, "actions.yml", multiActionYAML)

	got, err := LoadCatalog(path)
	require.NoError(t, err)
	require.Len(t, got.Actions, 3)

	byID := make(map[string]Action, len(got.Actions))
	for _, a := range got.Actions {
		byID[a.ID] = a
	}

	spawn := byID["spawn-review"]
	assert.Equal(t, "launch-session", spawn.Type)
	_, ok := spawn.Config.(*LaunchSessionConfig)
	require.True(t, ok)

	lint := byID["run-lint"]
	assert.Equal(t, "shell", lint.Type)
	shCfg, ok := lint.Config.(*ShellConfig)
	require.True(t, ok)
	assert.Equal(t, "/tmp", shCfg.Cwd)
	assert.Equal(t, "bar", shCfg.Env["FOO"])

	notify := byID["notify"]
	assert.Equal(t, "publish-message", notify.Type)
	peCfg, ok := notify.Config.(*PublishMessageConfig)
	require.True(t, ok)
	assert.Equal(t, "pipeline.notify", peCfg.Topic)
}

func TestLoadActions_MissingFile_IsEmptySetNotError(t *testing.T) {
	got, err := LoadCatalog(filepath.Join(t.TempDir(), "nope.yml"))
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestLoadActions_EmptyFile_IsEmptySetNotError(t *testing.T) {
	dir := t.TempDir()
	path := writeActionsFile(t, dir, "actions.yml", "")

	got, err := LoadCatalog(path)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestLoadActions_VersionMismatch_IsError(t *testing.T) {
	dir := t.TempDir()
	path := writeActionsFile(t, dir, "actions.yml", `version: 2
actions: []
`)
	_, err := LoadCatalog(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "version")
}

func TestLoadActions_UnknownTopLevelField_IsError(t *testing.T) {
	dir := t.TempDir()
	path := writeActionsFile(t, dir, "actions.yml", `version: 1
bogus: true
actions: []
`)
	_, err := LoadCatalog(path)
	require.Error(t, err)
}

func TestLoadActions_DuplicateID_IsError(t *testing.T) {
	dir := t.TempDir()
	path := writeActionsFile(t, dir, "actions.yml", `version: 1
actions:
  - id: dup
    label: One
    type: shell
    command_template: "true"
  - id: dup
    label: Two
    type: shell
    command_template: "true"
`)
	_, err := LoadCatalog(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate")
}

func TestLoadActions_BadSlug_IsError(t *testing.T) {
	dir := t.TempDir()
	path := writeActionsFile(t, dir, "actions.yml", `version: 1
actions:
  - id: "Not A Slug"
    label: X
    type: shell
    command_template: "true"
`)
	_, err := LoadCatalog(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "slug")
}

func TestLoadActions_MissingLabel_IsError(t *testing.T) {
	dir := t.TempDir()
	path := writeActionsFile(t, dir, "actions.yml", `version: 1
actions:
  - id: x
    type: shell
    command_template: "true"
`)
	_, err := LoadCatalog(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "label")
}

func TestLoadActions_UnsupportedLaunchSessionPost_IsError(t *testing.T) {
	dir := t.TempDir()
	path := writeActionsFile(t, dir, "actions.yml", `version: 1
actions:
  - id: x
    label: X
    type: launch-session
    prompt_template: "Review {{ .Payload.title }}"
    post: "comment"
`)
	_, err := LoadCatalog(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "post")
}

func TestLoadActions_MissingRequiredPerTypeField_IsError(t *testing.T) {
	dir := t.TempDir()
	path := writeActionsFile(t, dir, "actions.yml", `version: 1
actions:
  - id: x
    label: X
    type: launch-session
`)
	_, err := LoadCatalog(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "prompt_template")
}

// TestLoadActions_InjectedSetMigratesThroughRealLoader proves migrate-before-
// strict-decode composes through the real loader: a version:1 fixture is
// driven through LoadActions -> configmigrate.ActionsSet.Apply -> parseActions
// with a temporarily raised Current, and must come out as a valid catalog.
func TestLoadActions_InjectedSetMigratesThroughRealLoader(t *testing.T) {
	original := configmigrate.ActionsSet
	configmigrate.ActionsSet = configmigrate.Set{
		Name:     "actions",
		Baseline: 1,
		Current:  2,
		Migrations: []configmigrate.Migration{
			{To: 2, Migrate: func(doc map[string]any) error { return nil }},
		},
	}
	t.Cleanup(func() { configmigrate.ActionsSet = original })

	dir := t.TempDir()
	path := writeActionsFile(t, dir, "actions.yml", multiActionYAML)

	got, err := LoadCatalog(path)
	require.NoError(t, err)
	require.Len(t, got.Actions, 3)
}

func TestLoadActions_CorruptFileErrors(t *testing.T) {
	dir := t.TempDir()
	path := writeActionsFile(t, dir, "actions.yml", "not: [unterminated")
	_, err := LoadCatalog(path)
	require.Error(t, err)
}

// TestLoadActions_CurrentFileIsNotRewritten proves the read path is pure
// Apply: loading an already-current actions.yml must never write to disk.
func TestLoadActions_CurrentFileIsNotRewritten(t *testing.T) {
	dir := t.TempDir()
	path := writeActionsFile(t, dir, "actions.yml", multiActionYAML)
	before, err := os.ReadFile(path)
	require.NoError(t, err)

	_, err = LoadCatalog(path)
	require.NoError(t, err)

	after, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, before, after, "load path is pure Apply and must never write")
}

func TestLoadActions_BadDuration_IsError(t *testing.T) {
	dir := t.TempDir()
	path := writeActionsFile(t, dir, "actions.yml", `version: 1
actions:
  - id: x
    label: X
    type: shell
    command_template: "true"
    timeout: 30
`)
	_, err := LoadCatalog(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot unmarshal !!int `30` into time.Duration")
}
