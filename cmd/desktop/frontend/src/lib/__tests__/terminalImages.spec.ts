import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises } from '@vue/test-utils'
import { installTerminalImages } from '../terminalImages'
import { resetToastsForTests, useToasts } from '../../composables/useToasts'

const mocks = vi.hoisted(() => ({
  prepare: vi.fn(),
  drop: undefined as undefined | ((event: { data: { target: string; paths: string[] } }) => void),
  unsubscribe: vi.fn(),
}))
vi.mock('@wailsio/runtime', () => ({ Events: { On: (_: string, handler: typeof mocks.drop) => {
  mocks.drop = handler
  return mocks.unsubscribe
} } }))
vi.mock('../terminalImagesClient', () => ({ prepareTerminalImages: mocks.prepare }))

const disposers: (() => void)[] = []
afterEach(() => {
  disposers.splice(0).forEach((dispose) => dispose())
  document.body.replaceChildren()
  resetToastsForTests()
  vi.clearAllMocks()
  delete (window as Window & { _wails?: unknown })._wails
})

function pane() {
  const parent = document.createElement('div')
  const host = document.createElement('div')
  const textarea = document.createElement('textarea')
  host.append(textarea)
  parent.append(host)
  document.body.append(parent)
  host.getClientRects = () => (parent.hidden ? [] : [{}]) as unknown as DOMRectList
  let current = true
  const paste = vi.fn()
  const text = vi.fn()
  const focus = vi.fn()
  const dispose = installTerminalImages(host, {
    pasteText: text,
    capture: () => ({ current: () => current, paste, focus }),
  })
  disposers.push(dispose)
  return { parent, host, textarea, paste, text, focus, stale: () => { current = false }, dispose }
}

function drop(host: HTMLElement, paths = ['/one.png']) {
  mocks.drop!({ data: { target: host.id, paths } })
}

describe('terminal image input', () => {
  it('routes a native drop to exactly one pane, with a separate paste for each image', async () => {
    mocks.prepare.mockResolvedValue(['/one.png ', '/two.png '])
    const first = pane()
    const second = pane()
    drop(second.host, ['/one.png', '/two.png'])
    await flushPromises()
    expect(first.paste).not.toHaveBeenCalled()
    expect(second.paste.mock.calls.map(([text]) => text)).toEqual(['/one.png ', '/two.png '])
    expect(second.focus).toHaveBeenCalledOnce()
  })

  it('lets native Wails own a drop even when the DOM also exposes a file', async () => {
    Object.assign(window, { _wails: { flags: { enableFileDrop: true } } })
    mocks.prepare.mockResolvedValue(['/one.png '])
    const p = pane()
    const event = new Event('drop', { bubbles: true, cancelable: true })
    Object.defineProperty(event, 'dataTransfer', { value: { files: [new File(['image'], 'one.png')] } })
    p.host.dispatchEvent(event)
    drop(p.host)
    await flushPromises()
    expect(mocks.prepare).toHaveBeenCalledOnce()
    expect(mocks.prepare).toHaveBeenCalledWith(['/one.png'], expect.any(AbortSignal))
    expect(p.paste).toHaveBeenCalledOnce()
  })

  it('captures clipboard images before xterm and does not paste their alternate text', async () => {
    Object.assign(window, { _wails: { flags: { enableFileDrop: true } } })
    mocks.prepare.mockResolvedValue(['/clipboard.png '])
    const p = pane()
    const xterm = vi.fn()
    p.textarea.addEventListener('paste', xterm)
    const file = new File(['image'], 'screenshot.png', { type: 'image/png' })
    const event = new Event('paste', { bubbles: true, cancelable: true })
    Object.defineProperty(event, 'clipboardData', { value: { files: [file], getData: () => 'alternate text' } })
    p.textarea.dispatchEvent(event)
    expect(event.defaultPrevented).toBe(true)
    await flushPromises()
    expect(mocks.prepare).toHaveBeenCalledWith([file], expect.any(AbortSignal))
    expect(xterm).not.toHaveBeenCalled()
    expect(p.text).not.toHaveBeenCalled()
    expect(p.paste).toHaveBeenCalledOnce()
  })

  it('keeps ordinary multiline text paste synchronous', () => {
    const p = pane()
    const event = new Event('paste', { bubbles: true, cancelable: true })
    Object.defineProperty(event, 'clipboardData', { value: { getData: () => 'one\ntwo' } })
    p.textarea.dispatchEvent(event)
    expect(p.text).toHaveBeenCalledWith('one\ntwo')
    expect(mocks.prepare).not.toHaveBeenCalled()
  })

  it('does not paste after switching chats during upload', async () => {
    let finish!: (pastes: string[]) => void
    mocks.prepare.mockReturnValue(new Promise<string[]>((resolve) => { finish = resolve }))
    const p = pane()
    drop(p.host)
    await flushPromises()
    p.stale()
    finish(['/one.png '])
    await flushPromises()
    expect(p.paste).not.toHaveBeenCalled()
    expect(useToasts().toasts.value).toEqual([])
  })

  it('cancels an upload when a pooled pane is hidden, even if it is shown again', async () => {
    let finish!: (pastes: string[]) => void
    mocks.prepare.mockReturnValue(new Promise<string[]>((resolve) => { finish = resolve }))
    const p = pane()
    drop(p.host)
    await flushPromises()
    p.parent.hidden = true
    await flushPromises()
    p.parent.hidden = false
    finish(['/one.png '])
    await flushPromises()
    expect(mocks.prepare.mock.calls[0][1].aborted).toBe(true)
    expect(p.paste).not.toHaveBeenCalled()
  })

  it('serializes gestures and never retries an uncertain paste', async () => {
    mocks.prepare.mockResolvedValue(['/one.png '])
    const p = pane()
    let reject!: (error: Error) => void
    p.paste.mockReturnValueOnce(new Promise<void>((_, fail) => { reject = fail }))
    drop(p.host)
    drop(p.host)
    await flushPromises()
    expect(mocks.prepare).toHaveBeenCalledTimes(1)
    reject(new Error('Connection dropped'))
    await flushPromises()
    expect(p.paste).toHaveBeenCalledTimes(2)
    expect(mocks.prepare).toHaveBeenCalledTimes(2)
    expect(useToasts().toasts.value.some((toast) => toast.message === 'Connection dropped')).toBe(true)
  })

  it('ignores hidden or disposed drop targets and aborts pending requests on disposal', async () => {
    mocks.prepare.mockReturnValue(new Promise(() => {}))
    const p = pane()
    p.parent.hidden = true
    drop(p.host)
    await flushPromises()
    expect(mocks.prepare).not.toHaveBeenCalled()
    p.parent.hidden = false
    drop(p.host)
    await flushPromises()
    const id = p.host.id
    p.dispose()
    expect(mocks.prepare.mock.calls[0][1].aborted).toBe(true)
    mocks.drop!({ data: { target: id, paths: ['/one.png'] } })
    expect(mocks.prepare).toHaveBeenCalledOnce()
  })

  it('shows upload failures without sending anything to the terminal', async () => {
    mocks.prepare.mockRejectedValue(new Error('Image exceeds 20 MiB'))
    const p = pane()
    drop(p.host)
    await flushPromises()
    expect(p.paste).not.toHaveBeenCalled()
    expect(useToasts().toasts.value.map((toast) => toast.message)).toEqual(['Image exceeds 20 MiB'])
  })
})
