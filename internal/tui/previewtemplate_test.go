package tui

import (
	"testing"

	"github.com/hay-kot/hive/internal/core/session"
	"github.com/hay-kot/hive/internal/integration/terminal"
	"github.com/hay-kot/hive/internal/plugins"
	"github.com/hay-kot/hive/pkg/kv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShortID(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"abcdefgh", "efgh"},
		{"abcd", "abcd"},
		{"ab", "ab"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, shortID(tt.input))
		})
	}
}

func TestBuildPreviewTemplateData(t *testing.T) {
	sess := &session.Session{
		ID:   "abc12345",
		Name: "test-session",
		Path: "/tmp/test",
	}

	t.Run("nil stores populate basic fields", func(t *testing.T) {
		data := BuildPreviewTemplateData(sess, nil, nil, nil, true)
		assert.Equal(t, "test-session", data.Name)
		assert.Equal(t, "abc12345", data.ID)
		assert.Equal(t, "2345", data.ShortID)
		assert.Equal(t, "/tmp/test", data.Path)
		assert.Empty(t, data.Branch)
	})

	t.Run("git status populates branch and stats", func(t *testing.T) {
		gitStatuses := kv.New[string, GitStatus]()
		gitStatuses.Set("/tmp/test", GitStatus{
			Branch:     "main",
			Additions:  5,
			Deletions:  3,
			HasChanges: true,
		})

		data := BuildPreviewTemplateData(sess, gitStatuses, nil, nil, true)
		assert.Equal(t, "main", data.Branch)
		assert.Equal(t, 5, data.GitStatus.Additions)
		assert.Equal(t, 3, data.GitStatus.Deletions)
		assert.True(t, data.GitStatus.HasChanges)
	})

	t.Run("loading git status skipped", func(t *testing.T) {
		gitStatuses := kv.New[string, GitStatus]()
		gitStatuses.Set("/tmp/test", GitStatus{IsLoading: true, Branch: "main"})

		data := BuildPreviewTemplateData(sess, gitStatuses, nil, nil, true)
		assert.Empty(t, data.Branch)
	})

	t.Run("plugin statuses populated", func(t *testing.T) {
		pluginStatuses := map[string]*kv.Store[string, plugins.Status]{
			"github": kv.New[string, plugins.Status](),
			"beads":  kv.New[string, plugins.Status](),
			"claude": kv.New[string, plugins.Status](),
		}
		pluginStatuses["github"].Set("abc12345", plugins.Status{Label: "open"})
		pluginStatuses["beads"].Set("abc12345", plugins.Status{Label: "2/5"})
		pluginStatuses["claude"].Set("abc12345", plugins.Status{Label: "45%"})

		data := BuildPreviewTemplateData(sess, nil, pluginStatuses, nil, true)
		assert.Equal(t, "open", data.Plugin.Github)
		assert.Equal(t, "2/5", data.Plugin.Beads)
		assert.Equal(t, "45%", data.Plugin.Claude)
	})

	t.Run("git status with error skipped", func(t *testing.T) {
		gitStatuses := kv.New[string, GitStatus]()
		gitStatuses.Set("/tmp/test", GitStatus{
			Error:  assert.AnError,
			Branch: "main",
		})

		data := BuildPreviewTemplateData(sess, gitStatuses, nil, nil, true)
		assert.Empty(t, data.Branch)
	})

	t.Run("terminal status populated", func(t *testing.T) {
		termStatuses := kv.New[string, TerminalStatus]()
		termStatuses.Set("abc12345", TerminalStatus{Status: terminal.StatusActive})

		data := BuildPreviewTemplateData(sess, nil, nil, termStatuses, true)
		assert.Equal(t, "active", data.TerminalStatus)
	})

	t.Run("icons disabled returns empty icons", func(t *testing.T) {
		data := BuildPreviewTemplateData(sess, nil, nil, nil, false)
		assert.Empty(t, data.Icon.Git)
		assert.Empty(t, data.Icon.GitBranch)
	})
}

func TestParsePreviewTemplates(t *testing.T) {
	t.Run("empty strings use defaults", func(t *testing.T) {
		pt := ParsePreviewTemplates("", "")
		require.NotNil(t, pt.title)
		require.NotNil(t, pt.status)
	})

	t.Run("valid custom templates render correctly", func(t *testing.T) {
		pt := ParsePreviewTemplates("{{ .Name }}", "{{ .Branch }}")
		data := PreviewTemplateData{Name: "foo", Branch: "main"}
		assert.Equal(t, "foo", pt.RenderTitle(data))
		assert.Equal(t, "main", pt.RenderStatus(data))
	})

	t.Run("invalid template falls back to default", func(t *testing.T) {
		pt := ParsePreviewTemplates("{{ .Invalid", "{{ .Invalid")
		require.NotNil(t, pt.title)

		data := PreviewTemplateData{Name: "test", ShortID: "abcd"}
		assert.Contains(t, pt.RenderTitle(data), "test")
	})

	t.Run("nil template returns empty", func(t *testing.T) {
		pt := &PreviewTemplates{}
		assert.Empty(t, pt.RenderTitle(PreviewTemplateData{}))
		assert.Empty(t, pt.RenderStatus(PreviewTemplateData{}))
	})
}

func TestRenderStatus_EmptyData(t *testing.T) {
	pt := ParsePreviewTemplates("", "")
	result := pt.RenderStatus(PreviewTemplateData{})
	assert.Empty(t, result)
}
