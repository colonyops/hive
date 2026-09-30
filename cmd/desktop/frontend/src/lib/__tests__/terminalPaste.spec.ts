import { describe, expect, it, vi } from 'vitest'
import { interceptPaste } from '../terminalPaste'

function pasteEvent(text: string): Event {
  const event = new Event('paste', { bubbles: true, cancelable: true })
  Object.defineProperty(event, 'clipboardData', { value: { getData: () => text } })
  return event
}

describe('interceptPaste', () => {
  function pane() {
    const host = document.createElement('div')
    // Stands in for xterm's hidden textarea: the paste's real target, and where
    // xterm registers the handler this has to win against.
    const textarea = document.createElement('textarea')
    host.append(textarea)
    document.body.append(host)
    return { host, textarea }
  }

  it('takes the paste before the handler inside the host', () => {
    const { host, textarea } = pane()
    const xterm = vi.fn()
    textarea.addEventListener('paste', xterm)

    const pasted: string[] = []
    const release = interceptPaste(host, (text) => pasted.push(text))

    textarea.dispatchEvent(pasteEvent('one\ntwo'))
    expect(pasted).toEqual(['one\ntwo'])
    expect(xterm).not.toHaveBeenCalled()

    release()
    textarea.dispatchEvent(pasteEvent('after'))
    expect(pasted).toEqual(['one\ntwo'])
    expect(xterm).toHaveBeenCalledOnce()
  })

  it('leaves an empty paste to xterm', () => {
    const { host, textarea } = pane()
    const xterm = vi.fn()
    textarea.addEventListener('paste', xterm)
    interceptPaste(host, () => expect.unreachable('an empty paste is not sent'))

    textarea.dispatchEvent(pasteEvent(''))
    expect(xterm).toHaveBeenCalledOnce()
  })
})
