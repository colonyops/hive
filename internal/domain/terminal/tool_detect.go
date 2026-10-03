package terminal

import "strings"

// DetectTool attempts to identify the AI tool from terminal content.
func DetectTool(content string) string {
	if looksLikeAiderContent(content) {
		return "aider"
	}
	if looksLikePiContent(content) {
		return "pi"
	}

	lower := strings.ToLower(content)

	for _, p := range toolPatterns {
		for _, keyword := range p.keywords {
			if strings.Contains(lower, keyword) {
				return p.tool
			}
		}
	}

	return "shell"
}

// toolPatterns is ordered: specific tools first, so content matching multiple
// keywords resolves deterministically; the generic "agent" keyword is last
// because it substring-matches almost anything.
var toolPatterns = []struct {
	tool     string
	keywords []string
}{
	{"claude", []string{"claude", "anthropic", "ctrl+c to interrupt"}},
	{"codex", []string{"codex", "openai"}},
	{"gemini", []string{"gemini", "google ai"}},
	{"opencode", []string{"opencode", "open code"}},
	{"cursor", []string{"cursor"}},
	{"crush", []string{"crush"}},
	{"agent", []string{"agent"}},
}

func looksLikeAiderContent(content string) bool {
	for _, line := range strings.Split(content, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) == 0 {
			continue
		}
		if len(fields) < 2 || !strings.EqualFold(fields[0], "Aider") {
			return false
		}
		version := strings.TrimPrefix(strings.ToLower(fields[1]), "v")
		return version != "" && version[0] >= '0' && version[0] <= '9'
	}
	return false
}

func looksLikePiContent(content string) bool {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		return strings.HasPrefix(trimmed, "π - ") || strings.EqualFold(trimmed, "pi") || strings.HasPrefix(strings.ToLower(trimmed), "pi ")
	}
	return false
}
