package claude

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// SessionAnalytics represents parsed Claude session data.
type SessionAnalytics struct {
	CurrentContextTokens int           // Last turn's input + cache read
	InputTokens          int           // Cumulative input
	OutputTokens         int           // Cumulative output
	CacheReadTokens      int           // Cumulative cache reads
	CacheWriteTokens     int           // Cumulative cache writes
	TotalTurns           int           // Number of turns
	Duration             time.Duration // Session duration
	ToolCalls            []ToolCall    // Tool usage counts
}

// ToolCall represents a tool and its usage count.
type ToolCall struct {
	Name  string
	Count int
}

type jsonlEntry struct {
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	Message   struct {
		Usage struct {
			InputTokens              int `json:"input_tokens"`
			OutputTokens             int `json:"output_tokens"`
			CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
			CacheReadInputTokens     int `json:"cache_read_input_tokens"`
		} `json:"usage"`
		Content []struct {
			Type string `json:"type"`
			Name string `json:"name"`
		} `json:"content"`
	} `json:"message"`
}

// GetClaudeJSONLPath resolves the JSONL file path for a Claude session.
func GetClaudeJSONLPath(projectPath, claudeSessionID string) string {
	configDir := os.ExpandEnv("$HOME/.config/claude")

	// Resolve symlinks (macOS /tmp -> /private/tmp)
	resolvedPath, _ := filepath.EvalSymlinks(projectPath)

	// Convert to Claude directory naming
	// "/Users/name/Code/project" -> "-Users-name-Code-project"
	projectDir := convertToClaudeDirName(resolvedPath)

	sessionFile := filepath.Join(
		configDir,
		"projects",
		projectDir,
		claudeSessionID+".jsonl")

	// Verify exists
	if _, err := os.Stat(sessionFile); err != nil {
		return ""
	}

	return sessionFile
}

// ParseSessionJSONL parses a Claude JSONL file and extracts analytics.
func ParseSessionJSONL(path string) (*SessionAnalytics, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	analytics := &SessionAnalytics{
		ToolCalls: []ToolCall{},
	}

	scanner := bufio.NewScanner(file)
	toolCounts := make(map[string]int)
	var firstTime, lastTime time.Time

	for scanner.Scan() {
		var entry jsonlEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue // Skip invalid lines
		}

		// Only process assistant messages
		if entry.Type != "assistant" {
			continue
		}

		// Track timing
		if firstTime.IsZero() || entry.Timestamp.Before(firstTime) {
			firstTime = entry.Timestamp
		}
		lastTime = entry.Timestamp

		// Accumulate tokens (cumulative for cost)
		analytics.InputTokens += entry.Message.Usage.InputTokens
		analytics.OutputTokens += entry.Message.Usage.OutputTokens
		analytics.CacheReadTokens += entry.Message.Usage.CacheReadInputTokens
		analytics.CacheWriteTokens += entry.Message.Usage.CacheCreationInputTokens

		// Current context = last turn only (overwrites previous)
		analytics.CurrentContextTokens = entry.Message.Usage.InputTokens +
			entry.Message.Usage.CacheReadInputTokens

		analytics.TotalTurns++

		// Count tool calls
		for _, content := range entry.Message.Content {
			if content.Type == "tool_use" && content.Name != "" {
				toolCounts[content.Name]++
			}
		}
	}

	// Convert tool counts to slice
	for name, count := range toolCounts {
		analytics.ToolCalls = append(analytics.ToolCalls,
			ToolCall{Name: name, Count: count})
	}

	analytics.Duration = lastTime.Sub(firstTime)

	return analytics, scanner.Err()
}

// ContextPercent calculates context usage as percentage of model limit.
func (a *SessionAnalytics) ContextPercent(modelLimit int) float64 {
	if modelLimit == 0 {
		modelLimit = 200000
	}
	return float64(a.CurrentContextTokens) / float64(modelLimit) * 100
}

// convertToClaudeDirName converts a path to Claude's directory naming format.
func convertToClaudeDirName(path string) string {
	// Claude converts: /Users/name/Code -> -Users-name-Code
	result := strings.ReplaceAll(path, "/", "-")
	result = strings.TrimPrefix(result, "-")
	// Handle special chars
	result = strings.ReplaceAll(result, " ", "-")
	return result
}
