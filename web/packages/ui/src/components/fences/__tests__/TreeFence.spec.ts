import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import type { TreeData } from '../../../lib/fenceData'
import TreeFence from '../TreeFence.vue'

const data: TreeData = {
  root: 'src',
  files: [
    { path: 'lib/a.ts', status: 'added', note: 'new parser' },
    { path: 'lib/b.ts', status: 'modified' },
    { path: 'lib/c.ts', status: 'deleted' },
    { path: 'lib/d.ts', status: 'renamed', from: 'lib/old.ts' },
    { path: 'plain.ts' },
  ],
}

function mountTree(paths: string[] = [], follow = vi.fn()) {
  const wrapper = mount(TreeFence, {
    props: { data, source: 'files: []', ctx: { docPath: 'x.md', paths: new Set(paths), follow }, testid: 't' },
  })
  return { wrapper, follow }
}

describe('TreeFence', () => {
  it('renders folders, status letters and notes', () => {
    const { wrapper } = mountTree()
    const text = wrapper.text()
    expect(text).toContain('src/')
    expect(text).toContain('lib/')
    expect(text).toContain('new parser')
    expect(text).toContain('from lib/old.ts')
    const letters = wrapper.findAll('.tree-fence-status').map((el) => el.text())
    expect(letters).toEqual(['A', 'M', 'D', 'R'])
    expect(wrapper.text()).toContain('5 files')
  })

  it('counts added, modified (renamed included) and deleted in the header', () => {
    const { wrapper } = mountTree()
    expect(wrapper.get('[data-testid="t-added"]').text()).toBe('+1')
    expect(wrapper.get('[data-testid="t-modified"]').text()).toBe('~2')
    expect(wrapper.get('[data-testid="t-deleted"]').text()).toContain('1')
  })

  it('links a path that resolves to a document and leaves others plain', async () => {
    const { wrapper, follow } = mountTree(['src/lib/a.ts'])
    expect(wrapper.find('[data-testid="t-link-src/lib/b.ts"]').exists()).toBe(false)
    await wrapper.get('[data-testid="t-link-src/lib/a.ts"]').trigger('click')
    expect(follow).toHaveBeenCalledWith('src/lib/a.ts')
  })
})
