import { describe, expect, it } from 'vitest'
import { canvasLinkTarget } from '../agentCanvas'

const names = new Set(['plan', 'perf-report'])

describe('canvasLinkTarget', () => {
  it.each(['https://example.com/pr/1', 'http://example.com', 'mailto:dev@example.com'])(
    'opens %s outside the app',
    (href) => {
      expect(canvasLinkTarget(href, names)).toEqual({ kind: 'url', url: href })
    },
  )

  it.each(['perf-report', './perf-report'])('resolves %s to the canvas of that name', (href) => {
    expect(canvasLinkTarget(href, names)).toEqual({ kind: 'canvas', name: 'perf-report' })
  })

  it.each(['missing', 'javascript:alert(1)', '/etc/passwd', '../plan', '#top', ''])('leads nowhere for %j', (href) => {
    expect(canvasLinkTarget(href, names)).toBeNull()
  })

  // A canvas named like a scheme-less host must not shadow a real address.
  it('prefers the address when a link is both', () => {
    expect(canvasLinkTarget('https://plan', names)).toEqual({ kind: 'url', url: 'https://plan' })
  })
})
