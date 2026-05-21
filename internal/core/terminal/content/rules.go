package content

import (
	"regexp"
	"strings"
)

type rule struct {
	Category string
	Weight   int
	Match    func(lines []string) (pattern string, ok bool)
}

var (
	spinnerElapsedRE = regexp.MustCompile(`[⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏✳✽✶✢].*(\d+s|tokens?)`)
	promptGlyphRE    = regexp.MustCompile(`^(❯|>)\s*(Try .*)?$`)
	assistantGlyphRE = regexp.MustCompile(`^[●⎿✦◆]\s+`)
	toolCallRowRE    = regexp.MustCompile(`^(Read|Edit|Bash|Write|Grep|Glob|LS|TodoWrite|MultiEdit)\(`)
	shellPromptRE    = regexp.MustCompile(`(^|\s)([[:alnum:]_.-]+@[^\s:]+:|[$#%])\s*$`)
	ircChatRE        = regexp.MustCompile(`^\[[0-2][0-9]:[0-5][0-9]\]\s+<[^>]+>`)
	goTestLineRE     = regexp.MustCompile(`^(---\s+(PASS|FAIL):|ok\s+\S+|FAIL\s+\S+)`)
)

func defaultPositiveRules() []rule {
	return []rule{
		{Category: "interrupt_hint", Weight: 3, Match: containsAny("ctrl+c to interrupt", "esc to interrupt")},
		{Category: "approval_prompt", Weight: 3, Match: containsAny("Yes, allow", "No, and tell Claude", "Press enter to confirm or esc to cancel", "Would you like to run the following command?", "Run this command?", "(Y/n)", "[Y/n]")},
		{Category: "spinner_elapsed", Weight: 3, Match: matchRegexp(spinnerElapsedRE)},
		{Category: "prompt_glyph", Weight: 3, Match: promptGlyph},
		{Category: "assistant_glyph", Weight: 2, Match: matchRegexp(assistantGlyphRE)},
		{Category: "tool_call_row", Weight: 2, Match: matchRegexp(toolCallRowRE)},
		{Category: "markdown_fence", Weight: 1, Match: containsAny("```")},
		{Category: "markdown_header", Weight: 1, Match: matchLinePrefix("## ")},
	}
}

func defaultNegativeRules() []rule {
	return []rule{
		{Category: "shell_prompt", Weight: -3, Match: repeatedShellPrompt},
		{Category: "repl_banner", Weight: -2, Match: containsAny("Python 3.", "Node.js v", "Welcome to Node.js", "(gdb)", "irb(main)")},
		{Category: "pager_indicator", Weight: -2, Match: containsAny("(END)", "Manual page", "lines 1-", "Lines 1-")},
		{Category: "irc_chat", Weight: -2, Match: repeatedRegexp(ircChatRE, 2)},
		{Category: "build_log", Weight: -1, Match: buildLog},
		{Category: "package_prompt", Weight: -2, Match: containsAny("npm init", "pip install", "brew install", "package name:", "Is this OK?", "Proceed? [Y/n]")},
	}
}

func containsAny(patterns ...string) func([]string) (string, bool) {
	return func(lines []string) (string, bool) {
		for _, line := range lines {
			lowerLine := strings.ToLower(line)
			for _, pattern := range patterns {
				if strings.Contains(lowerLine, strings.ToLower(pattern)) {
					return pattern, true
				}
			}
		}
		return "", false
	}
}

func matchRegexp(re *regexp.Regexp) func([]string) (string, bool) {
	return func(lines []string) (string, bool) {
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if re.MatchString(trimmed) {
				return re.String(), true
			}
		}
		return "", false
	}
}

func repeatedRegexp(re *regexp.Regexp, threshold int) func([]string) (string, bool) {
	return func(lines []string) (string, bool) {
		count := 0
		for _, line := range lines {
			if re.MatchString(strings.TrimSpace(line)) {
				count++
			}
		}
		return re.String(), count >= threshold
	}
}

func matchLinePrefix(prefix string) func([]string) (string, bool) {
	return func(lines []string) (string, bool) {
		for _, line := range lines {
			if strings.HasPrefix(strings.TrimSpace(line), prefix) {
				return prefix, true
			}
		}
		return "", false
	}
}

func promptGlyph(lines []string) (string, bool) {
	start := len(lines) - 5
	if start < 0 {
		start = 0
	}
	for _, line := range lines[start:] {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, "@") || strings.Contains(trimmed, ":~") || strings.HasPrefix(trimmed, ">>>") || strings.HasPrefix(trimmed, "...") {
			continue
		}
		if promptGlyphRE.MatchString(trimmed) {
			return promptGlyphRE.String(), true
		}
	}
	return "", false
}

func repeatedShellPrompt(lines []string) (string, bool) {
	count := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "❯") || strings.HasPrefix(trimmed, ">>>") || strings.HasPrefix(trimmed, "...") {
			continue
		}
		if shellPromptRE.MatchString(trimmed) || strings.Contains(trimmed, "user@host") {
			count++
		}
	}
	return shellPromptRE.String(), count >= 2
}

func buildLog(lines []string) (string, bool) {
	count := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if goTestLineRE.MatchString(trimmed) || trimmed == "PASS" || trimmed == "FAIL" || strings.HasPrefix(trimmed, "make[") {
			count++
		}
	}
	return goTestLineRE.String(), count >= 3
}
