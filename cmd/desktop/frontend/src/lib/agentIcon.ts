import { h, markRaw, type Component } from 'vue'
import AgentIcon from '../components/AgentIcon.vue'

// Profile names can use known product or provider aliases; unknown names get
// no mark.
const ALIASES = {
  claude: ['claude', 'anthropic', 'fable'],
  codex: ['codex', 'openai', 'chatgpt', 'gpt'],
  pi: ['pi'],
  agents: ['agents'],
} as const

export type AgentIconID = keyof typeof ALIASES

export function agentIconID(name: string): AgentIconID | undefined {
  const words = name
    .toLowerCase()
    .split(/[^a-z0-9]+/)
    .filter(Boolean)
  return (Object.entries(ALIASES) as [AgentIconID, readonly string[]][]).find(([, aliases]) =>
    aliases.some((alias) => words.includes(alias)),
  )?.[0]
}

// Menus and selects take a bare icon component with no props, so each agent
// mark is wrapped once. A fresh component per render would remount every row.
const icons = new Map<AgentIconID, Component>()

export function agentIcon(agent: string): Component | undefined {
  const id = agentIconID(agent)
  if (!id) return undefined
  let icon = icons.get(id)
  if (!icon) {
    icon = markRaw(() => h(AgentIcon, { id }))
    icons.set(id, icon)
  }
  return icon
}
