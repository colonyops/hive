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
  it('reads from the cursor, runs the graph, commits, and advances the cursor to upToOffset', async () => {
    const batch = [msg('1'), msg('2'), msg('3')]
    const readFrom = vi.fn().mockResolvedValue(batch)
    const commit = vi.fn().mockResolvedValue(undefined)
    const client: PipelineClient = { readFrom, commit }
    const driver = new PipelineDriver(client, simpleFlow())

    const result = await driver.pump()

    expect(readFrom).toHaveBeenCalledWith(0, 500)
    expect(result?.upToOffset).toBe(3)
    expect(result?.outputs).toHaveLength(3)
    expect(commit).toHaveBeenCalledWith(result)
    expect(driver.offset).toBe(3)
  })

  it('returns null and leaves the cursor unchanged when there is nothing new to read', async () => {
    const readFrom = vi.fn().mockResolvedValue([])
    const commit = vi.fn()
    const driver = new PipelineDriver({ readFrom, commit }, simpleFlow())

    const result = await driver.pump()

    expect(result).toBeNull()
    expect(commit).not.toHaveBeenCalled()
    expect(driver.offset).toBe(0)
  })

  it('treats a null ReadFrom response the same as an empty batch', async () => {
    const readFrom = vi.fn().mockResolvedValue(null)
    const commit = vi.fn()
    const driver = new PipelineDriver({ readFrom, commit }, simpleFlow())

    expect(await driver.pump()).toBeNull()
    expect(commit).not.toHaveBeenCalled()
  })

  it('reads from the advanced cursor on the next pump', async () => {
    const readFrom = vi
      .fn()
      .mockResolvedValueOnce([msg('1'), msg('2')])
      .mockResolvedValueOnce([msg('3')])
    const commit = vi.fn().mockResolvedValue(undefined)
    const driver = new PipelineDriver({ readFrom, commit }, simpleFlow())

    await driver.pump()
    await driver.pump()

    expect(readFrom).toHaveBeenNthCalledWith(1, 0, 500)
    expect(readFrom).toHaveBeenNthCalledWith(2, 2, 500)
    expect(driver.offset).toBe(3)
  })

  it('honors a custom limit and starting offset', async () => {
    const readFrom = vi.fn().mockResolvedValue([])
    const driver = new PipelineDriver({ readFrom, commit: vi.fn() }, simpleFlow(), { limit: 50, fromOffset: 100 })
    expect(driver.offset).toBe(100)
    await driver.pump()
    expect(readFrom).toHaveBeenCalledWith(100, 50)
  })

  it('idempotent replay: pumping the same starting offset twice produces an identical CommitBatch', async () => {
    // Two independent drivers both starting at offset 0 and reading the
    // same not-yet-advanced window simulate a duplicate pump (e.g. a
    // repeated "log:appended" wake-up before either commit was observed).
    // The Go side dedups by offset; this asserts the TS-side computation
    // itself is pure and deterministic, which is what makes that dedup safe.
    const batch = [msg('1'), msg('2')]
    const driverA = new PipelineDriver({ readFrom: vi.fn().mockResolvedValue(batch), commit: vi.fn().mockResolvedValue(undefined) }, simpleFlow())
    const driverB = new PipelineDriver({ readFrom: vi.fn().mockResolvedValue(batch), commit: vi.fn().mockResolvedValue(undefined) }, simpleFlow())

    const resultA = await driverA.pump()
    const resultB = await driverB.pump()

    expect(resultA).toEqual(resultB)
  })
})
