import type { ThemeRegistration } from 'shiki/core'
import type { Theme } from '../composables/useTheme'

/** A code theme for one app theme: the registration's name, and how to get it. */
export interface CodeTheme {
  name: string
  load: () => Promise<ThemeRegistration>
}

/**
 * The hues a Hive syntax theme is drawn with. bg, fg, comment and punctuation
 * mirror the app theme's raised, text, text-3 and text-2 tokens in
 * styles/main.css; a palette edit there is a palette edit here. Shiki inlines
 * hex colours, so these cannot be CSS variables.
 */
interface Palette {
  bg: string
  fg: string
  comment: string
  punctuation: string
  keyword: string
  string: string
  number: string
  func: string
  type: string
  property: string
}

function syntaxTheme(name: string, type: 'dark' | 'light', p: Palette): ThemeRegistration {
  return {
    name,
    type,
    colors: { 'editor.background': p.bg, 'editor.foreground': p.fg },
    tokenColors: [
      { scope: ['comment', 'punctuation.definition.comment'], settings: { foreground: p.comment } },
      { scope: ['keyword', 'storage', 'storage.modifier'], settings: { foreground: p.keyword } },
      { scope: ['keyword.operator', 'punctuation', 'meta.brace'], settings: { foreground: p.punctuation } },
      { scope: ['string', 'punctuation.definition.string', 'string.template'], settings: { foreground: p.string } },
      {
        scope: ['constant.numeric', 'constant.language', 'constant.character', 'constant.other', 'variable.language'],
        settings: { foreground: p.number },
      },
      { scope: ['entity.name.function', 'support.function', 'meta.function-call'], settings: { foreground: p.func } },
      {
        scope: [
          'storage.type',
          'entity.name.type',
          'entity.name.class',
          'entity.name.struct',
          'entity.name.namespace',
          'support.type',
          'support.class',
        ],
        settings: { foreground: p.type },
      },
      { scope: ['variable', 'variable.parameter'], settings: { foreground: p.fg } },
      {
        scope: [
          'variable.other.property',
          'variable.other.member',
          'variable.other.object.property',
          'entity.other.attribute-name',
          'support.type.property-name',
          'meta.object-literal.key',
        ],
        settings: { foreground: p.property },
      },
      {
        scope: ['entity.name.tag', 'markup.heading', 'punctuation.definition.heading'],
        settings: { foreground: p.keyword },
      },
      { scope: ['markup.bold'], settings: { fontStyle: 'bold' } },
      { scope: ['markup.italic'], settings: { fontStyle: 'italic' } },
      { scope: ['markup.inserted'], settings: { foreground: p.string } },
      { scope: ['markup.deleted'], settings: { foreground: p.keyword } },
      { scope: ['markup.inline.raw', 'markup.raw'], settings: { foreground: p.type } },
    ],
  }
}

function custom(registration: ThemeRegistration): CodeTheme {
  return { name: registration.name!, load: () => Promise.resolve(registration) }
}

function bundled(name: string, load: () => Promise<{ default: ThemeRegistration }>): CodeTheme {
  return { name, load: () => load().then((module) => module.default) }
}

// The Hive Signal palettes: cool neutrals and amber as the one warm hue, so
// amber marks keywords and the rest stays in sage, cyan, blue and lavender.
const hiveDark = syntaxTheme('hive-dark', 'dark', {
  bg: '#1b2029',
  fg: '#e9edf4',
  comment: '#7a8698',
  punctuation: '#a6b0c0',
  keyword: '#f5b23f',
  string: '#9fcf9f',
  number: '#c9a9ff',
  func: '#85b6f5',
  type: '#6fcbe0',
  property: '#c9d2e0',
})

const hiveLight = syntaxTheme('hive-light', 'light', {
  bg: '#eceff4',
  fg: '#121722',
  comment: '#6a7688',
  punctuation: '#4b5568',
  keyword: '#a8660f',
  string: '#2f7d4f',
  number: '#6a3fd6',
  func: '#1a5fc2',
  type: '#0b7285',
  property: '#2d3747',
})

const hiveMidnight = syntaxTheme('hive-midnight', 'dark', {
  bg: '#141829',
  fg: '#eaf0ff',
  comment: '#7d88b3',
  punctuation: '#8791b8',
  keyword: '#f5a623',
  string: '#9ed3a6',
  number: '#c8a8ff',
  func: '#86b4ff',
  type: '#6fd0ea',
  property: '#c6cfee',
})

// Slate has a blue accent and no amber, so blue marks keywords and gold is
// kept for numbers.
const hiveSlate = syntaxTheme('hive-slate', 'dark', {
  bg: '#1a1f2a',
  fg: '#e8ecf4',
  comment: '#8390a6',
  punctuation: '#a2adc0',
  keyword: '#7aa7f8',
  string: '#9fcf9f',
  number: '#e8b86d',
  func: '#c9a9ff',
  type: '#6fcbe0',
  property: '#c9d2e0',
})

const hiveSlateLight = syntaxTheme('hive-slate-light', 'light', {
  bg: '#e7ebf1',
  fg: '#10151c',
  comment: '#6b7688',
  punctuation: '#4a5567',
  keyword: '#1653b8',
  string: '#2f7d4f',
  number: '#a8660f',
  func: '#6a3fd6',
  type: '#0b7285',
  property: '#2d3747',
})

// Shiki bundles Tokyo Night and Nord but neither light variant; these use
// the published Tokyo Night Day hues and the Nord palette darkened for paper.
const tokyoNightDay = syntaxTheme('tokyo-night-day', 'light', {
  bg: '#eff0f5',
  fg: '#3760bf',
  comment: '#848cb5',
  punctuation: '#6172b0',
  keyword: '#9854f1',
  string: '#587539',
  number: '#b15c00',
  func: '#2e7de9',
  type: '#007197',
  property: '#387068',
})

const nordLight = syntaxTheme('nord-light', 'light', {
  bg: '#eceff4',
  fg: '#2e3440',
  comment: '#6f7c95',
  punctuation: '#4c566a',
  keyword: '#5e81ac',
  string: '#4d7a3e',
  number: '#8f5e96',
  func: '#2d7c8f',
  type: '#2f8a8a',
  property: '#3b4252',
})

/** One code theme per app theme, so a fence takes the colours of the theme around it. */
export const codeThemes: Record<Theme, CodeTheme> = {
  dark: custom(hiveDark),
  light: custom(hiveLight),
  midnight: custom(hiveMidnight),
  slate: custom(hiveSlate),
  'slate-light': custom(hiveSlateLight),
  gruvbox: bundled('gruvbox-dark-medium', () => import('shiki/themes/gruvbox-dark-medium.mjs')),
  'one-dark': bundled('one-dark-pro', () => import('shiki/themes/one-dark-pro.mjs')),
  'one-light': bundled('one-light', () => import('shiki/themes/one-light.mjs')),
  'tokyo-night': bundled('tokyo-night', () => import('shiki/themes/tokyo-night.mjs')),
  'tokyo-night-day': custom(tokyoNightDay),
  'catppuccin-mocha': bundled('catppuccin-mocha', () => import('shiki/themes/catppuccin-mocha.mjs')),
  'catppuccin-latte': bundled('catppuccin-latte', () => import('shiki/themes/catppuccin-latte.mjs')),
  nord: bundled('nord', () => import('shiki/themes/nord.mjs')),
  'nord-light': custom(nordLight),
}
