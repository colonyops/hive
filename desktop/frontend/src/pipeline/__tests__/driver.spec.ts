import { describe, expect, it, vi } from 'vitest'
import { PipelineDriver, type PipelineClient } from '../driver'
import type { Flow, Msg } from '../types'

function msg(id: string, payload: any = {}): Msg {
  return { ID: id, Key: id, Topic: 'source:test', Ts: 0, Payload: payload, Meta: null }
}

function simpleFlow(): Flow {
  return {
    id: 'flow-1',
    nodes: [{ id: 'feed', type: 'feed', config: { feed: 'inbox' } }],
    wires: [],
  }
}

describe('PipelineDriver', () => {
  it('reads for the flow consumer, commits, and records the decimal committed offset', async () => {
    const batch = [msg('1'), msg('2'), msg('3')]
    const readFrom = vi.fn().mockResolvedValue(batch)
    const commit = vi.fn().mockResolvedValue(undefined)
    const driver = new PipelineDriver({ readFrom, commit }, simpleFlow())

    const result = await driver.pump()

    expect(readFrom).toHaveBeenCalledWith('flow-1', 500)
    expect(result?.upToOffset).toBe('3')
    expect(commit).toHaveBeenCalledWith(result)
    expect(driver.offset).toBe('3')
  })

  it('returns null and has no local zero cursor when there is nothing new to read', async () => {
    const readFrom = vi.fn().mockResolvedValue([])
    const commit = vi.fn()
    const driver = new PipelineDriver({ readFrom, commit }, simpleFlow())

    expect(driver.offset).toBeNull()
    expect(await driver.pump()).toBeNull()
    expect(commit).not.toHaveBeenCalled()
  })

  it('uses the persisted consumer checkpoint after a frontend restart', async () => {
    const events = [msg('1'), msg('2'), msg('3')]
    let persistedOffset = '0'
    const readFrom = vi.fn(async (consumer: string) => {
      expect(consumer).toBe('flow-1')
      return events.filter((event) => BigInt(event.ID) > BigInt(persistedOffset))
    })
    const commit = vi.fn(async (batch) => { persistedOffset = batch.upToOffset })

    const firstDriver = new PipelineDriver({ readFrom, commit }, simpleFlow())
    await firstDriver.pump()
    expect(persistedOffset).toBe('3')

    events.push(msg('4'))
    const restartedDriver = new PipelineDriver({ readFrom, commit }, simpleFlow())
    await restartedDriver.pump()

    expect(readFrom).toHaveBeenLastCalledWith('flow-1', 500)
    expect(commit).toHaveBeenLastCalledWith(expect.objectContaining({ upToOffset: '4' }))
  })

  it('honors a custom page limit', async () => {
    const readFrom = vi.fn().mockResolvedValue([])
    const driver = new PipelineDriver({ readFrom, commit: vi.fn() }, simpleFlow(), { limit: 50 })
    await driver.pump()
    expect(readFrom).toHaveBeenCalledWith('flow-1', 50)
  })
})
