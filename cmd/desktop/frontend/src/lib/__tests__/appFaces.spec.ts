import { describe, expect, it } from 'vitest'
import { appFontStacks } from '../appFaces'

describe('appFontStacks', () => {
  it('falls back to the bundled faces when nothing is chosen', () => {
    const { sans, mono } = appFontStacks('', '')

    expect(sans).toBe('"Inter Variable", system-ui, sans-serif')
    expect(mono).toBe('"JetBrains Mono", ui-monospace, monospace')
  })

  // The acceptance rule for a family the OS reported but the webview cannot
  // resolve: it degrades to the shipped face, never to a bare generic.
  it('backs a chosen family with the bundled stack', () => {
    const { sans, mono } = appFontStacks('Helvetica Neue', 'Fira Code')

    expect(sans).toBe('"Helvetica Neue", "Inter Variable", system-ui, sans-serif')
    expect(mono).toBe('"Fira Code", "JetBrains Mono", ui-monospace, monospace')
  })

  // Quoting a generic keyword turns it into a literal family name nothing
  // resolves, which is what the System options persist.
  it('leaves generic keywords unquoted', () => {
    const { sans, mono } = appFontStacks('system-ui', 'ui-monospace')

    expect(sans.startsWith('system-ui, ')).toBe(true)
    expect(mono.startsWith('ui-monospace, ')).toBe(true)
  })

  // A name reaches here from an OS font scan or a hand-edited settings.yaml. An
  // unbalanced quote would make the whole custom property invalid, taking the
  // bundled fallback down with the bad name.
  it('neutralises quotes and escapes in a family name', () => {
    const { sans } = appFontStacks('Ev"il\\ Sans', '')

    expect(sans).toBe('"Evil Sans", "Inter Variable", system-ui, sans-serif')
  })

  it('ignores surrounding whitespace', () => {
    expect(appFontStacks('  ', '  ').sans).toBe('"Inter Variable", system-ui, sans-serif')
    expect(appFontStacks(' Iosevka ', '').sans).toBe('"Iosevka", "Inter Variable", system-ui, sans-serif')
  })
})
