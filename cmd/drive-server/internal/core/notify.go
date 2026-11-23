package core

// Notifier hears about changes that open document streams care about. The
// calls must not block: a service makes them after its write commits.
type Notifier interface {
	DocumentChanged(ownerID, documentID, sha256 string)
	// DocumentClosed means the document was deleted.
	DocumentClosed(ownerID, documentID string)
	// LinkClosed means the document's share link was revoked. Only streams
	// opened through the link end.
	LinkClosed(ownerID, documentID string)
	ThreadsChanged(ownerID, documentID string)
}

type noopNotifier struct{}

func (noopNotifier) DocumentChanged(string, string, string) {}
func (noopNotifier) DocumentClosed(string, string)          {}
func (noopNotifier) LinkClosed(string, string)              {}
func (noopNotifier) ThreadsChanged(string, string)          {}
