import type { ITheme } from '@xterm/xterm'

// The app's palette lives in CSS custom properties that the theme switcher
// swaps on <html> (src/styles/main.css). xterm.js paints to a canvas and can
// not read them, so every colour it needs — chrome and the 16-colour ANSI
// set — is sampled here and re-applied whenever the theme changes.
const ANSI_VARS = {
  black: '--hv-term-black',
  red: '--hv-term-red',
  green: '--hv-term-green',
  yellow: '--hv-term-yellow',
  blue: '--hv-term-blue',
  magenta: '--hv-term-magenta',
  cyan: '--hv-term-cyan',
  white: '--hv-term-white',
  brightBlack: '--hv-term-bright-black',
  brightRed: '--hv-term-bright-red',
  brightGreen: '--hv-term-bright-green',
  brightYellow: '--hv-term-bright-yellow',
  brightBlue: '--hv-term-bright-blue',
  brightMagenta: '--hv-term-bright-magenta',
  brightCyan: '--hv-term-bright-cyan',
  brightWhite: '--hv-term-bright-white',
} as const

// The dark theme's values, for contexts with no styled document.
const FALLBACK: ITheme = {
  background: '#101318',
  foreground: '#c4c8cf',
  cursor: '#f5b23f',
  cursorAccent: '#101318',
  selectionBackground: 'rgba(245, 178, 63, 0.35)',
  black: '#1b2029',
  red: '#ef7183',
  green: '#8fd67f',
  yellow: '#e6b45a',
  blue: '#6ba8f0',
  magenta: '#b98ef0',
  cyan: '#5fc8de',
  white: '#b8c1cf',
  brightBlack: '#55606f',
  brightRed: '#ff8b9b',
  brightGreen: '#a6e695',
  brightYellow: '#f5c973',
  brightBlue: '#86bcff',
  brightMagenta: '#cba6ff',
  brightCyan: '#7adcf0',
  brightWhite: '#e9edf4',
}

function cssColor(style: CSSStyleDeclaration | null, name: string, fallback: string): string {
  if (!style) return fallback
  return style.getPropertyValue(name).trim() || fallback
}

function documentStyle(): CSSStyleDeclaration | null {
  return typeof getComputedStyle === 'function' && typeof document !== 'undefined'
    ? getComputedStyle(document.documentElement)
    : null
}

/**
 * The two colours search highlighting paints with: every match, then the one
 * the viewport is on. xterm parses a decoration colour itself rather than
 * handing it to CSS and accepts `#RRGGBB` alone, so a theme that spells one any
 * other way falls back rather than losing the highlight.
 */
export function searchHighlightColors(): { match: string; active: string } {
  const style = documentStyle()
  return {
    match: hexOrFallback(cssColor(style, '--hv-term-bright-black', ''), FALLBACK.brightBlack!),
    active: hexOrFallback(cssColor(style, '--hv-term-yellow', ''), FALLBACK.yellow!),
  }
}

function hexOrFallback(color: string, fallback: string): string {
  return /^#[0-9a-f]{6}$/i.test(color) ? color : fallback
}

/** The xterm palette for the theme currently applied to the document. */
export function xtermTheme(): ITheme {
  const style = documentStyle()
  const background = cssColor(style, '--hv-app', FALLBACK.background!)
  const theme: ITheme = {
    background,
    // Every theme must set --hv-term-foreground. :root defines it, so an
    // unset theme inherits the dark value rather than falling through to the
    // --hv-text default below — which is how light themes ended up painting a
    // 1.7:1 grey on white.
    foreground: cssColor(style, '--hv-term-foreground', cssColor(style, '--hv-text', FALLBACK.foreground!)),
    cursor: cssColor(style, '--hv-accent', FALLBACK.cursor!),
    cursorAccent: background,
    // --hv-selection is tuned for a 10% overlay on app chrome; a terminal
    // selection has to stay legible over arbitrary output, so it is the
    // theme's accent at a stronger alpha. xterm parses the colour itself and
    // takes #RRGGBBAA, so the alpha is appended rather than mixed in CSS.
    selectionBackground: `${hexOrFallback(cssColor(style, '--hv-accent', ''), FALLBACK.cursor!)}59`,
  }
  for (const key of Object.keys(ANSI_VARS) as (keyof typeof ANSI_VARS)[]) {
    theme[key] = cssColor(style, ANSI_VARS[key], FALLBACK[key]!)
  }
  return theme
}
