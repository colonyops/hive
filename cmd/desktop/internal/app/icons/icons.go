// Package icons holds the curated glyph set feed rows and sidebar entries
// draw from. It is a leaf: the flow package validates a feed node's icon and
// the webhook connector validates its own, and neither may import the other,
// so the set they share lives below both.
package icons

// feed is the scoped set of glyphs a feed node (sidebar entry) or a webhook
// source node (feed-item rows) may carry. It is intentionally small — a
// curated list rather than every available icon — and must stay in sync with
// the frontend's feed icon registry (desktop/frontend/src/lib/feedIcons.ts).
var feed = map[string]bool{
	"git-branch":       true,
	"git-pull-request": true,
	"circle-dot":       true,
	"message-square":   true,
	"at-sign":          true,
	"rss":              true,
	"webhook":          true,
	"bell":             true,
	"eye":              true,
	"star":             true,
	"bug":              true,
	"shield":           true,
	"zap":              true,
	"sparkles":         true,
	"flag":             true,
	"inbox":            true,
	"users":            true,
	"tag":              true,
	"package":          true,
	"rocket":           true,
	"clock":            true,
	"terminal":         true,
}

// launcher is the scoped set of glyphs a pop-up terminal launcher may carry in
// the command palette. It is its own set rather than a share of feed's: these
// name programs and tasks, not the kinds of thing a feed carries. It must stay
// in sync with desktop/frontend/src/lib/launcherIcons.ts.
var launcher = map[string]bool{
	"terminal":      true,
	"git-branch":    true,
	"git-compare":   true,
	"folder":        true,
	"file-text":     true,
	"search":        true,
	"database":      true,
	"gauge":         true,
	"activity":      true,
	"flask-conical": true,
	"hammer":        true,
	"container":     true,
	"cloud":         true,
	"bug":           true,
	"zap":           true,
	"package":       true,
}

// ValidFeed reports whether name is a supported feed glyph. The empty string
// means "use the default" and is always allowed, so callers can validate an
// optional field without special-casing it.
func ValidFeed(name string) bool {
	return name == "" || feed[name]
}

// ValidLauncher reports whether name is a supported launcher glyph, on the same
// terms as ValidFeed.
func ValidLauncher(name string) bool {
	return name == "" || launcher[name]
}
