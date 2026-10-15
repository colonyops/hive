import type { MermaidConfig } from 'mermaid'

export interface MermaidPalette {
  background: string
  surface: string
  surfaceAlt: string
  border: string
  text: string
  mutedText: string
  accent: string
  darkMode: boolean
  fontFamily: string
}

let modulePromise: Promise<typeof import('mermaid').default> | null = null
let renderTail: Promise<void> = Promise.resolve()
let nextID = 0

function loadMermaid(): Promise<typeof import('mermaid').default> {
  modulePromise ??= import('mermaid').then((module) => module.default)
  return modulePromise
}

function config(palette: MermaidPalette): MermaidConfig {
  return {
    startOnLoad: false,
    securityLevel: 'strict',
    secure: [
      'secure',
      'securityLevel',
      'startOnLoad',
      'maxTextSize',
      'maxEdges',
      'htmlLabels',
      'suppressErrorRendering',
      'dompurifyConfig',
      'theme',
      'themeVariables',
      'themeCSS',
      'fontFamily',
      'altFontFamily',
      'darkMode',
    ],
    suppressErrorRendering: true,
    maxTextSize: 50 * 1024,
    maxEdges: 500,
    htmlLabels: false,
    theme: 'base',
    fontFamily: palette.fontFamily,
    themeVariables: {
      darkMode: palette.darkMode,
      background: palette.background,
      primaryColor: palette.surface,
      primaryTextColor: palette.text,
      primaryBorderColor: palette.border,
      secondaryColor: palette.surfaceAlt,
      secondaryTextColor: palette.text,
      secondaryBorderColor: palette.border,
      tertiaryColor: palette.background,
      tertiaryTextColor: palette.text,
      tertiaryBorderColor: palette.border,
      lineColor: palette.mutedText,
      textColor: palette.text,
      mainBkg: palette.surface,
      nodeBorder: palette.border,
      clusterBkg: palette.surfaceAlt,
      clusterBorder: palette.border,
      edgeLabelBackground: palette.background,
      actorBkg: palette.surface,
      actorBorder: palette.border,
      actorTextColor: palette.text,
      signalColor: palette.mutedText,
      signalTextColor: palette.text,
      labelBoxBkgColor: palette.surface,
      labelBoxBorderColor: palette.border,
      labelTextColor: palette.text,
      noteBkgColor: palette.surfaceAlt,
      noteBorderColor: palette.accent,
      noteTextColor: palette.text,
      activationBkgColor: palette.surfaceAlt,
      activationBorderColor: palette.border,
    },
  }
}

export function renderMermaid(source: string, palette: MermaidPalette): Promise<string> {
  const render = renderTail.then(async () => {
    const mermaid = await loadMermaid()
    mermaid.initialize(config(palette))
    const result = await mermaid.render(`hive-mermaid-${nextID++}`, source)
    return result.svg
  })
  renderTail = render.then(
    () => undefined,
    () => undefined,
  )
  return render
}
