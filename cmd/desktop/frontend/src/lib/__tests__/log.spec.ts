import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, defineComponent, h } from 'vue'
import {
  Level,
  Write,
} from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/clientlogservice'
import * as log from '../log'

// test-setup stubs the binding for every spec.
const mocks = { Level: vi.mocked(Level), Write: vi.mocked(Write) }

function written(call = 0): Array<Record<string, unknown>> {
  return (mocks.Write.mock.calls[call][0] ?? []) as unknown as Array<Record<string, unknown>>
}

beforeEach(() => {
  vi.clearAllMocks()
  log.resetLogForTests()
  mocks.Level.mockResolvedValue('debug')
  mocks.Write.mockResolvedValue(undefined)
})

afterEach(() => {
  vi.useRealTimers()
  log.resetLogForTests()
})

describe('createLogger', () => {
  it('batches entries into one call and names the component', async () => {
    const feed = log.createLogger('feed-state')

    feed.warn('Unable to load flows', new Error('boom'), { flowId: 'f1' })
    feed.info('loaded')
    await log.flushLogs()

    expect(mocks.Write).toHaveBeenCalledTimes(1)
    const batch = written()
    expect(batch).toHaveLength(2)
    expect(batch[0]).toMatchObject({
      level: 'warn',
      name: 'feed-state',
      message: 'Unable to load flows',
      error: 'boom',
      window: 'main',
      fields: { flowId: 'f1' },
    })
    expect(batch[0].stack).toContain('boom')
    expect(batch[1]).toMatchObject({ level: 'info', message: 'loaded' })
  })

  it('reads the Kind and message the Go core put on a binding error', async () => {
    const err = new Error('call failed', { cause: { kind: 'not_found', message: 'no such flow' } })

    log.createLogger().error('Unable to open flow', err)
    await log.flushLogs()

    expect(written()[0]).toMatchObject({ name: '', error: 'no such flow', kind: 'not_found' })
  })

  it('collapses identical entries into one with a count', async () => {
    const loop = log.createLogger('render')

    for (let i = 0; i < 50; i++) loop.error('frame failed', 'bad state')
    await log.flushLogs()

    expect(written()).toHaveLength(1)
    expect(written()[0]).toMatchObject({ message: 'frame failed', count: 50 })
  })

  it('drops entries below the backend level', async () => {
    mocks.Level.mockResolvedValue('warn')
    const logger = log.createLogger()

    logger.debug('noise')
    logger.info('noise')
    logger.warn('kept')
    await log.flushLogs()

    expect(written().map((e) => e.message)).toEqual(['kept'])
  })

  it('reports how many entries an overflowing buffer dropped', async () => {
    const logger = log.createLogger()

    for (let i = 0; i < 205; i++) logger.info(`entry ${i}`)
    await log.flushLogs()

    const batch = written()
    expect(batch[0].message).toBe('entry 5')
    expect(batch.at(-1)).toMatchObject({ level: 'warn', fields: { dropped: 5 } })
  })

  it('flushes errors sooner than other entries', async () => {
    vi.useFakeTimers()
    const logger = log.createLogger()

    logger.info('later')
    logger.error('now')
    await vi.advanceTimersByTimeAsync(300)

    expect(mocks.Write).toHaveBeenCalledTimes(1)
    expect(written()).toHaveLength(2)
  })

  it('swallows a failed delivery', async () => {
    mocks.Write.mockRejectedValue(new Error('backend gone'))

    log.createLogger().error('lost')
    await expect(log.flushLogs()).resolves.toBeUndefined()
  })
})

describe('error capture', () => {
  it('logs uncaught errors and unhandled rejections', async () => {
    const target = new EventTarget() as unknown as Window
    log.installErrorCapture(target)

    target.dispatchEvent(new ErrorEvent('error', { error: new Error('kaboom'), filename: 'app.js', lineno: 3 }))
    const rejection = new Event('unhandledrejection') as PromiseRejectionEvent
    Object.defineProperty(rejection, 'reason', { value: 'nope' })
    target.dispatchEvent(rejection)
    await log.flushLogs()

    expect(written()).toEqual([
      expect.objectContaining({
        name: 'uncaught',
        message: 'Uncaught error',
        error: 'kaboom',
        fields: { source: 'app.js', line: 3 },
      }),
      expect.objectContaining({ name: 'uncaught', message: 'Unhandled promise rejection', error: 'nope' }),
    ])
  })

  it('logs errors Vue catches in a component', async () => {
    const Broken = defineComponent({
      name: 'BrokenPanel',
      setup() {
        throw new Error('setup failed')
      },
      render: () => h('div'),
    })
    const app = createApp(Broken)
    log.captureVueErrors(app)
    app.mount(document.createElement('div'))
    await log.flushLogs()

    expect(written()[0]).toMatchObject({
      name: 'vue',
      error: 'setup failed',
      fields: { component: 'BrokenPanel', hook: 'setup function' },
    })
    app.unmount()
  })
})
