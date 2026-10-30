import type { Terminal } from '@xterm/xterm'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  Info,
  Record as RecordSamples,
} from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/perfservice'
import { flushPerf, resetPerfForTests, resolvePerfEnabled } from '../../composables/usePerf'
import { resetFrameCollectorForTests } from '../frameCollector'
import { probePane } from '../terminalPerf'

function fakeTerminal() {
  let render: () => void = () => {}
  const term = {
    element: { offsetParent: {}, addEventListener: vi.fn(), removeEventListener: vi.fn() },
    buffer: { active: { viewportY: 0, type: 'normal' } },
    modes: { mouseTrackingMode: 'none' },
    onRender: (listener: () => void) => {
      render = listener
      return { dispose: vi.fn() }
    },
  } as unknown as Terminal
  return { term, render: () => render() }
}

async function recorded(): Promise<{ name: string; attrs: Record<string, unknown> }[]> {
  await flushPerf()
  return vi.mocked(RecordSamples).mock.calls.flatMap(([batch]) => batch) as unknown as {
    name: string
    attrs: Record<string, unknown>
  }[]
}

describe('probePane', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    resetPerfForTests()
    vi.mocked(RecordSamples).mockClear()
  })
  afterEach(() => {
    vi.useRealTimers()
    resetFrameCollectorForTests()
  })

  it('installs nothing when recording is off', async () => {
    vi.mocked(Info).mockResolvedValueOnce({ enabled: false } as never)
    await resolvePerfEnabled()
    const { term } = fakeTerminal()
    const onRender = vi.spyOn(term, 'onRender')
    const probe = probePane('%1', term, () => true)
    expect(probe.parsed).toBeNull()
    expect(onRender).not.toHaveBeenCalled()
  })

  describe('when recording', () => {
    beforeEach(async () => {
      vi.mocked(Info).mockResolvedValue({ enabled: true } as never)
      await resolvePerfEnabled()
    })

    it('times a key to its echo on screen, split by stage', async () => {
      const { term, render } = fakeTerminal()
      const probe = probePane('%1', term, () => true)
      probe.input('a')
      vi.advanceTimersByTime(2)
      probe.received(1)
      vi.advanceTimersByTime(3)
      probe.parsed?.(1)
      vi.advanceTimersByTime(5)
      render()
      probe.dispose()
      const echo = (await recorded()).find((sample) => sample.name === 'input:echo')
      expect(echo?.attrs).toMatchObject({ wireMs: 2, parseMs: 3, renderMs: 5, busy: false })
    })

    it('does not count a mouse report as a key', async () => {
      const { term, render } = fakeTerminal()
      const probe = probePane('%1', term, () => true)
      probe.input('\x1b[<0;1;1M')
      probe.received(1)
      probe.parsed?.(1)
      render()
      probe.dispose()
      expect((await recorded()).some((sample) => sample.name === 'input:echo')).toBe(false)
    })

    it('records a large burst once with its backlog, and ignores a small one', async () => {
      const { term } = fakeTerminal()
      const probe = probePane('%1', term, () => true)
      probe.received(100)
      probe.parsed?.(100)
      vi.advanceTimersByTime(1000)
      expect((await recorded()).some((sample) => sample.name === 'output:burst')).toBe(false)

      probe.received(64 << 10)
      expect(probe.backlog()).toBe(64 << 10)
      probe.parsed?.(64 << 10)
      vi.advanceTimersByTime(1000)
      const bursts = (await recorded()).filter((sample) => sample.name === 'output:burst')
      expect(bursts).toHaveLength(1)
      expect(bursts[0].attrs).toMatchObject({ bytes: 64 << 10, socketFrames: 1, maxBacklog: 64 << 10 })
      probe.dispose()
    })

    it('keeps a burst open while the emulator still owes bytes', async () => {
      const { term } = fakeTerminal()
      const probe = probePane('%1', term, () => true)
      probe.received(64 << 10)
      vi.advanceTimersByTime(1000)
      expect((await recorded()).some((sample) => sample.name === 'output:burst')).toBe(false)
      probe.parsed?.(64 << 10)
      vi.advanceTimersByTime(1000)
      expect((await recorded()).some((sample) => sample.name === 'output:burst')).toBe(true)
      probe.dispose()
    })
  })
})
