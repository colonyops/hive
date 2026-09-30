// The faces themselves are declared from styles/main.css, not imported here:
// JetBrains Mono is the app's mono as well as the terminal's, so declaring it
// from this lazily-loaded module would emit the @font-face block twice.

// The text face every terminal pane falls back to, and the name the OS font
// scan reports it as.
export const TERMINAL_FONT = 'JetBrains Mono'

// Agent TUIs draw powerline and devicon glyphs no text font covers. This one
// covers nothing else, so it sits behind whatever renders the text (ADR bundled-faces-are-jetbrains-mono-inter-and-a-symbol-font).
export const SYMBOL_FONT = 'Symbols Nerd Font Mono'

// A private-use glyph, to warm the symbol face. It carries no space and no
// Latin, so the space document.fonts.load defaults to would never reach it.
const SYMBOL_SAMPLE = ''

const FALLBACKS = "ui-monospace, 'SF Mono', Menlo, Consolas, monospace"

/**
 * The `fontFamily` xterm is given. A chosen system family leads, the bundled
 * text face backs it, and the symbol face backs both: a family the OS reports
 * but the webview cannot resolve — or one uninstalled since it was picked —
 * degrades to the shipped font rather than to whatever `monospace` happens to
 * be, and a chosen family without icon glyphs still renders a TUI's icons.
 */
export function terminalFontStack(family: string): string {
  const chosen = family.trim()
  const backing = `${quoted(TERMINAL_FONT)}, ${quoted(SYMBOL_FONT)}, ${FALLBACKS}`
  if (!chosen || chosen === TERMINAL_FONT) return backing
  return `${quoted(chosen)}, ${backing}`
}

// A family name reaches xterm inside a canvas font shorthand, and an unquoted
// name that starts with a digit or holds a stray token is a parse error there —
// which leaves ctx.font at its previous value rather than throwing.
function quoted(family: string): string {
  return family.includes("'") ? `"${family}"` : `'${family}'`
}

// Kept per (stack, size, weight pair) because document.fonts.load re-resolves
// on every call — 12ms to 47ms measured, even for a face already resident — and
// every pane awaits it before it may so much as ask for a terminal.
const faceLoads = new Map<string, Promise<void>>()

/**
 * Make every face a pane can draw with resident before it opens.
 *
 * xterm measures its cell when a Terminal opens and never re-measures when a
 * face arrives later, and an atlas renderer caches the glyphs it rasterised
 * from whatever was resident (ADR terminal-atlas-renderer) — so both weights belong here, not just
 * the normal one, the italics of each with them, and the symbol face too: an
 * icon rasterised before it arrives is cached as tofu for the pane's life.
 */
export function loadTerminalFaces(
  family: string,
  px: number,
  weight: number,
  weightBold: number,
): Promise<void> {
  const stack = terminalFontStack(family)
  const key = `${stack}|${px}|${weight}|${weightBold}`
  const loaded = faceLoads.get(key)
  if (loaded) return loaded

  const specs = [weight, weightBold].flatMap((value) => [
    `${value} ${px}px ${stack}`,
    `italic ${value} ${px}px ${stack}`,
  ])
  const pending = Promise.all([
    ...specs.map((spec) => document.fonts?.load(spec).catch(() => {})),
    // The symbol face is asked for by name rather than through the stack.
    // document.fonts.load matches on the unicode-range descriptor, not on what
    // a face actually carries, and these faces declare none — so every family
    // reads as covering everything and a stack always resolves to its first
    // one. Asked for through the stack, this call fetches the text face a
    // second time and the symbol face never at all.
    document.fonts?.load(`${weight} ${px}px ${quoted(SYMBOL_FONT)}`, SYMBOL_SAMPLE).catch(() => {}),
  ]).then(() => {})
  faceLoads.set(key, pending)
  return pending
}

export function resetTerminalFacesForTests(): void {
  faceLoads.clear()
}
