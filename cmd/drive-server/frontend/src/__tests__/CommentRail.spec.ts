import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'
import CommentRail from '../components/collab/CommentRail.vue'
import type { Thread, Viewer } from '../types'

const at = '2026-11-20T12:00:00Z'

function thread(id: string, overrides: Partial<Thread> = {}): Thread {
  return {
    id,
    author: { id: 'u1', name: 'Bea' },
    outdated: false,
    created_at: at,
    comments: [
      { id: `${id}-c1`, thread_id: id, author: { id: 'u1', name: 'Bea' }, body: `body of ${id}`, created_at: at },
    ],
    ...overrides,
  }
}

const collab = {
  createThread: vi.fn().mockResolvedValue(null),
  reply: vi.fn().mockResolvedValue(null),
  setResolved: vi.fn().mockResolvedValue(null),
  editComment: vi.fn().mockResolvedValue(null),
  deleteComment: vi.fn().mockResolvedValue(null),
}

async function mountRail(threads: Thread[], viewer: Viewer | null, signIn: string | null = '/sign-in') {
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/:p(.*)*', component: {} }] })
  await router.push('/')
  return mount(CommentRail, {
    props: { threads, viewer, collab, activeId: null, hoveredId: null, alignTo: null, signIn, error: null },
    global: { plugins: [router] },
  })
}

describe('CommentRail', () => {
  it('groups general, open, outdated, and resolved threads', async () => {
    const wrapper = await mountRail(
      [
        thread('g'),
        thread('o', { anchor: { start_line: 1, end_line: 1, quote: 'q' } }),
        thread('x', { anchor: { start_line: 2, end_line: 2, quote: 'gone' }, outdated: true }),
        thread('r', { anchor: { start_line: 3, end_line: 3, quote: 'r' }, resolved_at: at }),
      ],
      { is_owner: true, can_comment: true, user_id: 'u0' },
    )
    expect(wrapper.get('[data-testid="group-general"]').text()).toContain('body of g')
    expect(wrapper.get('[data-testid="group-open"]').text()).toContain('body of o')
    expect(wrapper.get('[data-testid="group-outdated"]').text()).toContain('gone')
    expect(wrapper.get('[data-testid="group-resolved"]').text()).toContain('body of r')
  })

  it('offers sign-in instead of a composer to an anonymous reader', async () => {
    const wrapper = await mountRail([thread('g')], { is_owner: false, can_comment: false })
    expect(wrapper.find('[data-testid="rail-composer"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="rail-sign-in"]').text()).toBe('Sign in to comment')
    expect(wrapper.find('[data-testid="thread-reply"]').exists()).toBe(false)
  })

  it('marks an agent reply and a deleted first comment', async () => {
    const t = thread('a', {
      comments: [
        { id: 'c1', thread_id: 'a', author: { name: '' }, body: '', deleted: true, created_at: at },
        {
          id: 'c2',
          thread_id: 'a',
          author: { id: 'u0', name: 'Owner', agent: 'claude' },
          body: 'done',
          created_at: at,
        },
      ],
    })
    const wrapper = await mountRail([t], { is_owner: true, can_comment: true, user_id: 'u0' })
    expect(wrapper.get('[data-testid="comment-deleted"]').text()).toBe('Comment deleted')
    expect(wrapper.get('[data-testid="agent-badge"]').text()).toContain('claude')
  })

  it('posts a general comment from the composer', async () => {
    const wrapper = await mountRail([], { is_owner: true, can_comment: true, user_id: 'u0' })
    await wrapper.get('[data-testid="new-thread-input"]').setValue('hello')
    await wrapper.get('[data-testid="new-thread-submit"]').trigger('click')
    expect(collab.createThread).toHaveBeenCalledWith(null, 'hello')
  })
})
