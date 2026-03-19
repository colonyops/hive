package hive

import (
	"bytes"
	"context"
	"testing"

	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/pkg/executil"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunHooks_RendersTemplateVariables(t *testing.T) {
	var stdout, stderr bytes.Buffer
	renderer := testRenderer()
	log := zerolog.Nop()

	// Use a temp dir as the working directory
	dir := t.TempDir()

	runner := NewHookRunner(log, &executil.RealExecutor{}, renderer, &stdout, &stderr)

	rule := config.Rule{
		Pattern:  "",
		Commands: []string{"echo {{ .Name }}-{{ .Slug }}-{{ .ID }}"},
	}
	data := config.SpawnTemplateData{
		Name: "My Feature",
		Slug: "my-feature",
		ID:   "abc123",
		Path: dir,
	}

	err := runner.RunHooks(context.Background(), rule, dir, data)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "My Feature-my-feature-abc123")
}

func TestRunHooks_RendersOwnerAndRepo(t *testing.T) {
	var stdout, stderr bytes.Buffer
	renderer := testRenderer()
	log := zerolog.Nop()
	dir := t.TempDir()

	runner := NewHookRunner(log, &executil.RealExecutor{}, renderer, &stdout, &stderr)

	rule := config.Rule{
		Commands: []string{"echo {{ .Owner }}/{{ .Repo }}"},
	}
	data := config.SpawnTemplateData{
		Owner: "acme",
		Repo:  "myrepo",
		Path:  dir,
	}

	err := runner.RunHooks(context.Background(), rule, dir, data)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "acme/myrepo")
}

func TestRunHooks_InvalidTemplateFails(t *testing.T) {
	var stdout, stderr bytes.Buffer
	renderer := testRenderer()
	log := zerolog.Nop()
	dir := t.TempDir()

	runner := NewHookRunner(log, &executil.RealExecutor{}, renderer, &stdout, &stderr)

	rule := config.Rule{
		Commands: []string{"echo {{ .Unclosed"},
	}

	err := runner.RunHooks(context.Background(), rule, dir, config.SpawnTemplateData{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "render command")
}

func TestRunHooks_EmptyCommandsIsNoop(t *testing.T) {
	var stdout, stderr bytes.Buffer
	renderer := testRenderer()
	log := zerolog.Nop()

	runner := NewHookRunner(log, &executil.RealExecutor{}, renderer, &stdout, &stderr)

	rule := config.Rule{}
	err := runner.RunHooks(context.Background(), rule, t.TempDir(), config.SpawnTemplateData{})
	require.NoError(t, err)
	assert.Empty(t, stdout.String())
}
