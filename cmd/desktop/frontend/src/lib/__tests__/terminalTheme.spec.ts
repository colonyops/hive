import { afterEach, describe, expect, it } from 'vitest'
import { xtermTheme } from '../terminalTheme'

describe('xtermTheme', () => {
  afterEach(() => {
    document.documentElement.removeAttribute('style')
  })

  it('samples the palette applied to the document', () => {
    const root = document.documentElement.style
    root.setProperty('--hv-app', '#101010')
    root.setProperty('--hv-text', '#eeeeee')
    root.setProperty('--hv-term-foreground', '#c4c8cf')
    root.setProperty('--hv-accent', '#ff8800')
    root.setProperty('--hv-term-red', '#cc241d')
    root.setProperty('--hv-term-bright-white', '#ebdbb2')

    const theme = xtermTheme()
    expect(theme.background).toBe('#101010')
    expect(theme.foreground).toBe('#c4c8cf')
    expect(theme.cursor).toBe('#ff8800')
    expect(theme.cursorAccent).toBe('#101010')
    expect(theme.red).toBe('#cc241d')
    expect(theme.brightWhite).toBe('#ebdbb2')
  })

  // Unstyled output takes the terminal's own foreground over the UI text
  // colour, which is tuned for small labels on panels and is glare across a
  // screen of monospace.
  it('prefers the terminal foreground over the UI text colour', () => {
    const root = document.documentElement.style
    root.setProperty('--hv-app', '#0e1116')
    root.setProperty('--hv-text', '#e8ecf4')
    root.setProperty('--hv-term-foreground', '#c2c9d8')

    expect(xtermTheme().foreground).toBe('#c2c9d8')
  })

  // The light themes define no terminal foreground on purpose: their ANSI
  // white is a pale grey that would all but vanish on a white background, so
  // falling through to the UI text colour is the correct outcome, not a gap.
  it('falls through to the UI text colour when a theme defines no terminal foreground', () => {
    const root = document.documentElement.style
    root.setProperty('--hv-app', '#ffffff')
    root.setProperty('--hv-text', '#121722')

    expect(xtermTheme().foreground).toBe('#121722')
  })

  it('falls back to the dark palette for variables the document does not define', () => {
    const theme = xtermTheme()
    expect(theme.background).toBe('#101318')
    expect(theme.foreground).toBe('#c4c8cf')
    expect(theme.blue).toBe('#6ba8f0')
    expect(theme.brightBlack).toBe('#55606f')
  })
})
