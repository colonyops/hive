import { describe, expect, it } from 'vitest'
import { themes } from '../themes'
import { codeThemes } from '../codeThemes'

describe('codeThemes', () => {
  it('loads a registration named after itself for every app theme', async () => {
    for (const theme of themes) {
      const code = codeThemes[theme]
      const registration = await code.load()
      expect(registration.name, theme).toBe(code.name)
    }
  })

  it('pairs a light app theme with a light code theme and a dark one with a dark code theme', async () => {
    for (const theme of themes) {
      const light = /light|latte|day/.test(theme)
      const registration = await codeThemes[theme].load()
      expect(registration.type, theme).toBe(light ? 'light' : 'dark')
    }
  })

  it('gives every Hive palette a colour for each token kind', async () => {
    for (const theme of ['dark', 'light', 'midnight', 'slate', 'slate-light'] as const) {
      const registration = await codeThemes[theme].load()
      const scopes = new Set(registration.tokenColors!.flatMap((rule) => rule.scope ?? []))
      for (const scope of ['comment', 'keyword', 'string', 'constant.numeric', 'entity.name.function', 'storage.type'])
        expect(scopes.has(scope), `${theme} ${scope}`).toBe(true)
    }
  })
})
