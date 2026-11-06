import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import DriveReader from '../DriveReader.vue'
import type { DocumentWire } from '../../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'
import { doc, documentFor } from '../../../test-utils/drive'

const settingsBindings = vi.hoisted(() => ({
  AppearanceSettings: vi.fn().mockResolvedValue({ canvasLineSpacing: '' }),
  SetCanvasLineSpacing: vi.fn(),
  ReviewLabels: vi.fn().mockResolvedValue(null),
}))
vi.mock(
  '../../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/settingsservice',
  () => settingsBindings,
)

const paths = new Set(['plans/x.md', 'reports/report.md', 'research/r.md'])

async function mountReader(document: DocumentWire | null, extra: Record<string, unknown> = {}) {
  const wrapper = mount(DriveReader, {
    props: { document, paths, loading: false, error: null, testid: 'r', ...extra },
  })
  await flushPromises()
  return wrapper
}

describe('DriveReader', () => {
  it('resolves a link against the space: web links open outside, document links open in place', async () => {
    const wrapper = await mountReader(
      documentFor('acme/site', doc('plans/x.md'), {
        markdown:
          '[web](https://example.com) [report](../reports/report.md) [research](.hive/research/r.md) [typo](../reports/reprot.md)',
      }),
    )
    for (const anchor of wrapper.findAll('[data-testid="r-markdown"] a')) await anchor.trigger('click')

    expect(wrapper.emitted('open-url')).toEqual([['https://example.com']])
    expect(wrapper.emitted('open')).toEqual([['reports/report.md'], ['research/r.md']])
  })

  it('draws an image file', async () => {
    const image = 'data:image/png;base64,cG5n'
    const wrapper = await mountReader(documentFor('acme/site', doc('shots/login.png'), { image }))

    expect(wrapper.get('[data-testid="r-image"]').attributes('src')).toBe(image)
    expect(wrapper.get('[data-testid="r-image"]').attributes('alt')).toBe('login.png')
    expect(wrapper.find('[data-testid="r-file"]').exists()).toBe(false)
  })

  it('says a plain file is not shown', async () => {
    const wrapper = await mountReader(documentFor('acme/site', doc('notes.txt')))
    expect(wrapper.find('[data-testid="r-file"]').exists()).toBe(true)
  })

  it('shows the empty state with no document, and an error over everything', async () => {
    const empty = await mountReader(null)
    expect(empty.find('[data-testid="r-empty"]').exists()).toBe(true)

    const failed = await mountReader(null, { error: 'Could not read the document.' })
    expect(failed.get('[data-testid="r-error"]').text()).toContain('Could not read the document.')
    expect(failed.find('[data-testid="r-empty"]').exists()).toBe(false)
  })

  it('passes the reading settings to the document', async () => {
    settingsBindings.AppearanceSettings.mockResolvedValue({ documentScale: 133, canvasLineSpacing: 'relaxed' })
    const wrapper = await mountReader(documentFor('acme/site', doc('plans/x.md'), { markdown: '# X' }))
    const style = (wrapper.get('[data-testid="r-document"]').element as HTMLElement).style
    expect(style.getPropertyValue('--hv-font-size')).toBe('17.96px')
    expect(style.getPropertyValue('--hv-line-height')).toBe('1.85')
  })
})
