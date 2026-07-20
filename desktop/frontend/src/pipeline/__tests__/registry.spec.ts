import { describe, expect, it } from 'vitest'
import { processorRegistry } from '../processors'
import { InProcessTransport } from '../engine/transport'
import type { Msg } from '../types'

describe('processorRegistry', () => {
  it('discovers exactly the function and github-filter runtimes via the nodes/*/runtime.ts glob', () => {
    expect(Object.keys(processorRegistry).sort()).toEqual(['function', 'github-filter'])
  })

  it('every registered runtime carries a `type` matching its registry key', () => {
    for (const [key, runtime] of Object.entries(processorRegistry)) {
      expect(runtime.type).toBe(key)
    }
  })

  it('is directly usable by InProcessTransport', async () => {
    const transport = new InProcessTransport(processorRegistry)
    const msg: Msg = { ID: '1', Key: '1', Topic: 'source:test', Ts: 0, Payload: { repo: 'acme/app' }, Meta: null }
    const result = await transport.run('github-filter', 'flow:node', { repos: ['acme/*'] }, msg, {}, 1000)
    expect(result).toEqual([msg, null])
  })
})
