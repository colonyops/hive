import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import type { ForgeData } from '../../../lib/fenceData'
import ForgeFence from '../ForgeFence.vue'

const data: ForgeData = {
  snapshot: '2026-11-22T12:30',
  host: 'gitea.example.com',
  items: [
    {
      kind: 'pr',
      ref: 'infra/deploy#212',
      title: 'Pin the runner',
      state: 'merged',
      checks: '6/6',
      diff: '+42 -9',
      comments: 3,
    },
    {
      kind: 'issue',
      ref: 'colonyops/hive#864',
      url: 'https://github.com/colonyops/hive/issues/864',
      title: 'Anchor lost',
      state: 'open',
      labels: ['bug'],
      author: 'sam',
    },
  ],
}

function mountForge(value: ForgeData = data) {
  const follow = vi.fn()
  const wrapper = mount(ForgeFence, {
    props: {
      data: value,
      source: 'items: []',
      ctx: { docPath: 'x.md', paths: new Set<string>(), follow },
      testid: 'f',
    },
  })
  return { wrapper, follow }
}

describe('ForgeFence', () => {
  it('renders the cards and the snapshot', () => {
    const { wrapper } = mountForge()
    expect(wrapper.text()).toContain('snapshot 2026-11-22T12:30')
    expect(wrapper.text()).toContain('Pin the runner')
    expect(wrapper.get('[data-testid="f-state-0"]').text()).toBe('merged')
    expect(wrapper.text()).toContain('checks 6/6')
    expect(wrapper.text()).toContain('+42')
    expect(wrapper.text()).toContain('bug')
    expect(wrapper.text()).toContain('sam')
  })

  it('shows a host only when it differs from the fence host', () => {
    const { wrapper } = mountForge()
    expect(wrapper.find('[data-testid="f-host-0"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="f-host-1"]').text()).toBe('github.com')
  })

  it('builds the default url from the fence host and the kind', async () => {
    const { wrapper, follow } = mountForge()
    await wrapper.get('[data-testid="f-item-0"]').trigger('click')
    expect(follow).toHaveBeenCalledWith('https://gitea.example.com/infra/deploy/pull/212')
  })

  it('uses an explicit url and defaults the host to github.com', async () => {
    const { wrapper, follow } = mountForge()
    await wrapper.get('[data-testid="f-item-1"]').trigger('click')
    expect(follow).toHaveBeenCalledWith('https://github.com/colonyops/hive/issues/864')
    const plain = mountForge({ items: [{ kind: 'issue', ref: 'a/b#5', title: 't', state: 'open' }] })
    await plain.wrapper.get('[data-testid="f-item-0"]').trigger('click')
    expect(plain.follow).toHaveBeenCalledWith('https://github.com/a/b/issues/5')
  })
})
