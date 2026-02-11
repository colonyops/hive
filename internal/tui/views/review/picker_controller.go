package review

import (
	"strings"
	"time"
)

// PickerController handles data operations for the document picker.
type PickerController struct {
	documents       []Document
	recentThreshold time.Duration
}

// NewPickerController creates a new PickerController with the provided documents.
// The recent threshold defaults to 24 hours.
func NewPickerController(documents []Document) PickerController {
	return PickerController{
		documents:       documents,
		recentThreshold: 24 * time.Hour,
	}
}

// Filter returns documents that match the query string.
// Performs case-insensitive fuzzy substring matching on Name (RelPath) and Path.
func (pc PickerController) Filter(query string) []Document {
	if query == "" {
		return pc.documents
	}

	query = strings.ToLower(query)
	var filtered []Document

	for _, doc := range pc.documents {
		// Check both RelPath and Path for matches
		relPathLower := strings.ToLower(doc.RelPath)
		pathLower := strings.ToLower(doc.Path)

		if strings.Contains(relPathLower, query) || strings.Contains(pathLower, query) {
			filtered = append(filtered, doc)
		}
	}

	return filtered
}

// IsRecent returns true if the document's modification time is within the recent threshold.
func (pc PickerController) IsRecent(doc Document) bool {
	return time.Since(doc.ModTime) <= pc.recentThreshold
}

// GetLatest returns the document with the most recent modification time.
// Returns nil if there are no documents.
func (pc PickerController) GetLatest() *Document {
	if len(pc.documents) == 0 {
		return nil
	}

	latest := &pc.documents[0]
	for i := 1; i < len(pc.documents); i++ {
		if pc.documents[i].ModTime.After(latest.ModTime) {
			latest = &pc.documents[i]
		}
	}

	return latest
}

// SortByModTime returns a copy of the documents sorted by modification time (newest first).
func (pc PickerController) SortByModTime() []Document {
	// Create a copy
	sorted := make([]Document, len(pc.documents))
	copy(sorted, pc.documents)

	// Simple bubble sort for small collections
	for i := range sorted {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i].ModTime.Before(sorted[j].ModTime) {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	return sorted
}
