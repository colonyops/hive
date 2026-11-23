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
    expect(sessionActivity(status)).toEqual({ activity: 'approval', tool: 'codex', windowId: '@2' })
  })

  it('is unknown for a running session whose windows report nothing it knows', () => {
    expect(sessionActivity({ sessionId: 's', running: true, windows: null }).activity).toBe('unknown')
  })
})

describe('taskAgent', () => {
  it('falls back to the id, and cannot be opened, for a session with no loaded row', () => {
    const agent = taskAgent('abc', undefined, undefined)
    expect(agent.name).toBe('abc')
    expect(agent.slug).toBe('')
  })

  it('opens only an active session', () => {
    expect(taskAgent('abc', { name: 'auth', slug: 'auth-abc', state: 'active' }, undefined).slug).toBe('auth-abc')
    expect(taskAgent('abc', { name: 'auth', slug: 'auth-abc', state: 'recycled' }, undefined).slug).toBe('')
  })

  it('labels a working agent with its tool and session', () => {
    const agent = taskAgent(
      'abc',
      { name: 'auth', slug: 'auth', state: 'active' },
      {
        sessionId: 'abc',
        running: true,
        windows: [{ windowId: '@1', status: 'active', tool: 'claude' }],
      },
    )
    expect(agentLabel(agent)).toBe('claude is working in auth')
  })
})
