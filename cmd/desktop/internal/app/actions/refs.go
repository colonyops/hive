package actions

// Refs resolves a flow's only remaining cross-file reference — the action
// node's actions.yml action id — against the actions catalog, satisfying
// flow.Refs and flow.HeadlessActionRefs. Source and feed nodes are now
// self-contained (a source embeds its fetch config, a feed's identity is its
// node id), so this adapter no longer touches the feed provider.
//
// It deliberately names no flow type: flow declares the interfaces it
// consumes and this satisfies them structurally at the wiring site, which is
// what keeps actions free of a production dependency on flow.
type Refs struct {
	actions *ActionStore
}

func NewRefs(catalog *ActionStore) *Refs {
	return &Refs{actions: catalog}
}

func (r *Refs) ResolveAction(id string) bool {
	_, ok := r.actions.Get(id)
	return ok
}

func (r *Refs) ActionHeadlessCapable(id string) bool {
	action, ok := r.actions.Get(id)
	return ok && action.HeadlessCapable()
}
