import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it, vi, beforeEach } from 'vitest'
import type { CodeViewFileItem } from '@pierre/diffs'
import DriveCodeView from '../DriveCodeView.vue'

const mocks = vi.hoisted(() => ({
  setItems: vi.fn<(items: CodeViewFileItem<undefined>[]) => void>(),
  scrollTo: vi.fn(),
  setOptions: vi.fn(),
  cleanUp: vi.fn(),
  setRenderOptions: vi.fn().mockResolvedValue(undefined),
  options: null as Record<string, unknown> | null,
}))

vi.mock('../../../lib/diffHighlight', () => ({
  diffWorkerPool: () => ({ setRenderOptions: mocks.setRenderOptions }),
  diffThemeName: (theme: string) => `code-${theme}`,
}))
vi.mock('@pierre/diffs', () => ({
  CodeView: class {
    constructor(options: Record<string, unknown>) {
      mocks.options = options
    }
    setup() {}
    setItems = mocks.setItems
    scrollTo = mocks.scrollTo
    setOptions = mocks.setOptions
    cleanUp = mocks.cleanUp
  },
}))

const base = { id: 'acme/site:src/main.go', name: 'main.go', text: 'package main\n', version: 'v1', testid: 'code' }

function lastItem(): CodeViewFileItem<undefined> {
  return mocks.setItems.mock.calls.at(-1)![0][0]
}

describe('DriveCodeView', () => {
  beforeEach(() => vi.clearAllMocks())

  it('draws the file as one item, named for language inference and keyed for the highlight cache', async () => {
    mount(DriveCodeView, { props: base })
    await flushPromises()

    expect(lastItem()).toEqual({
      id: 'acme/site:src/main.go',
      type: 'file',
      version: 0,
      file: { name: 'main.go', contents: 'package main\n', cacheKey: 'drive:acme/site:src/main.go:v1' },
    })
    expect(mocks.options).toMatchObject({ disableFileHeader: true, overflow: 'scroll' })
    expect(mocks.setRenderOptions).toHaveBeenCalled()
  })

  it('keeps the scroll position on a reload of the same file and resets it for another', async () => {
    const wrapper = mount(DriveCodeView, { props: base })
    await flushPromises()

    await wrapper.setProps({ text: 'package main\n\nfunc main() {}\n', version: 'v2' })
    expect(lastItem().file.cacheKey).toBe('drive:acme/site:src/main.go:v2')
    expect(lastItem().version).toBe(1)
    expect(mocks.scrollTo).not.toHaveBeenCalled()

    await wrapper.setProps({ id: 'acme/site:notes.txt', name: 'notes.txt', text: 'hi\n' })
    expect(lastItem().id).toBe('acme/site:notes.txt')
    expect(mocks.scrollTo).toHaveBeenCalledWith({ type: 'position', position: 0 })
  })

  it('releases the viewer on unmount', async () => {
    const wrapper = mount(DriveCodeView, { props: base })
    await flushPromises()
    wrapper.unmount()
    expect(mocks.cleanUp).toHaveBeenCalled()
  })
})
