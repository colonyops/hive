import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import AgentCommentCard from '../AgentCommentCard.vue'
import type { AgentComment } from '../../../lib/agentReview'

const comment: AgentComment = {
  id: 'c1',
  kind: '',
  path: 'a.go',
  startSide: 'additions',
  startLine: 1,
  endSide: 'additions',
  endLine: 1,
  quote: '+x',
  severity: 'issue',
  text: 'this leaks',
  reviewId: 'r1',
  author: 'claude',
  agent: 'claude',
}
const testid = (id: string) => `[data-testid="agent-comment-c1-${id}"]`

describe('AgentCommentCard', () => {
  it('writes a reply and saves it with mod+enter', async () => {
    const wrapper = mount(AgentCommentCard, { props: { comment } })
    await wrapper.get(testid('reply-start')).trigger('click')
    const field = wrapper.get(testid('reply-text'))
    await field.setValue('  closed in the defer  ')
    await field.trigger('keydown', { key: 'Enter', metaKey: true })
    expect(wrapper.emitted('reply')).toEqual([['closed in the defer']])
    expect(wrapper.find(testid('reply-text')).exists()).toBe(false)
  })

  it('cancels a reply with Escape and saves nothing', async () => {
    const wrapper = mount(AgentCommentCard, { props: { comment } })
    await wrapper.get(testid('reply-start')).trigger('click')
    await wrapper.get(testid('reply-text')).setValue('draft')
    await wrapper.get(testid('reply-text')).trigger('keydown', { key: 'Escape' })
    expect(wrapper.emitted('reply')).toBeUndefined()
    expect(wrapper.find(testid('reply-start')).exists()).toBe(true)
  })

  it('shows a saved reply, edits it from its text, and deletes it', async () => {
    const wrapper = mount(AgentCommentCard, { props: { comment, reply: 'on purpose' } })
    expect(wrapper.get(testid('reply')).text()).toContain('on purpose')
    expect(wrapper.find(testid('reply-start')).exists()).toBe(false)

    await wrapper.get(testid('reply-edit')).trigger('click')
    expect((wrapper.get(testid('reply-text')).element as HTMLTextAreaElement).value).toBe('on purpose')
    await wrapper.get(testid('reply-text')).trigger('keydown', { key: 'Escape' })

    await wrapper.get(testid('reply-delete')).trigger('click')
    expect(wrapper.emitted('reply')).toEqual([['']])
  })
})
