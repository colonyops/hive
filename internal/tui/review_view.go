package tui

import (
	"fmt"
	"io"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
)

// ReviewView manages the review interface.
type ReviewView struct {
	list list.Model
}

// NewReviewView creates a new review view.
func NewReviewView(documents []ReviewDocument) ReviewView {
	items := BuildReviewTreeItems(documents)
	delegate := NewReviewTreeDelegate()
	l := list.New(items, delegate, 0, 0)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.SetShowTitle(false)
	l.SetShowFilter(false)
	l.Styles.TitleBar = lipgloss.NewStyle()
	l.Styles.Title = lipgloss.NewStyle()

	// Configure help to match sessions view
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#565f89"))
	l.Help.Styles.ShortKey = helpStyle
	l.Help.Styles.ShortDesc = helpStyle
	l.Help.Styles.ShortSeparator = helpStyle
	l.Help.Styles.FullKey = helpStyle
	l.Help.Styles.FullDesc = helpStyle
	l.Help.Styles.FullSeparator = helpStyle
	l.Help.ShortSeparator = " • "
	l.Styles.HelpStyle = lipgloss.NewStyle().PaddingLeft(1)

	return ReviewView{
		list: l,
	}
}

// SetSize updates the view dimensions.
func (v *ReviewView) SetSize(width, height int) {
	v.list.SetSize(width, height)
}

// Update handles messages.
// The underlying list handles j/k navigation, Enter selection, and / filtering.
func (v ReviewView) Update(msg tea.Msg) (ReviewView, tea.Cmd) {
	var cmd tea.Cmd
	v.list, cmd = v.list.Update(msg)
	return v, cmd
}

// View renders the review view.
func (v ReviewView) View() string {
	return v.list.View()
}

// SelectedDocument returns the currently selected document, or nil if none.
func (v ReviewView) SelectedDocument() *ReviewDocument {
	item := v.list.SelectedItem()
	if item == nil {
		return nil
	}
	if reviewItem, ok := item.(ReviewTreeItem); ok {
		if reviewItem.IsHeader {
			return nil
		}
		return &reviewItem.Document
	}
	return nil
}

// ReviewTreeItem represents an item in the review tree.
type ReviewTreeItem struct {
	IsHeader     bool           // True if this is a document type header
	HeaderName   string         // Document type name (e.g., "Plans", "Research")
	Document     ReviewDocument // The document (when !IsHeader)
	IsLastInType bool           // True if last document in this type group
	CommentCount int            // Number of comments on this document
}

// FilterValue returns the value used for filtering.
func (i ReviewTreeItem) FilterValue() string {
	if i.IsHeader {
		return ""
	}
	return i.Document.RelPath
}

// BuildReviewTreeItems converts documents into tree items grouped by type.
func BuildReviewTreeItems(documents []ReviewDocument) []list.Item {
	if len(documents) == 0 {
		return nil
	}

	items := make([]list.Item, 0)

	// Group documents by type
	groups := make(map[DocumentType][]ReviewDocument)
	for _, doc := range documents {
		groups[doc.Type] = append(groups[doc.Type], doc)
	}

	// Render in order: Plans, Research, Context, Other
	typeOrder := []DocumentType{DocTypePlan, DocTypeResearch, DocTypeContext, DocTypeOther}

	for _, docType := range typeOrder {
		docs, exists := groups[docType]
		if !exists || len(docs) == 0 {
			continue
		}

		// Add header
		header := ReviewTreeItem{
			IsHeader:   true,
			HeaderName: docType.String(),
		}
		items = append(items, header)

		// Add documents
		for idx, doc := range docs {
			isLast := idx == len(docs)-1
			item := ReviewTreeItem{
				IsHeader:     false,
				Document:     doc,
				IsLastInType: isLast,
				CommentCount: 0, // TODO: Load from store in Phase 6
			}
			items = append(items, item)
		}
	}

	return items
}

// ReviewTreeDelegate handles rendering of review tree items.
type ReviewTreeDelegate struct {
	styles ReviewTreeDelegateStyles
}

// ReviewTreeDelegateStyles defines the styles for review tree rendering.
type ReviewTreeDelegateStyles struct {
	HeaderNormal   lipgloss.Style
	HeaderSelected lipgloss.Style
	TreeLine       lipgloss.Style
	DocName        lipgloss.Style
	DocMeta        lipgloss.Style
	Selected       lipgloss.Style
	SelectedBorder lipgloss.Style
}

// DefaultReviewTreeDelegateStyles returns the default styles.
func DefaultReviewTreeDelegateStyles() ReviewTreeDelegateStyles {
	return ReviewTreeDelegateStyles{
		HeaderNormal:   lipgloss.NewStyle().Bold(true).Foreground(colorWhite),
		HeaderSelected: lipgloss.NewStyle().Bold(true).Foreground(colorBlue),
		TreeLine:       lipgloss.NewStyle().Foreground(colorGray),
		DocName:        lipgloss.NewStyle().Foreground(colorWhite),
		DocMeta:        lipgloss.NewStyle().Foreground(colorGray),
		Selected:       lipgloss.NewStyle().Foreground(colorBlue).Bold(true),
		SelectedBorder: lipgloss.NewStyle().Foreground(colorBlue),
	}
}

// NewReviewTreeDelegate creates a new review tree delegate.
func NewReviewTreeDelegate() ReviewTreeDelegate {
	return ReviewTreeDelegate{
		styles: DefaultReviewTreeDelegateStyles(),
	}
}

// Height returns the height of each item.
func (d ReviewTreeDelegate) Height() int {
	return 1
}

// Spacing returns the spacing between items.
func (d ReviewTreeDelegate) Spacing() int {
	return 0
}

// Update handles item updates.
func (d ReviewTreeDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd {
	return nil
}

// Render renders a single review tree item.
func (d ReviewTreeDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	reviewItem, ok := item.(ReviewTreeItem)
	if !ok {
		return
	}

	isSelected := index == m.Index()

	var line string
	if reviewItem.IsHeader {
		line = d.renderHeader(reviewItem, isSelected)
	} else {
		line = d.renderDocument(reviewItem, isSelected)
	}

	// Selection indicator
	var prefix string
	if isSelected {
		prefix = d.styles.SelectedBorder.Render("┃") + " "
	} else {
		prefix = "  "
	}

	_, _ = fmt.Fprintf(w, "%s%s", prefix, line)
}

// renderHeader renders a document type header.
func (d ReviewTreeDelegate) renderHeader(item ReviewTreeItem, isSelected bool) string {
	nameStyle := d.styles.HeaderNormal
	if isSelected {
		nameStyle = d.styles.HeaderSelected
	}
	return nameStyle.Render(item.HeaderName)
}

// renderDocument renders a document entry with tree prefix.
func (d ReviewTreeDelegate) renderDocument(item ReviewTreeItem, isSelected bool) string {
	// Tree prefix
	var prefix string
	if item.IsLastInType {
		prefix = treeLast
	} else {
		prefix = treeBranch
	}
	prefixStyled := d.styles.TreeLine.Render(prefix)

	// Document name
	nameStyle := d.styles.DocName
	if isSelected {
		nameStyle = d.styles.Selected
	}
	name := nameStyle.Render(item.Document.RelPath)

	// Comment count (if any)
	var comments string
	if item.CommentCount > 0 {
		comments = d.styles.DocMeta.Render(fmt.Sprintf(" (%d)", item.CommentCount))
	}

	return fmt.Sprintf("%s %s%s", prefixStyled, name, comments)
}
