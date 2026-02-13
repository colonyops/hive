package review

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/glamour"
	"github.com/hay-kot/hive/internal/core/styles"
)

// DocumentType categorizes documents.
// ENUM(plan, research, context, other).
type DocumentType string

// DisplayName returns the capitalized display name for UI rendering.
func (t DocumentType) DisplayName() string {
	switch t {
	case DocumentTypePlan:
		return "Plan"
	case DocumentTypeResearch:
		return "Research"
	case DocumentTypeContext:
		return "Context"
	case DocumentTypeOther:
		return "Other"
	default:
		return "Unknown"
	}
}

// priority returns the sort priority (lower values sort first).
func (t DocumentType) priority() int {
	switch t {
	case DocumentTypePlan:
		return 0
	case DocumentTypeResearch:
		return 1
	case DocumentTypeContext:
		return 2
	case DocumentTypeOther:
		return 3
	default:
		return 999
	}
}

// Document represents a reviewable file.
type Document struct {
	Path          string       // Absolute path
	RelPath       string       // Relative to repo (e.g., ".hive/plans/...")
	Type          DocumentType // Plan, Research, Context, Other
	ModTime       time.Time
	Content       string   // Raw content
	RenderedLines []string // Glamour-rendered lines with ANSI (cached)
	cachedWidth   int      // Width used for cached rendering
}

// Comment represents inline feedback.
type Comment struct {
	ID          string // UUID
	SessionID   string // Associated session ID
	StartLine   int    // 1-indexed line number
	EndLine     int    // Inclusive
	ContextText string // Quoted text from document
	CommentText string // User's feedback
	CreatedAt   time.Time
}

// Session holds state for active review.
type Session struct {
	ID         string
	DocPath    string
	Comments   []Comment
	CreatedAt  time.Time
	ModifiedAt time.Time
}

// DiscoverDocuments walks the actual context directory and returns categorized documents.
// It uses the context directory path directly, avoiding symlink issues.
// Returns documents sorted by type, then by modification time (newest first).
func DiscoverDocuments(contextDir string) ([]Document, error) {
	// Check if context directory exists
	if _, err := os.Stat(contextDir); os.IsNotExist(err) {
		return []Document{}, nil
	}

	var docs []Document

	// Walk directory tree
	err := filepath.WalkDir(contextDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		// Only include markdown and text files
		ext := filepath.Ext(path)
		if ext != ".md" && ext != ".txt" {
			return nil
		}

		// Get relative path from context directory root
		relPath, err := filepath.Rel(contextDir, path)
		if err != nil {
			return err
		}

		// Infer document type from path
		docType := inferDocumentType(relPath)

		// Get modification time
		info, err := d.Info()
		if err != nil {
			return err
		}

		docs = append(docs, Document{
			Path:    path,
			RelPath: relPath,
			Type:    docType,
			ModTime: info.ModTime(),
		})

		return nil
	})
	if err != nil {
		return nil, err
	}

	// Sort by type, then by modification time (newest first within each type)
	sortDocuments(docs)

	return docs, nil
}

// inferDocumentType determines the document type based on its path.
// Expects relPath to be relative to the context directory root.
func inferDocumentType(relPath string) DocumentType {
	// Normalize path separators
	relPath = filepath.ToSlash(relPath)

	// Check the top-level directory name
	parts := strings.Split(relPath, "/")
	if len(parts) > 0 {
		switch parts[0] {
		case "plans":
			return DocumentTypePlan
		case "research":
			return DocumentTypeResearch
		case "context":
			return DocumentTypeContext
		}
	}

	return DocumentTypeOther
}

// sortDocuments sorts documents by type, then by modification time (newest first).
func sortDocuments(docs []Document) {
	// Use simple bubble sort for small collections
	// Type priority: Plans > Research > Context > Other
	for i := range docs {
		for j := i + 1; j < len(docs); j++ {
			// Compare types first using priority
			if docs[i].Type.priority() > docs[j].Type.priority() {
				docs[i], docs[j] = docs[j], docs[i]
				continue
			}
			// If same type, sort by modification time (newest first)
			if docs[i].Type == docs[j].Type && docs[i].ModTime.Before(docs[j].ModTime) {
				docs[i], docs[j] = docs[j], docs[i]
			}
		}
	}
}

// LoadContent reads the document content from disk.
func (d *Document) LoadContent() error {
	content, err := os.ReadFile(d.Path)
	if err != nil {
		return err
	}
	d.Content = string(content)
	d.RenderedLines = nil // Clear cache
	return nil
}

// Render renders the document content using Glamour with line numbers.
// Returns a string with ANSI-styled markdown and line numbers.
func (d *Document) Render(width int) (string, error) {
	// Use cached rendered lines if available and width matches
	if d.RenderedLines != nil && d.cachedWidth == width {
		return d.formatWithLineNumbers(d.RenderedLines), nil
	}

	// Load content if not already loaded
	if d.Content == "" {
		if err := d.LoadContent(); err != nil {
			return "", err
		}
	}

	// Create glamour renderer with Tokyo Night theme
	// Account for line numbers (4 chars) + separator (2 chars)
	// Ensure minimum reasonable width of 20 characters, maximum of 120 for readability
	wrapWidth := max(min(width-6, 120), 20)
	r, err := glamour.NewTermRenderer(
		glamour.WithStyles(styles.GlamourStyle()),
		glamour.WithWordWrap(wrapWidth),
	)
	if err != nil {
		return "", err
	}

	// Render markdown
	rendered, err := r.Render(d.Content)
	if err != nil {
		return "", err
	}

	// Split into lines and cache with width
	d.RenderedLines = strings.Split(strings.TrimRight(rendered, "\n"), "\n")
	d.cachedWidth = width

	return d.formatWithLineNumbers(d.RenderedLines), nil
}

// formatWithLineNumbers adds line numbers to rendered content.
func (d *Document) formatWithLineNumbers(lines []string) string {
	if len(lines) == 0 {
		return ""
	}

	// Calculate max line number width
	maxLineNum := len(lines)
	lineNumWidth := len(fmt.Sprintf("%d", maxLineNum))

	// Build output with line numbers
	var result strings.Builder
	lineNumStyle := styles.TextMutedStyle
	separatorStyle := styles.TextMutedStyle

	for i, line := range lines {
		lineNum := fmt.Sprintf("%*d", lineNumWidth, i+1)
		styledNum := lineNumStyle.Render(lineNum)
		leftSep := separatorStyle.Render(" ")
		rightSep := separatorStyle.Render("  ")
		result.WriteString(leftSep + styledNum + rightSep + line + "\n")
	}

	return result.String()
}
