import { beforeEach, describe, expect, it, vi } from 'vitest'
import { renderMermaid, type MermaidPalette } from '../mermaid'

const mermaid = vi.hoisted(() => ({
  initialize: vi.fn(),
  render: vi.fn().mockResolvedValue({ svg: '<svg />' }),
}))
vi.mock('mermaid', () => ({ default: mermaid }))

const palette: MermaidPalette = {
  background: '#101318',
  surface: '#1b2029',
  surfaceAlt: '#232a36',
  border: '#414d5e',
  text: '#e9edf4',
  mutedText: '#a6b0c0',
  accent: '#f5b23f',
  darkMode: true,
  fontFamily: 'Inter, sans-serif',
}

function deferred<T>() {
  let resolve!: (value: T | PromiseLike<T>) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise
    reject = rejectPromise
  })
  return { promise, resolve, reject }
}

describe('renderMermaid', () => {
  beforeEach(() => {
    mermaid.initialize.mockClear()
    mermaid.render.mockClear().mockResolvedValue({ svg: '<svg />' })
  })

  it('renders with locked-down, app-themed configuration', async () => {
    await expect(renderMermaid('flowchart LR\nA --> B', palette)).resolves.toBe('<svg />')

    expect(mermaid.initialize).toHaveBeenCalledWith(
      expect.objectContaining({
        securityLevel: 'strict',
        secure: expect.arrayContaining([
          'securityLevel',
          'maxTextSize',
          'maxEdges',
          'htmlLabels',
          'dompurifyConfig',
          'theme',
          'themeVariables',
          'themeCSS',
          'fontFamily',
        ]),
        startOnLoad: false,
        htmlLabels: false,
        suppressErrorRendering: true,
        maxTextSize: 50 * 1024,
        maxEdges: 500,
        theme: 'base',
        themeVariables: expect.objectContaining({
          darkMode: true,
          primaryColor: '#1b2029',
          primaryTextColor: '#e9edf4',
          primaryBorderColor: '#414d5e',
          lineColor: '#a6b0c0',
        }),
      }),
    )
    expect(mermaid.render).toHaveBeenCalledWith(expect.stringMatching(/^hive-mermaid-/), 'flowchart LR\nA --> B')
  })

  it('serializes concurrent renders so each palette stays with its diagram', async () => {
    const firstRender = deferred<{ svg: string }>()
    const lightPalette = { ...palette, background: '#ffffff', darkMode: false }
    mermaid.render
      .mockImplementationOnce(() => firstRender.promise)
      .mockResolvedValueOnce({ svg: '<svg id="second" />' })

    const first = renderMermaid('flowchart LR\nA --> B', palette)
    const second = renderMermaid('flowchart LR\nC --> D', lightPalette)

    await vi.waitFor(() => expect(mermaid.render).toHaveBeenCalledTimes(1))
    expect(mermaid.initialize).toHaveBeenCalledTimes(1)
    expect(mermaid.initialize).toHaveBeenLastCalledWith(
      expect.objectContaining({ themeVariables: expect.objectContaining({ background: '#101318', darkMode: true }) }),
    )

    firstRender.resolve({ svg: '<svg id="first" />' })

    await expect(first).resolves.toBe('<svg id="first" />')
    await expect(second).resolves.toBe('<svg id="second" />')
    expect(mermaid.initialize).toHaveBeenCalledTimes(2)
    expect(mermaid.initialize).toHaveBeenLastCalledWith(
      expect.objectContaining({ themeVariables: expect.objectContaining({ background: '#ffffff', darkMode: false }) }),
    )
  })

  it('continues the render queue after a diagram fails', async () => {
    const failedRender = deferred<{ svg: string }>()
    mermaid.render
      .mockImplementationOnce(() => failedRender.promise)
      .mockResolvedValueOnce({ svg: '<svg id="recovered" />' })

    const failed = renderMermaid('not a diagram', palette).catch((error: unknown) => error)
    const recovered = renderMermaid('flowchart LR\nA --> B', palette)

    await vi.waitFor(() => expect(mermaid.render).toHaveBeenCalledTimes(1))
    failedRender.reject(new Error('parse failed'))

    await expect(failed).resolves.toEqual(new Error('parse failed'))
    await expect(recovered).resolves.toBe('<svg id="recovered" />')
    expect(mermaid.render).toHaveBeenCalledTimes(2)
  })
})
