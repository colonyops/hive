package diff

import (
	"fmt"
	"strconv"
	"strings"
)

// LineType represents the type of a line in a unified diff.
type LineType int

const (
	LineTypeContext LineType = iota // Context line (starts with space)
	LineTypeAdd                     // Addition line (starts with +)
	LineTypeDelete                  // Deletion line (starts with -)
	LineTypeHunk                    // Hunk header (@@ ... @@)
	LineTypeFileHeader              // File header (--- or +++)
)

// ParsedLine represents a single parsed line from a unified diff with its metadata.
type ParsedLine struct {
	Type       LineType // Type of the line
	Content    string   // Raw content (without leading +/- for add/delete)
	OldLineNum int      // Line number in old file (0 if not applicable)
	NewLineNum int      // Line number in new file (0 if not applicable)
	RawLine    string   // Original line including +/- prefix
}

// HunkHeader represents the metadata from a hunk header line.
type HunkHeader struct {
	OldStart int    // Starting line number in old file
	OldCount int    // Number of lines in old file
	NewStart int    // Starting line number in new file
	NewCount int    // Number of lines in new file
	Comment  string // Optional comment after @@
}

// ParseDiffLines parses a unified diff string into a slice of ParsedLine.
// It tracks line numbers for both old and new files, handling add/delete/context lines.
func ParseDiffLines(diff string) ([]ParsedLine, error) {
	lines := strings.Split(diff, "\n")
	var result []ParsedLine

	var currentOldLine, currentNewLine int
	var inHunk bool

	for _, line := range lines {
		if line == "" {
			continue
		}

		// File headers (--- and +++)
		if strings.HasPrefix(line, "---") || strings.HasPrefix(line, "+++") {
			result = append(result, ParsedLine{
				Type:    LineTypeFileHeader,
				Content: line,
				RawLine: line,
			})
			continue
		}

		// Hunk header (@@ -old +new @@)
		if strings.HasPrefix(line, "@@") {
			hunk, err := parseHunkHeader(line)
			if err != nil {
				return nil, fmt.Errorf("parse hunk header: %w", err)
			}

			result = append(result, ParsedLine{
				Type:       LineTypeHunk,
				Content:    line,
				OldLineNum: hunk.OldStart,
				NewLineNum: hunk.NewStart,
				RawLine:    line,
			})

			// Set current line numbers from hunk header
			currentOldLine = hunk.OldStart
			currentNewLine = hunk.NewStart
			inHunk = true
			continue
		}

		// Must be in a hunk to process diff lines
		if !inHunk {
			continue
		}

		// Parse diff lines based on first character
		if len(line) == 0 {
			continue
		}

		prefix := line[0]
		content := line[1:]

		switch prefix {
		case '+':
			// Addition: only in new file
			result = append(result, ParsedLine{
				Type:       LineTypeAdd,
				Content:    content,
				OldLineNum: 0,
				NewLineNum: currentNewLine,
				RawLine:    line,
			})
			currentNewLine++

		case '-':
			// Deletion: only in old file
			result = append(result, ParsedLine{
				Type:       LineTypeDelete,
				Content:    content,
				OldLineNum: currentOldLine,
				NewLineNum: 0,
				RawLine:    line,
			})
			currentOldLine++

		case ' ':
			// Context: in both files
			result = append(result, ParsedLine{
				Type:       LineTypeContext,
				Content:    content,
				OldLineNum: currentOldLine,
				NewLineNum: currentNewLine,
				RawLine:    line,
			})
			currentOldLine++
			currentNewLine++

		default:
			// Skip unknown line types
			continue
		}
	}

	return result, nil
}

// parseHunkHeader parses a hunk header line like "@@ -1,7 +1,8 @@ function_name"
// Returns the parsed metadata or an error if the format is invalid.
func parseHunkHeader(line string) (HunkHeader, error) {
	// Find the @@ markers
	if !strings.HasPrefix(line, "@@") {
		return HunkHeader{}, fmt.Errorf("invalid hunk header: missing @@ prefix")
	}

	// Find the closing @@
	closeIdx := strings.Index(line[2:], "@@")
	if closeIdx == -1 {
		return HunkHeader{}, fmt.Errorf("invalid hunk header: missing closing @@")
	}
	closeIdx += 2

	// Extract the range part between @@ markers
	rangeStr := strings.TrimSpace(line[2:closeIdx])

	// Extract optional comment after closing @@
	comment := ""
	if closeIdx+2 < len(line) {
		comment = strings.TrimSpace(line[closeIdx+2:])
	}

	// Parse "-old +new" ranges
	parts := strings.Fields(rangeStr)
	if len(parts) != 2 {
		return HunkHeader{}, fmt.Errorf("invalid hunk header: expected 2 ranges, got %d", len(parts))
	}

	oldRange := parts[0]
	newRange := parts[1]

	if !strings.HasPrefix(oldRange, "-") {
		return HunkHeader{}, fmt.Errorf("invalid hunk header: old range missing - prefix")
	}
	if !strings.HasPrefix(newRange, "+") {
		return HunkHeader{}, fmt.Errorf("invalid hunk header: new range missing + prefix")
	}

	oldStart, oldCount, err := parseRange(oldRange[1:])
	if err != nil {
		return HunkHeader{}, fmt.Errorf("parse old range: %w", err)
	}

	newStart, newCount, err := parseRange(newRange[1:])
	if err != nil {
		return HunkHeader{}, fmt.Errorf("parse new range: %w", err)
	}

	return HunkHeader{
		OldStart: oldStart,
		OldCount: oldCount,
		NewStart: newStart,
		NewCount: newCount,
		Comment:  comment,
	}, nil
}

// parseRange parses a range string like "1,7" or "1" into start line and count.
// Single numbers (like "1") are treated as "1,1".
func parseRange(rangeStr string) (start, count int, err error) {
	parts := strings.Split(rangeStr, ",")

	start, err = strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("parse start: %w", err)
	}

	if len(parts) == 1 {
		// Single line (e.g., "1" means "1,1")
		count = 1
	} else if len(parts) == 2 {
		count, err = strconv.Atoi(parts[1])
		if err != nil {
			return 0, 0, fmt.Errorf("parse count: %w", err)
		}
	} else {
		return 0, 0, fmt.Errorf("invalid range format: %s", rangeStr)
	}

	return start, count, nil
}

// FilterByType returns only the lines matching the given type.
func FilterByType(lines []ParsedLine, typ LineType) []ParsedLine {
	var filtered []ParsedLine
	for _, line := range lines {
		if line.Type == typ {
			filtered = append(filtered, line)
		}
	}
	return filtered
}

// GetLineAtOffset returns the parsed line at the given raw line offset (0-indexed).
// Returns nil if offset is out of bounds.
func GetLineAtOffset(lines []ParsedLine, offset int) *ParsedLine {
	if offset < 0 || offset >= len(lines) {
		return nil
	}
	return &lines[offset]
}
