import { h, markRaw, type Component } from 'vue'
import { agentIconID, type AgentIconID } from '../../lib/agentIcon'
import SessionTabIcon from './SessionTabIcon.vue'

// A menu takes a bare icon component with no props, so each mark is wrapped once.
// A fresh component per render would remount every row.
const icons = new Map<AgentIconID, Component>()

/** The coloured mark, for places where agents have to stand apart from each other. */
export function agentBrandIcon(agent: string): Component | undefined {
  const id = agentIconID(agent)
  if (!id) return undefined
  let icon = icons.get(id)
  if (!icon) {
    icon = markRaw(() => h(SessionTabIcon, { kind: 'window', agent: id }))
    icons.set(id, icon)
  }
  return icon
}
