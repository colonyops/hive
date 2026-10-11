import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, defineComponent, h } from 'vue'
import type { ClientLogEntry } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'
import { LogSink } from '../log'

function setup(level = 'debug') {
  const transport = {
    level: vi.fn(() => Promise.resolve(level)),
    write: vi.fn((_entries: ClientLogEntry[]) => Promise.resolve()),
  }
  const sink = new LogSink({
    transport,
    echoToConsole: false,
    location: () => ({ search: '', hash: '#/inbox', pathname: '/' }),
  })
  const written = (call = 0) => transport.write.mock.calls[call][0]
  return { transport, sink, written }
}

afterEach(() => {
  vi.useRealTimers()
})

describe('LogSink', () => {
  it('batches entries into one call and names the component', async () => {
    const { sink, transport, written } = setup()
    const feed = sink.logger('feed-state')

    feed.warn('unable to load flows', new Error('boom'), { flowId: 'f1' })
    feed.info('loaded')
    await sink.flush()

    expect(transport.write).toHaveBeenCalledTimes(1)
    const batch = written()
    expect(batch).toHaveLength(2)
    expect(batch[0]).toMatchObject({
      level: 'warn',
      name: 'feed-state',
      message: 'unable to load flows',
      error: 'boom',
      window: 'main',
      route: '#/inbox',
      fields: { flowId: 'f1' },
    })
    expect(batch[0].stack).toContain('boom')
    expect(batch[1]).toMatchObject({ level: 'info', message: 'loaded' })
  })

  it('reads the Kind and message the Go core put on a binding error', async () => {
    const { sink, written } = setup()
    const err = new Error('call failed', { cause: { kind: 'not_found', message: 'no such flow' } })

    sink.logger().error('unable to open flow', err)
    await sink.flush()

    expect(written()[0]).toMatchObject({ name: '', error: 'no such flow', kind: 'not_found' })
  })

  it('collapses identical entries into one with a count', async () => {
    const { sink, written } = setup()
    const loop = sink.logger('render')

    for (let i = 0; i < 50; i++) loop.error('frame failed', 'bad state')
    await sink.flush()

    expect(written()).toHaveLength(1)
    expect(written()[0]).toMatchObject({ message: 'frame failed', count: 50 })
  })

  it('drops entries below the backend level', async () => {
    const { sink, written } = setup('warn')
    const logger = sink.logger()

    logger.debug('noise')
    logger.info('noise')
    logger.warn('kept')
    await sink.flush()

    expect(written().map((e) => e.message)).toEqual(['kept'])
  })

  it('reports how many entries an overflowing buffer dropped', async () => {
    const { sink, written } = setup()
    const logger = sink.logger()

    for (let i = 0; i < 205; i++) logger.info(`entry ${i}`)
    await sink.flush()

    const batch = written()
    expect(batch[0].message).toBe('entry 5')
    expect(batch.at(-1)).toMatchObject({ level: 'warn', fields: { dropped: 5 } })
  })

  it('flushes errors sooner than other entries', async () => {
    vi.useFakeTimers()
    const { sink, transport, written } = setup()
    const logger = sink.logger()

    logger.info('later')
    logger.error('now')
    await vi.advanceTimersByTimeAsync(300)

    expect(transport.write).toHaveBeenCalledTimes(1)
    expect(written()).toHaveLength(2)
  })

  it('swallows a failed delivery', async () => {
    const { sink, transport } = setup()
    transport.write.mockRejectedValue(new Error('backend gone'))

    sink.logger().error('lost')
    await expect(sink.flush()).resolves.toBeUndefined()
  })

  it('logs uncaught errors and unhandled rejections', async () => {
    const { sink, written } = setup()
    const target = new EventTarget() as unknown as Window
    sink.installErrorCapture(target)

    target.dispatchEvent(new ErrorEvent('error', { error: new Error('kaboom'), filename: 'app.js', lineno: 3 }))
    const rejection = new Event('unhandledrejection') as PromiseRejectionEvent
    Object.defineProperty(rejection, 'reason', { value: 'nope' })
    target.dispatchEvent(rejection)
    await sink.flush()

    expect(written()).toEqual([
      expect.objectContaining({
        name: 'uncaught',
        message: 'uncaught error',
        error: 'kaboom',
        fields: { source: 'app.js', line: 3 },
      }),
      expect.objectContaining({ name: 'uncaught', message: 'unhandled promise rejection', error: 'nope' }),
    ])
  })

  it('logs errors Vue catches in a component', async () => {
    const { sink, written } = setup()
    const Broken = defineComponent({
      name: 'BrokenPanel',
      setup() {
        throw new Error('setup failed')
      },
      render: () => h('div'),
    })
    const app = createApp(Broken)
    sink.captureVueErrors(app)
    app.mount(document.createElement('div'))
    await sink.flush()

    expect(written()[0]).toMatchObject({
      name: 'vue',
      error: 'setup failed',
      fields: { component: 'BrokenPanel', hook: 'setup function' },
    })
    app.unmount()
  })
})
