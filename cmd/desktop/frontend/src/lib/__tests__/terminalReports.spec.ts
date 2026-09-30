import { describe, expect, it } from 'vitest'
import { Terminal } from '@xterm/xterm'
import { silenceDeviceReports } from '../terminalReports'

// What xterm answers is xterm's behaviour, not ours, so these drive a real
// emulator: a version bump that adds a reply to some other query should fail
// here rather than turn up as characters on someone's prompt.

function harness(): { term: Terminal; replies: () => string } {
  const term = new Terminal({ cols: 20, rows: 5 })
  let sent = ''
  term.onData((data) => { sent += data })
  return { term, replies: () => sent }
}

function write(term: Terminal, data: string): Promise<void> {
  return new Promise((resolve) => term.write(data, () => resolve()))
}

const QUERIES = {
  'primary device attributes': '\x1b[c',
  'secondary device attributes': '\x1b[>c',
  'cursor position report': '\x1b[6n',
  'operating status report': '\x1b[5n',
  'private cursor position report': '\x1b[?6n',
  'DECRQM': '\x1b[?2026$p',
  'DECRQSS': '\x1bP$q"q\x1b\\',
}

// Colour queries are answered off the theme service, which a terminal that was
// never opened has not built — so they cannot be driven end to end here and are
// covered by the handler-order test instead. Everything else must actually
// answer, or the suppression case below would pass on a sequence xterm ignores.
describe('the replies being suppressed', () => {
  for (const [name, sequence] of Object.entries(QUERIES)) {
    it(`xterm answers the ${name} on its own`, async () => {
      const { term, replies } = harness()
      await write(term, sequence)
      expect(replies()).not.toBe('')
    })
  }

  it('answers a device attributes request with the bytes seen on the prompt', async () => {
    const { term, replies } = harness()
    await write(term, '\x1b[c')
    expect(replies()).toBe('\x1b[?1;2c')
  })
})

describe('silenceDeviceReports', () => {
  for (const [name, sequence] of Object.entries(QUERIES)) {
    it(`swallows the ${name}`, async () => {
      const { term, replies } = harness()
      silenceDeviceReports(term)
      await write(term, sequence)
      expect(replies()).toBe('')
    })
  }

  it('consumes a colour query and lets a colour set through', async () => {
    const { term } = harness()
    const reached: string[] = []
    // Registered first, so the parser tries it *after* the suppressor.
    term.parser.registerOscHandler(11, (data) => { reached.push(data); return true })
    silenceDeviceReports(term)

    await write(term, '\x1b]11;?\x07')
    expect(reached).toEqual([])

    await write(term, '\x1b]11;rgb:00/ff/00\x07')
    expect(reached).toEqual(['rgb:00/ff/00'])
  })

  it('still forwards what the user types', async () => {
    const { term, replies } = harness()
    silenceDeviceReports(term)
    term.input('ls\r')
    expect(replies()).toBe('ls\r')
  })

  it('restores answering on dispose', async () => {
    const { term, replies } = harness()
    silenceDeviceReports(term).dispose()
    await write(term, '\x1b[c')
    expect(replies()).toBe('\x1b[?1;2c')
  })
})
