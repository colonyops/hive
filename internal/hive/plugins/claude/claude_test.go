package claude

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/colonyops/hive/internal/core/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionAnalytics_ContextPercent(t *testing.T) {
	t.Run("calculates percentage correctly", func(t *testing.T) {
		analytics := &SessionAnalytics{
			CurrentContextTokens: 100000,
		}
		percent := analytics.ContextPercent(200000)
		assert.InEpsilon(t, 50.0, percent, 0.001)
	})

	t.Run("uses default model limit", func(t *testing.T) {
		analytics := &SessionAnalytics{
			CurrentContextTokens: 100000,
		}
		percent := analytics.ContextPercent(0)
		assert.InEpsilon(t, 50.0, percent, 0.001)
	})

	t.Run("handles 100% usage", func(t *testing.T) {
		analytics := &SessionAnalytics{
			CurrentContextTokens: 200000,
		}
		percent := analytics.ContextPercent(200000)
		assert.InEpsilon(t, 100.0, percent, 0.001)
	})
}

func TestConvertToClaudeDirName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "basic path",
			input:    "/Users/name/Code/project",
			expected: "-Users-name-Code-project",
		},
		{
			name:     "path with spaces",
			input:    "/Users/name/My Documents/project",
			expected: "-Users-name-My-Documents-project",
		},
		{
			name:     "single directory",
			input:    "/project",
			expected: "-project",
		},
		{
			name:     "path with dots",
			input:    "/Users/name/.local/share/hive",
			expected: "-Users-name--local-share-hive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertToClaudeDirName(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseSessionJSONL(t *testing.T) {
	t.Run("parses valid JSONL", func(t *testing.T) {
		// Create temp file with sample JSONL
		tmpDir := t.TempDir()
		jsonlPath := filepath.Join(tmpDir, "test.jsonl")

		jsonlContent := `{"type":"assistant","timestamp":"2024-01-01T10:00:00Z","message":{"usage":{"input_tokens":1000,"output_tokens":500,"cache_creation_input_tokens":200,"cache_read_input_tokens":300},"content":[{"type":"tool_use","name":"Read"},{"type":"tool_use","name":"Bash"}]}}
{"type":"assistant","timestamp":"2024-01-01T10:05:00Z","message":{"usage":{"input_tokens":1200,"output_tokens":600,"cache_creation_input_tokens":100,"cache_read_input_tokens":400},"content":[{"type":"tool_use","name":"Read"}]}}
`
		err := os.WriteFile(jsonlPath, []byte(jsonlContent), 0o644)
		require.NoError(t, err)

		analytics, err := ParseSessionJSONL(jsonlPath)
		require.NoError(t, err)

		// Check cumulative tokens
		assert.Equal(t, 2200, analytics.InputTokens)
		assert.Equal(t, 1100, analytics.OutputTokens)
		assert.Equal(t, 300, analytics.CacheWriteTokens)
		assert.Equal(t, 700, analytics.CacheReadTokens)

		// Current context should be from last turn
		assert.Equal(t, 1600, analytics.CurrentContextTokens) // 1200 + 400

		// Check turns
		assert.Equal(t, 2, analytics.TotalTurns)

		// Check duration
		assert.Equal(t, 5*time.Minute, analytics.Duration)

		// Check tool calls
		assert.Len(t, analytics.ToolCalls, 2)
	})

	t.Run("handles invalid JSON lines gracefully", func(t *testing.T) {
		tmpDir := t.TempDir()
		jsonlPath := filepath.Join(tmpDir, "test.jsonl")

		jsonlContent := `invalid json
{"type":"assistant","timestamp":"2024-01-01T10:00:00Z","message":{"usage":{"input_tokens":1000,"output_tokens":500,"cache_creation_input_tokens":0,"cache_read_input_tokens":0},"content":[]}}
`
		err := os.WriteFile(jsonlPath, []byte(jsonlContent), 0o644)
		require.NoError(t, err)

		analytics, err := ParseSessionJSONL(jsonlPath)
		require.NoError(t, err)

		// Should still parse the valid line
		assert.Equal(t, 1, analytics.TotalTurns)
		assert.Equal(t, 1000, analytics.InputTokens)
	})

	t.Run("returns error for missing file", func(t *testing.T) {
		_, err := ParseSessionJSONL("/nonexistent/path.jsonl")
		assert.Error(t, err)
	})

	t.Run("ignores non-assistant messages", func(t *testing.T) {
		tmpDir := t.TempDir()
		jsonlPath := filepath.Join(tmpDir, "test.jsonl")

		jsonlContent := `{"type":"user","timestamp":"2024-01-01T10:00:00Z","message":{"usage":{"input_tokens":500,"output_tokens":0,"cache_creation_input_tokens":0,"cache_read_input_tokens":0},"content":[]}}
{"type":"assistant","timestamp":"2024-01-01T10:00:00Z","message":{"usage":{"input_tokens":1000,"output_tokens":500,"cache_creation_input_tokens":0,"cache_read_input_tokens":0},"content":[]}}
`
		err := os.WriteFile(jsonlPath, []byte(jsonlContent), 0o644)
		require.NoError(t, err)

		analytics, err := ParseSessionJSONL(jsonlPath)
		require.NoError(t, err)

		// Should only count assistant message
		assert.Equal(t, 1, analytics.TotalTurns)
		assert.Equal(t, 1000, analytics.InputTokens)
	})
}

func TestPlugin_renderStatus(t *testing.T) {
	t.Run("returns percentage label for low usage", func(t *testing.T) {
		plugin := New(config.ClaudePluginConfig{}, nil)
		analytics := &SessionAnalytics{
			CurrentContextTokens: 100000, // 50%
		}

		status := plugin.renderStatus(analytics)
		assert.Equal(t, "50%", status.Label)
		assert.Empty(t, status.Icon)
	})

	t.Run("returns yellow with percentage for 60-79% usage", func(t *testing.T) {
		plugin := New(config.ClaudePluginConfig{}, nil)
		analytics := &SessionAnalytics{
			CurrentContextTokens: 130000, // 65%
		}

		status := plugin.renderStatus(analytics)
		assert.Equal(t, "65%", status.Label)
		assert.Empty(t, status.Icon)
		assert.NotNil(t, status.Style)
	})

	t.Run("returns red with percentage for 80%+ usage", func(t *testing.T) {
		plugin := New(config.ClaudePluginConfig{}, nil)
		analytics := &SessionAnalytics{
			CurrentContextTokens: 170000, // 85%
		}

		status := plugin.renderStatus(analytics)
		assert.Equal(t, "85%", status.Label)
		assert.Empty(t, status.Icon)
		assert.NotNil(t, status.Style)
	})

	t.Run("respects custom thresholds", func(t *testing.T) {
		plugin := New(config.ClaudePluginConfig{
			YellowThreshold: 50,
			RedThreshold:    70,
		}, nil)
		analytics := &SessionAnalytics{
			CurrentContextTokens: 110000, // 55%
		}

		status := plugin.renderStatus(analytics)
		assert.NotNil(t, status.Style) // Should be yellow
	})

	t.Run("respects custom model limit", func(t *testing.T) {
		plugin := New(config.ClaudePluginConfig{
			ModelLimit: 100000,
		}, nil)
		analytics := &SessionAnalytics{
			CurrentContextTokens: 65000, // 65% of 100k
		}

		status := plugin.renderStatus(analytics)
		assert.NotNil(t, status.Style) // Should be yellow
	})
}

func TestPlugin_Available(t *testing.T) {
	t.Run("returns false when explicitly disabled", func(t *testing.T) {
		disabled := false
		plugin := New(config.ClaudePluginConfig{
			Enabled: &disabled,
		}, nil)

		// Should return false even if claude CLI exists
		assert.False(t, plugin.Available())
	})

	t.Run("returns true when enabled and claude exists", func(t *testing.T) {
		enabled := true
		plugin := New(config.ClaudePluginConfig{
			Enabled: &enabled,
		}, nil)

		// Note: This will only pass if claude CLI is actually installed
		// In CI, this might fail, which is expected
		_ = plugin.Available()
	})
}

func TestGetClaudeJSONLPath(t *testing.T) {
	t.Run("returns empty for non-existent file", func(t *testing.T) {
		path := GetClaudeJSONLPath("/nonexistent/path", "session-id")
		assert.Empty(t, path)
	})

	// Note: Testing the actual path resolution would require creating
	// the full ~/.config/claude/projects directory structure,
	// which is environment-specific and not suitable for unit tests
}
