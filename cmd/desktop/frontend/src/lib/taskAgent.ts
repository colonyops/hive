import type { SessionStatus } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/models'

/**
 * What the agent in a task's session is doing. `stopped` is a session with no
 * tmux session; `unknown` is one with no status yet, or one this machine does
 * not poll.
 */
export type AgentActivity = 'active' | 'approval' | 'ready' | 'stopped' | 'unknown'

export interface TaskAgent {
  sessionId: string
  /** The session's name, or its id when no loaded session has it (an ended one). */
  name: string
  activity: AgentActivity
  /** The agent tool in the window that set `activity`, '' when none did. */
  tool: string
}

const RANK: Record<string, number> = { ready: 1, active: 2, approval: 3 }

/** A session runs several windows; the one that most needs the user speaks for it. */
export function sessionActivity(status: SessionStatus | undefined): { activity: AgentActivity; tool: string } {
  if (!status) return { activity: 'unknown', tool: '' }
  if (!status.running) return { activity: 'stopped', tool: '' }
  let best = { activity: 'unknown' as AgentActivity, tool: '' }
  let bestRank = 0
  for (const window of status.windows ?? []) {
    const rank = RANK[window.status] ?? 0
    if (rank > bestRank) {
      bestRank = rank
      best = { activity: window.status as AgentActivity, tool: window.tool }
    }
  }
  return best
}

export function taskAgent(sessionId: string, name: string | undefined, status: SessionStatus | undefined): TaskAgent {
  return { sessionId, name: name || sessionId, ...sessionActivity(status) }
}

export function agentLabel(agent: TaskAgent): string {
  const who = agent.tool || 'Agent'
  switch (agent.activity) {
    case 'active':
      return `${who} is working in ${agent.name}`
    case 'approval':
      return `${who} needs approval in ${agent.name}`
    case 'ready':
      return `${who} is ready in ${agent.name}`
    case 'stopped':
      return `Assigned to ${agent.name}, which is not running`
    default:
      return `Assigned to ${agent.name}`
  }
}
