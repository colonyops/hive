import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  newTerminalWindow,
  overrideTerminalTreeHandles,
  setTerminalTreeHandles,
  type TerminalTreeHandles,
} from '../terminalTree'

afterEach(() => setTerminalTreeHandles(null))
describe('terminal keyboard target', () => {
  it('routes commands to the visible remote, then restores the local handles', () => {
    const local = vi.fn()
    const remote = vi.fn()
    let localActive = false
    setTerminalTreeHandles({ newWindow: local } as unknown as TerminalTreeHandles, () => localActive)
    newTerminalWindow()
    expect(local).not.toHaveBeenCalled()
    const release = overrideTerminalTreeHandles({ newWindow: remote })
    newTerminalWindow()
    expect(remote).toHaveBeenCalledOnce()
    expect(local).not.toHaveBeenCalled()
    release()
    localActive = true
    newTerminalWindow()
    expect(local).toHaveBeenCalledOnce()
  })
  it('an old pane cleanup does not discard the new pane handles', () => {
    const first = overrideTerminalTreeHandles({ newWindow: vi.fn() })
    const next = vi.fn()
    const second = overrideTerminalTreeHandles({ newWindow: next })
    first()
    newTerminalWindow()
    expect(next).toHaveBeenCalledOnce()
    second()
  })
})
