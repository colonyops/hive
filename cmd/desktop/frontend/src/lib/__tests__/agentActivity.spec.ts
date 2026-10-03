import { describe, expect, it } from 'vitest'
import { activityIndicator } from '../agentActivity'

describe('activityIndicator', () => {
  it('names the tool, and falls back to "Agent"', () => {
    expect(activityIndicator('active', 'claude')).toMatchObject({ label: 'claude is working', animated: true })
    expect(activityIndicator('approval', '').label).toBe('Agent needs approval')
    expect(activityIndicator('ready').label).toBe('Agent is ready')
    expect(activityIndicator('bogus').label).toBe('Agent status unavailable')
  })
})
