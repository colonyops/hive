export const themes = [
  'dark',
  'light',
  'midnight',
  'gruvbox',
  'slate',
  'slate-light',
  'one-dark',
  'one-light',
  'tokyo-night',
  'tokyo-night-day',
  'catppuccin-mocha',
  'catppuccin-latte',
  'nord',
  'nord-light',
] as const
export type Theme = (typeof themes)[number]

export const lightThemes: ReadonlySet<Theme> = new Set<Theme>([
  'light',
  'slate-light',
  'one-light',
  'tokyo-night-day',
  'catppuccin-latte',
  'nord-light',
])
