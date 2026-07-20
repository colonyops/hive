package actions

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
    auto_apply: true
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
    type: publish-event
    show_in_detail: true
    topic: pipeline.notify
`

func TestLoadActions_MultiActionFile(t *testing.T) {
	dir := t.TempDir()
	path := writeActionsFile(t, dir, "actions.yml", multiActionYAML)

	got, err := LoadActions(path)
	require.NoError(t, err)
	require.Len(t, got, 3)

	byID := make(map[string]Action, len(got))
	for _, a := range got {
		byID[a.ID] = a
	}

	spawn := byID["spawn-review"]
	assert.Equal(t, "launch-session", spawn.Type)
	assert.True(t, spawn.AutoApply)
	_, ok := spawn.Config.(*LaunchSessionConfig)
	require.True(t, ok)

	lint := byID["run-lint"]
	assert.Equal(t, "shell", lint.Type)
	shCfg, ok := lint.Config.(*ShellConfig)
	require.True(t, ok)
	assert.Equal(t, "/tmp", shCfg.Cwd)
	assert.Equal(t, "bar", shCfg.Env["FOO"])

	notify := byID["notify"]
	assert.Equal(t, "publish-event", notify.Type)
	peCfg, ok := notify.Config.(*PublishEventConfig)
	require.True(t, ok)
	assert.Equal(t, "pipeline.notify", peCfg.Topic)
}

func TestLoadActions_MissingFile_IsEmptySetNotError(t *testing.T) {
	got, err := LoadActions(filepath.Join(t.TempDir(), "nope.yml"))
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestLoadActions_EmptyFile_IsEmptySetNotError(t *testing.T) {
	dir := t.TempDir()
	path := writeActionsFile(t, dir, "actions.yml", "")

	got, err := LoadActions(path)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestLoadActions_VersionMismatch_IsError(t *testing.T) {
	dir := t.TempDir()
	path := writeActionsFile(t, dir, "actions.yml", `version: 2
actions: []
`)
	_, err := LoadActions(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "version")
}

func TestLoadActions_UnknownTopLevelField_IsError(t *testing.T) {
	dir := t.TempDir()
	path := writeActionsFile(t, dir, "actions.yml", `version: 1
bogus: true
actions: []
`)
	_, err := LoadActions(path)
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
	_, err := LoadActions(path)
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
	_, err := LoadActions(path)
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
	_, err := LoadActions(path)
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
	_, err := LoadActions(path)
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
	_, err := LoadActions(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "prompt_template")
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
	_, err := LoadActions(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bare number")
}
