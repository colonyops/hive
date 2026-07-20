// Configured action views are the one action contract consumed by desktop UI.
// The backend derives them from actions.ActionStore; UI presentation is based
// only on this safe view, never executable action configuration.
export type { View as ActionView } from '../../bindings/github.com/colonyops/hive/internal/desktop/pipeline/actions/models'
