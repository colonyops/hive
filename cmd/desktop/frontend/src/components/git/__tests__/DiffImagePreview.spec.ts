import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import DiffImagePreview from '../DiffImagePreview.vue'
import type { SessionFileImage } from '../../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'

// jsdom decodes nothing, so each test says what a data URL decodes to.
const decodes = new Map<string, { width: number; height: number } | null>()

function version(name: string, size = 100, mime = 'image/png') {
  return { mime, data: btoa(name), size }
}
function src(name: string, mime = 'image/png') {
  return `data:${mime};base64,${btoa(name)}`
}
function file(extra: Record<string, unknown> = {}) {
  return {
    path: 'a.png',
    oldPath: '',
    status: 'modified',
    additions: 0,
    deletions: 0,
    binary: true,
    stamp: '1',
    ...extra,
  }
}
async function mountPreview(image: unknown, extra: Record<string, unknown> = {}) {
  const load = vi.fn().mockResolvedValue(image)
  const wrapper = mount(DiffImagePreview, { props: { file: file(extra), version: '1', load } })
  await flushPromises()
  await flushPromises()
  return { wrapper, load }
}
const testid = (id: string) => `[data-testid="changes-image-a.png-${id}"]`

describe('DiffImagePreview', () => {
  beforeEach(() => {
    localStorage.clear()
    decodes.clear()
    vi.spyOn(HTMLImageElement.prototype, 'decode').mockImplementation(function (this: HTMLImageElement) {
      const dims = decodes.get(this.src)
      if (!dims) return Promise.reject(new Error('cannot decode'))
      Object.defineProperty(this, 'naturalWidth', { value: dims.width })
      Object.defineProperty(this, 'naturalHeight', { value: dims.height })
      return Promise.resolve()
    })
  })
  afterEach(() => vi.restoreAllMocks())

  it('compares two versions side by side and names a resize and the size change', async () => {
    decodes.set(src('old'), { width: 64, height: 64 })
    decodes.set(src('new'), { width: 128, height: 128 })
    const { wrapper } = await mountPreview({
      path: 'a.png',
      state: 'ready',
      before: version('old', 1000),
      after: version('new', 1500),
      error: '',
    })

    expect(wrapper.get(testid('before')).attributes('src')).toBe(src('old'))
    expect(wrapper.get(testid('after')).attributes('src')).toBe(src('new'))
    expect(wrapper.get(testid('meta')).text()).toContain('64 × 64 → 128 × 128')
    expect(wrapper.get(testid('meta')).text()).toContain('+500 B')
    expect(wrapper.find(testid('resized')).exists()).toBe(true)
  })

  it('remembers the compare mode for the next preview', async () => {
    decodes.set(src('old'), { width: 10, height: 10 })
    decodes.set(src('new'), { width: 10, height: 10 })
    const image = { path: 'a.png', state: 'ready', before: version('old'), after: version('new'), error: '' }
    const { wrapper } = await mountPreview(image)

    await wrapper.get(`${testid('compare')} button:nth-child(2)`).trigger('click')
    expect(wrapper.find(testid('swipe')).exists()).toBe(true)
    wrapper.unmount()

    const next = await mountPreview(image)
    expect(next.wrapper.find(testid('swipe')).exists()).toBe(true)
  })

  it('labels an added file and draws its one version', async () => {
    decodes.set(src('new'), { width: 20, height: 10 })
    const { wrapper } = await mountPreview(
      { path: 'a.png', state: 'ready', before: null, after: version('new'), error: '' },
      { status: 'added' },
    )

    expect(wrapper.text()).toContain('Added')
    expect(wrapper.get(testid('meta')).text()).toContain('20 × 10')
    expect(wrapper.find(testid('compare')).exists()).toBe(false)
  })

  it('says which version it cannot draw instead of calling the file deleted', async () => {
    decodes.set(src('old'), { width: 16, height: 16 })
    const { wrapper } = await mountPreview({
      path: 'a.png',
      state: 'ready',
      before: version('old'),
      after: { mime: '', data: btoa('junk'), size: 4 },
      error: '',
    })

    expect(wrapper.get(testid('note')).text()).toBe('The after version is not an image this view can draw')
    expect(wrapper.text()).not.toContain('Deleted')
  })

  it('reports a file too large to read', async () => {
    const { wrapper } = await mountPreview({ path: 'a.png', state: 'too_large', before: null, after: null, error: '' })

    expect(wrapper.get(testid('note')).text()).toBe('Too large to preview')
  })

  it('reads the image again when the file changes', async () => {
    decodes.set(src('old'), { width: 10, height: 10 })
    const { wrapper, load } = await mountPreview({
      path: 'a.png',
      state: 'ready',
      before: null,
      after: version('old'),
      error: '',
    })
    expect(load).toHaveBeenCalledTimes(1)

    await wrapper.setProps({ version: '2' })
    await flushPromises()
    expect(load).toHaveBeenCalledTimes(2)
  })

  it('shows the newest version when an older read finishes last', async () => {
    decodes.set(src('base'), { width: 10, height: 10 })
    decodes.set(src('old'), { width: 10, height: 10 })
    decodes.set(src('new'), { width: 10, height: 10 })
    const reads: ((image: SessionFileImage) => void)[] = []
    const load = vi.fn(() => new Promise<SessionFileImage>((resolve) => reads.push(resolve)))
    const wrapper = mount(DiffImagePreview, { props: { file: file(), version: '1', load } })
    await flushPromises()
    await wrapper.setProps({ version: '2' })
    await flushPromises()
    expect(reads).toHaveLength(2)

    const ready = (name: string): SessionFileImage => ({
      path: 'a.png',
      state: 'ready',
      before: version('base'),
      after: version(name),
      error: '',
    })
    reads[1](ready('new'))
    await flushPromises()
    reads[0](ready('old'))
    await flushPromises()

    expect(wrapper.get(testid('after')).attributes('src')).toBe(src('new'))
  })
})
