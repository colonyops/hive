package flow

// Refs resolves the one remaining cross-file reference a flow's nodes point
// at: the action node's desktop actions.yml action id. (Source and feed nodes
// are now self-contained — a source embeds its fetch config, a feed's
// identity is its node id — so they no longer resolve anything.) It is
// injected rather than imported so this package never depends on the actions
// loader.
type Refs interface {
	// ResolveAction reports whether id names a known action.
	ResolveAction(id string) bool
}

// refsResolveAction guards the action node's Validate against a nil Refs (an
// untyped nil interface panics if called directly) so a flow validated
// without a resolver — or with one that doesn't know about a given id — fails
// with an "unresolved reference" error rather than a panic.
func refsResolveAction(refs Refs, id string) bool {
	if refs == nil {
		return false
	}
	return refs.ResolveAction(id)
}
