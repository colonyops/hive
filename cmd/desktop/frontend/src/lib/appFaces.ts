// The app chrome's two font stacks. The terminal builds its own
// (lib/terminalFaces.ts) and shares nothing with these — a family chosen here
// never reaches a pane, which is what lets the chrome be proportional while
// the terminal stays fixed-pitch.

// The bundled faces (ADR bundled-faces-are-jetbrains-mono-inter-and-a-symbol-font). Inter is declared by
// @fontsource-variable/inter as "Inter Variable", so it does not collide with a
// separately installed static Inter — which the picker offers as its own entry.
const SANS_BUNDLED = '"Inter Variable", system-ui, sans-serif'
const MONO_BUNDLED = '"JetBrains Mono", ui-monospace, monospace'

/**
 * The stacks `--font-sans` and `--font-mono` are set to. A chosen family leads
 * and the bundled stack backs it, so a name the OS scan reported but the
 * webview cannot resolve — or one uninstalled since it was picked — degrades to
 * the shipped face rather than to whatever `sans-serif` happens to be.
 */
export function appFontStacks(sans: string, mono: string): { sans: string; mono: string } {
  return { sans: stack(sans, SANS_BUNDLED), mono: stack(mono, MONO_BUNDLED) }
}

function stack(family: string, bundled: string): string {
  const chosen = family.trim()
  if (!chosen) return bundled
  return `${cssFamily(chosen)}, ${bundled}`
}

// The System options persist a generic keyword rather than a family name, and
// quoting one turns it into a literal family nothing resolves.
const GENERIC_FAMILIES = new Set([
  'system-ui',
  'ui-sans-serif',
  'ui-monospace',
  'ui-serif',
  'sans-serif',
  'monospace',
  'serif',
  'cursive',
  'fantasy',
])

// A family name arrives from an OS font scan or a hand-edited settings.yaml, so
// it can hold anything. Quoting keeps a name with spaces, digits, or a stray
// token from ending the declaration early — an invalid one drops the whole
// custom-property value, which would take the bundled fallback down with it.
function cssFamily(family: string): string {
  if (GENERIC_FAMILIES.has(family.toLowerCase())) return family
  return `"${family.replace(/["\\\n\r]/g, '')}"`
}
