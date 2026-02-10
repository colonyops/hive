// Package review implements a TUI for reviewing and commenting on markdown documents.
//
// # Line Coordinate System
//
// This package uses two coordinate systems:
//
// 1. Document coordinates: Line numbers in the original markdown document (1-indexed)
// 2. Display coordinates: Line numbers after comments are inserted inline (1-indexed)
//
// When comments are inserted after document lines, display coordinates shift downward.
// For example, if a comment is inserted after line 5:
//   - Document line 5 → Display line 5
//   - Comment lines → Display lines 6, 7, 8 (no document line mapping)
//   - Document line 6 → Display line 9
//
// The lineMapping map records this transformation: lineMapping[docLine] = displayLine
// When lineMapping is nil, document coordinates equal display coordinates (no comments shown).
//
// # Helper Functions
//
//   - mapDocToDisplay: Document line → Display line
//   - mapDisplayToDoc: Display line → Document line (returns 0 for comment lines)
//   - buildDisplayToDocMap: Creates reverse lookup (display → document)
//
// # Architecture
//
// The review view is composed of several components:
//
//   - DocumentView: Renders markdown content with line numbers
//   - SearchMode: Handles search input and result navigation
//   - CommentModal: Multiline comment entry dialog
//   - PickerModal: Document selection tree
//   - ModalState: Coordinates modal lifecycle and rendering
//
// Sessions persist comments to a database (via stores.ReviewStore) and are tied
// to document content hashes. This allows reviews to be resumed across TUI invocations.
package review
