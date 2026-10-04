/**
 * Which canvas a canvas surface is pointed at. A null name lets the default
 * win: the most recent canvas of `session`, else the workspace's most recent.
 */
export interface CanvasScope {
  workspace: string
  name: string | null
  session: number | null
}

export type CanvasLinkTarget = { kind: 'url'; url: string } | { kind: 'canvas'; name: string }

/**
 * Where a link on a canvas leads. A web or mail address opens outside the app,
 * and a relative reference that names a canvas in the same workspace opens that
 * canvas. Anything else leads nowhere, because the webview must never navigate.
 */
export function canvasLinkTarget(href: string, canvasNames: ReadonlySet<string>): CanvasLinkTarget | null {
  if (/^(https?:|mailto:)/i.test(href)) return { kind: 'url', url: href }
  const name = href.replace(/^\.\//, '')
  return canvasNames.has(name) ? { kind: 'canvas', name } : null
}
