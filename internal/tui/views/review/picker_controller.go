package review

import (
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
