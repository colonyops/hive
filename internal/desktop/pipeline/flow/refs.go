package flow

// Refs resolves the cross-file references a flow's nodes point at: profiles
// sources (github-source/rpc-source), profiles feeds (feed), and
// .hive/actions.yml actions (action). It is injected rather than imported so
// this package never depends on the profiles/actions loaders — a caller
// wires in the real resolver once those loaders exist.
type Refs interface {
	// ResolveSource reports whether id names a known source and, if so, its
	// kind (e.g. "github-search", "github-notifications", "rpc").
	ResolveSource(id string) (kind string, ok bool)
	// ResolveFeed reports whether id names a known feed.
	ResolveFeed(id string) bool
	// ResolveAction reports whether id names a known action.
	ResolveAction(id string) bool
}

// MapRefs is a simple map-backed Refs implementation for tests (and for any
// caller happy to precompute the lookups into maps rather than implement the
// interface directly).
type MapRefs struct {
	// Sources maps a source id to its kind.
	Sources map[string]string
	// Feeds is the set of known feed ids.
	Feeds map[string]bool
	// Actions is the set of known action ids.
	Actions map[string]bool
}

func (m MapRefs) ResolveSource(id string) (string, bool) {
	kind, ok := m.Sources[id]
	return kind, ok
}

func (m MapRefs) ResolveFeed(id string) bool {
	return m.Feeds[id]
}

func (m MapRefs) ResolveAction(id string) bool {
	return m.Actions[id]
}

// refsResolveSource, refsResolveFeed, and refsResolveAction guard every
// per-type Validate against a nil Refs (an untyped nil interface panics if
// called directly) so a flow validated without a resolver — or with one that
// doesn't know about a given id — fails with an "unresolved reference"
// error rather than a panic.
func refsResolveSource(refs Refs, id string) (string, bool) {
	if refs == nil {
		return "", false
	}
	return refs.ResolveSource(id)
}

func refsResolveFeed(refs Refs, id string) bool {
	if refs == nil {
		return false
	}
	return refs.ResolveFeed(id)
}

func refsResolveAction(refs Refs, id string) bool {
	if refs == nil {
		return false
	}
	return refs.ResolveAction(id)
}
