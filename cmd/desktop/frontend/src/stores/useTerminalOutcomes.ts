import { shallowReactive } from 'vue'
import { defineStore } from './defineStore'

export type TerminalEndReason =
  | 'not-started'
  | 'start-failed'
  | 'attach-failed'
  | 'exited'
  | 'terminated'
  | 'completed'
  | 'stopped'
  | 'error'
  | 'disconnected'

export interface TerminalOutcome {
  reason: TerminalEndReason
  detail: string
}

interface SessionOutcome {
  attempt: number
  wasRunning: boolean
  notice: TerminalOutcome | null
}

const maxSessionOutcomes = 64

export const useTerminalOutcomes = defineStore('terminalOutcomes', () => {
  const sessions = shallowReactive(new Map<string, SessionOutcome>())
  let nextAttempt = 0

  function put(slug: string, state: SessionOutcome): void {
    sessions.delete(slug)
    sessions.set(slug, state)
    if (sessions.size > maxSessionOutcomes) sessions.delete(sessions.keys().next().value!)
  }

  function beginAttempt(slug: string): number {
    const previous = sessions.get(slug)
    const attempt = ++nextAttempt
    put(slug, { attempt, wasRunning: previous?.wasRunning ?? false, notice: previous?.notice ?? null })
    return attempt
  }

  function report(slug: string, notice: TerminalOutcome, attempt?: number): void {
    const previous = sessions.get(slug)
    if (attempt !== undefined && previous?.attempt !== attempt) return
    put(slug, { attempt: previous?.attempt ?? 0, wasRunning: previous?.wasRunning ?? false, notice })
  }

  function running(slug: string): void {
    const previous = sessions.get(slug)
    put(slug, { attempt: previous?.attempt ?? 0, wasRunning: true, notice: null })
  }

  return {
    outcome: (slug: string) => sessions.get(slug)?.notice ?? null,
    wasRunning: (slug: string) => sessions.get(slug)?.wasRunning ?? false,
    beginAttempt,
    report,
    running,
    forget: (slug: string) => sessions.delete(slug),
  }
})
