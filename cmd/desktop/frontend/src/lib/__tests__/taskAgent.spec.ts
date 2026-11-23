import { describe, expect, it } from 'vitest'
import { agentLabel, sessionActivity, taskAgent } from '../taskAgent'

describe('sessionActivity', () => {
  it('is unknown without a status and stopped when tmux is not running', () => {
    expect(sessionActivity(undefined).activity).toBe('unknown')
    expect(sessionActivity({ sessionId: 's', running: false, windows: [] }).activity).toBe('stopped')
  })

  it('lets the window that most needs the user speak for the session', () => {
    const status = {
      sessionId: 's',
      running: true,
      windows: [
        { windowId: '@1', status: 'ready', tool: 'claude' },
        { windowId: '@2', status: 'approval', tool: 'codex' },
        { windowId: '@3', status: 'active', tool: 'pi' },
      ],
    }
    expect(sessionActivity(status)).toEqual({ activity: 'approval', tool: 'codex' })
  })

  it('is unknown for a running session whose windows report nothing it knows', () => {
    expect(sessionActivity({ sessionId: 's', running: true, windows: null }).activity).toBe('unknown')
  })
})

describe('taskAgent', () => {
  it('falls back to the id for a session with no loaded name', () => {
    expect(taskAgent('abc', undefined, undefined).name).toBe('abc')
  })

  it('labels a working agent with its tool and session', () => {
    const agent = taskAgent('abc', 'auth', {
      sessionId: 'abc',
      running: true,
      windows: [{ windowId: '@1', status: 'active', tool: 'claude' }],
    })
    expect(agentLabel(agent)).toBe('claude is working in auth')
  })
})
