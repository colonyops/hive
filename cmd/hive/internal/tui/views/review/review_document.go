package review

import (
	"cmp"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"charm.land/glamour/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/colonyops/hive/cmd/hive/internal/styles"
	"github.com/colonyops/hive/internal/domain/drive"
)

const (
	// maxContentWidth is the maximum line width for content wrapping (markdown and comments).
	maxContentWidth = 120
	// lineNumberReserved is the space reserved for line numbers and separators.
	lineNumberReserved = 6
)

// contentWrapWidth computes the wrap width for a given viewport width.
// Used by both markdown rendering and comment wrapping to ensure consistent line lengths.
func contentWrapWidth(viewportWidth int) int {
	return max(min(viewportWidth-lineNumberReserved, maxContentWidth), 20)
}

// TypeOther is the type of a document with neither a front matter type nor a
// folder to take one from.
const TypeOther = "other"

// Document represents a reviewable file.
type Document struct {
	Path          string // Absolute path
	RelPath       string // relative to the space, "<space key>/<path>" when spaces are mixed
	Type          string // front matter "type", else the top folder name, else "other"
	Space         drive.Space
	inCheckout    bool // reachable from a repository checkout through the context symlink
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

// knownTypes lead every listing, in this order. Other types follow
// alphabetically and TypeOther comes last.
var knownTypes = []string{"plan", "design-doc", "research", "review", "reference", "context"}

func typeRank(t string) int {
	if i := slices.Index(knownTypes, t); i >= 0 {
		return i
	}
	if t == TypeOther {
		return len(knownTypes) + 1
	}
	return len(knownTypes)
}

func compareTypes(a, b string) int {
	if c := cmp.Compare(typeRank(a), typeRank(b)); c != 0 {
		return c
	}
	return strings.Compare(a, b)
}

// typeTitle is the group header of a type: "design-doc" reads "Design doc".
func typeTitle(t string) string {
	if t == "" {
		t = TypeOther
	}
	t = strings.ReplaceAll(t, "-", " ")
	r, size := utf8.DecodeRuneInString(t)
	return string(unicode.ToUpper(r)) + t[size:]
}

// folderTypes maps the plural folder names agents write to the singular types
// front matter uses, so a plan in plans/ groups with one typed "plan".
var folderTypes = map[string]string{
	"plans":       "plan",
	"design-docs": "design-doc",
	"reviews":     "review",
	"references":  "reference",
}

// typeFromPath is the type of a document that has no front matter type: its
// top folder.
func typeFromPath(relPath string) string {
	relPath = filepath.ToSlash(relPath)
	if top, _, ok := strings.Cut(relPath, "/"); ok && top != "" {
		top = strings.ToLower(top)
		if t, ok := folderTypes[top]; ok {
			return t
		}
		return top
	}
	return TypeOther
}

func documentType(entry drive.Entry) string {
	if t, ok := entry.Front["type"].(string); ok {
		if t = strings.ToLower(strings.TrimSpace(t)); t != "" {
			return t
		}
	}
	return typeFromPath(entry.Path)
}

// isPlainText reports whether a file entry is listed and shown as plain text.
func isPlainText(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".txt")
}

// DocumentsFromTree lists the markdown and plain-text entries of a space's tree, sorted by
// type and then newest first. dir is the space's directory on disk. With
// qualified, RelPath names the space, so documents of several spaces stay
// apart.
func DocumentsFromTree(space drive.Space, dir string, root drive.Entry, qualified bool) []Document {
	var docs []Document
	collectDocuments(&docs, space, dir, root, qualified)
	SortDocuments(docs)
	return docs
}

func collectDocuments(docs *[]Document, space drive.Space, dir string, entry drive.Entry, qualified bool) {
	for _, child := range entry.Children {
		if child.Kind == drive.KindDir {
			collectDocuments(docs, space, dir, child, qualified)
			continue
		}
		if child.Kind != drive.KindMarkdown && (child.Kind != drive.KindFile || !isPlainText(child.Path)) {
			continue
		}
		doc := spaceDocument(space, dir, child.Path, qualified)
		doc.Type = documentType(child)
		doc.ModTime = child.Modified
		*docs = append(*docs, doc)
	}
}

// spaceDocument is the document at path, relative to the space directory
// dir, with its RelPath named the way the space's listing names it.
func spaceDocument(space drive.Space, dir, path string, qualified bool) Document {
	rel := path
	if qualified {
		rel = space.Key + "/" + path
	}
	return Document{
		Path:       filepath.Join(dir, filepath.FromSlash(path)),
		RelPath:    rel,
		Space:      space,
		inCheckout: !qualified && space.Kind == drive.SpaceKindRepository,
	}
}

// CheckoutPath is the path an agent in a repository checkout reaches the
// document by: RelPath under the context symlink, named symlinkName. A
// document outside a repository space has no such path and keeps RelPath.
func (d Document) CheckoutPath(symlinkName string) string {
	if !d.inCheckout || symlinkName == "" {
		return d.RelPath
	}
	return symlinkName + "/" + d.RelPath
}

// SortDocuments orders documents by type, then newest first.
func SortDocuments(docs []Document) {
	slices.SortStableFunc(docs, func(a, b Document) int {
		if c := compareTypes(a.Type, b.Type); c != 0 {
			return c
		}
		if c := b.ModTime.Compare(a.ModTime); c != 0 {
			return c
		}
		return strings.Compare(a.RelPath, b.RelPath)
	})
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

// Render renders the document with line numbers: markdown through Glamour, a
// .txt file as wrapped plain text.
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

	wrapWidth := contentWrapWidth(width)
	if isPlainText(d.Path) {
		d.RenderedLines = strings.Split(ansi.Wrap(strings.TrimRight(d.Content, "\n"), wrapWidth, ""), "\n")
		d.cachedWidth = width
		return d.formatWithLineNumbers(d.RenderedLines), nil
	}

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
