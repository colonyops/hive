import { flushPromises } from '@vue/test-utils'
import { beforeEach, expect, it, vi } from 'vitest'
import { openManagedStream } from '../managedTransport'
const mocks = vi.hoisted(() => ({
  OpenStream: vi.fn(),
  ReadStream: vi.fn(),
  WriteStream: vi.fn(),
  CloseStream: vi.fn(),
}))
vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/remoteservice', () => mocks)
beforeEach(() => {
  vi.resetAllMocks()
  mocks.OpenStream.mockResolvedValue('stream')
  mocks.ReadStream.mockReturnValue(new Promise(() => {}))
  mocks.CloseStream.mockResolvedValue(undefined)
  mocks.WriteStream.mockResolvedValue(undefined)
})
it('preserves input order while a write is pending', async () => {
  let finish!: () => void
  mocks.WriteStream.mockReturnValueOnce(
    new Promise<void>((resolve) => {
      finish = resolve
    }),
  )
  const stream = openManagedStream('connection', 'session')
  await flushPromises()
  stream.send(new Uint8Array([1]))
  stream.send(new Uint8Array([2]))
  expect(mocks.WriteStream.mock.calls).toEqual([['connection', 'stream', 'AQ==']])
  finish()
  await flushPromises()
  expect(mocks.WriteStream.mock.calls).toEqual([
    ['connection', 'stream', 'AQ=='],
    ['connection', 'stream', 'Ag=='],
  ])
  stream.close()
})
it('delivers final bytes before the close event', async () => {
  mocks.ReadStream.mockResolvedValueOnce({ frames: ['AQI='], ended: true })
  const events: string[] = []
  const stream = openManagedStream('connection', 'session')
  stream.onopen = () => events.push('open')
  stream.onmessage = (event) => events.push([...new Uint8Array(event.data as ArrayBuffer)].join(','))
  stream.onclose = () => events.push('close')
  await flushPromises()
  expect(events).toEqual(['open', '1,2', 'close'])
  expect(mocks.CloseStream).toHaveBeenCalledWith('connection', 'stream')
})
it('closes a late stream when the pane is disposed during attachment', async () => {
  let finish!: (id: string) => void
  mocks.OpenStream.mockReturnValue(
    new Promise<string>((resolve) => {
      finish = resolve
    }),
  )
  const stream = openManagedStream('connection', 'session')
  const opened = vi.fn()
  stream.onopen = opened
  stream.close()
  finish('late')
  await flushPromises()
  expect(opened).not.toHaveBeenCalled()
  expect(mocks.CloseStream).toHaveBeenCalledWith('connection', 'late')
  expect(mocks.ReadStream).not.toHaveBeenCalled()
})
it('drops pending input on disconnect', async () => {
  let finish!: () => void
  mocks.WriteStream.mockReturnValueOnce(
    new Promise<void>((resolve) => {
      finish = resolve
    }),
  )
  const stream = openManagedStream('connection', 'session')
  await flushPromises()
  stream.send(new Uint8Array([1]))
  stream.send(new Uint8Array([2]))
  stream.close()
  finish()
  await flushPromises()
  expect(mocks.WriteStream).toHaveBeenCalledTimes(1)
})
