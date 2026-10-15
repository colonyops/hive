import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'
import SpaceDocuments from '../pages/SpaceDocuments.vue'
import { spaceFromKey } from '../lib/space'
import { doc } from './fixtures'

const documents = vi.hoisted(() => vi.fn())
vi.mock('../api', () => ({ api: { documents } }))

describe('SpaceDocuments', () => {
  beforeEach(() => documents.mockReset())

  it('lists documents sorted by path with kind, size, and shared marker', async () => {
    documents.mockResolvedValue([
      doc({ path: 'b/plan.html', kind: 'html', size: 10 }),
      doc({ path: 'a.md', shared: { url: 'http://x/s/abc' } }),
    ])
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/', name: 'spaces', component: {} },
        { path: '/:p(.*)*', component: {} },
      ],
    })
    const wrapper = mount(SpaceDocuments, {
      props: { space: spaceFromKey('acme/notes') },
      global: { plugins: [router] },
    })
    await flushPromises()

    expect(documents).toHaveBeenCalledWith('/api/v1/repos/acme/notes')
    const rows = wrapper.findAll('[data-testid="document-row"]')
    expect(rows.map((r) => r.find('span').text())).toEqual(['a.md', 'b/plan.html'])
    expect(rows[0].text()).toContain('Shared')
    expect(rows[0].text()).toContain('markdown')
    expect(rows[0].text()).toContain('2.0 KB')
    expect(rows[1].text()).not.toContain('Shared')
    expect(rows[1].find('a').attributes('href')).toBe('/repos/acme/notes/b/plan.html')
  })
})
