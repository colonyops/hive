import { describe, expect, it } from 'vitest'
import { buildFlowPrompt } from '../flowPrompt'
import { byType } from '../../registry'

describe('buildFlowPrompt', () => {
  it("includes every registered node type's help text and default config", () => {
    const prompt = buildFlowPrompt()

    for (const def of Object.values(byType)) {
      expect(prompt).toContain(`type: "${def.type}"`)
      expect(prompt).toContain(def.label)
      // help.md's own heading is a stable substring of its rendered content.
      const firstLine = def.help.trim().split('\n')[0]!
      expect(prompt).toContain(firstLine)
      expect(prompt).toContain(JSON.stringify(def.defaults, null, 2))
    }
  })

  it('includes the top-level flows/*.yaml schema and a worked example', () => {
    const prompt = buildFlowPrompt()

    expect(prompt).toContain('version: must be 1')
    expect(prompt).toContain('wires: list of { from, out, to }')
    expect(prompt).toContain('```yaml')
    expect(prompt).toContain('github-source')
  })

  it('is deterministic across calls', () => {
    expect(buildFlowPrompt()).toBe(buildFlowPrompt())
  })
})
