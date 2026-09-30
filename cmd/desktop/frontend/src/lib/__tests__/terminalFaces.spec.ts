import { beforeEach, describe, expect, it, vi } from 'vitest'
import {
  loadTerminalFaces,
  resetTerminalFacesForTests,
  SYMBOL_FONT,
  TERMINAL_FONT,
  terminalFontStack,
} from '../terminalFaces'

describe('terminalFontStack', () => {
  it('falls back to the bundled face when nothing is chosen', () => {
    expect(terminalFontStack('')).toBe(terminalFontStack(TERMINAL_FONT))
    expect(terminalFontStack('')).toContain(`'${TERMINAL_FONT}'`)
  })

  it('treats whitespace as nothing chosen', () => {
    expect(terminalFontStack('   ')).toBe(terminalFontStack(''))
  })

  // A family the OS reports but the webview cannot resolve — or one
  // uninstalled since it was picked — has to land on the shipped text face
  // rather than on whatever `monospace` happens to be.
  it('backs a chosen family with the bundled face, not just a generic', () => {
    const stack = terminalFontStack('Menlo')

    expect(stack.indexOf("'Menlo'")).toBeLessThan(stack.indexOf(`'${TERMINAL_FONT}'`))
    expect(stack).toContain('monospace')
  })

  // The text faces carry no icons, so the symbol face has to sit behind both of
  // them and in front of the generics, or a TUI's icons become tofu.
  it('puts the symbol face behind every text face', () => {
    for (const stack of [terminalFontStack(''), terminalFontStack('Menlo')]) {
      expect(stack.indexOf(`'${TERMINAL_FONT}'`)).toBeLessThan(stack.indexOf(`'${SYMBOL_FONT}'`))
      expect(stack.indexOf(`'${SYMBOL_FONT}'`)).toBeLessThan(stack.indexOf('ui-monospace'))
    }
  })

  it('does not repeat the bundled face when it is the chosen family', () => {
    expect(terminalFontStack(TERMINAL_FONT).match(new RegExp(TERMINAL_FONT, 'g'))).toHaveLength(1)
  })

  // The stack reaches xterm inside a canvas font shorthand, where an unquoted
  // family starting with a digit is a parse error — and a failed parse leaves
  // ctx.font at its previous value rather than throwing, so the terminal would
  // silently render in the wrong font.
  it('quotes family names that would not parse bare', () => {
    expect(terminalFontStack('3270Medium Nerd Font')).toContain("'3270Medium Nerd Font'")
    expect(terminalFontStack("Fred's Mono")).toContain('"Fred\'s Mono"')
  })
})

describe('loadTerminalFaces', () => {
  beforeEach(() => {
    resetTerminalFacesForTests()
    Object.defineProperty(document, 'fonts', {
      configurable: true,
      value: { load: vi.fn().mockResolvedValue([]) },
    })
  })

  // xterm measures its cell once on open and the atlas caches what was
  // resident, so a bold or italic face arriving late stays wrong. ADR terminal-atlas-renderer.
  it('loads both weights and the italic of each', async () => {
    await loadTerminalFaces('', 13, 300, 700)

    const stack = terminalFontStack('')
    expect((document.fonts.load as ReturnType<typeof vi.fn>).mock.calls.map(([spec]) => spec)).toEqual([
      `300 13px ${stack}`,
      `italic 300 13px ${stack}`,
      `700 13px ${stack}`,
      `italic 700 13px ${stack}`,
      `300 13px '${SYMBOL_FONT}'`,
    ])
  })

  // Two ways this call can silently fetch nothing. The default text is a space,
  // which the symbol face has no glyph for; and document.fonts.load matches on
  // unicode-range rather than on real coverage, so asking through the stack
  // resolves to the text face and leaves this one unfetched. Canvas does not
  // load a font on demand, so either mistake ends as tofu the atlas caches for
  // the pane's life.
  it('warms the symbol face by name, with a glyph only it carries', async () => {
    await loadTerminalFaces('', 13, 300, 700)

    const [spec, sample] = (document.fonts.load as ReturnType<typeof vi.fn>).mock.calls.at(-1) ?? []
    expect(spec).toContain(SYMBOL_FONT)
    expect(spec).not.toContain(TERMINAL_FONT)
    expect(sample.codePointAt(0)).toBeGreaterThanOrEqual(0xe000)
  })

  // document.fonts.load re-resolves on every call — tens of milliseconds even
  // for a resident face — and every pane awaits this before it may ask for a
  // terminal.
  it('resolves a repeat request from cache', async () => {
    await loadTerminalFaces('', 13, 300, 700)
    await loadTerminalFaces('', 13, 300, 700)

    expect(document.fonts.load).toHaveBeenCalledTimes(5)
  })

  it('reloads when the family, size, or a weight changes', async () => {
    await loadTerminalFaces('', 13, 300, 700)
    await loadTerminalFaces('Menlo', 13, 300, 700)
    await loadTerminalFaces('', 16, 300, 700)
    await loadTerminalFaces('', 13, 400, 700)

    expect(document.fonts.load).toHaveBeenCalledTimes(20)
  })

  // A font that never loads must not stop a pane from opening.
  it('resolves when a face fails to load', async () => {
    Object.defineProperty(document, 'fonts', {
      configurable: true,
      value: { load: vi.fn().mockRejectedValue(new Error('no such face')) },
    })

    await expect(loadTerminalFaces('Absent Mono', 13, 300, 700)).resolves.toBeUndefined()
  })
})
