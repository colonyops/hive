// Single source of truth for how a configured action's type reads in the UI:
// the glyph, the human label, and the identity hue used by the detail-pane
// run card. The settings list (ActionSettingsView) and the detail-pane card
// (ActionCard) both present the same three action types, so the mapping lives
// here rather than duplicated at each call site.

export interface ActionTypeMeta {
  // AppIcon registry name — every value here must be registered in AppIcon.vue.
  icon: string
  label: string
  color: string
}

const registry: Record<string, ActionTypeMeta> = {
  'launch-session': { icon: 'play', label: 'Launch session', color: '#34d399' },
  shell: { icon: 'terminal', label: 'Run shell command', color: '#60a5fa' },
  'publish-message': { icon: 'radio', label: 'Publish message', color: '#a78bfa' },
}

export function actionTypeMeta(type: string): ActionTypeMeta {
  return registry[type] ?? { icon: 'play', label: type, color: '#94a3b8' }
}
