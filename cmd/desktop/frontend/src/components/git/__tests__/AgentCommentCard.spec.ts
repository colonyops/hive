import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import AgentCommentCard from '../AgentCommentCard.vue'
import type { AgentComment } from '../../../lib/agentReview'
import type { ThreadResponse } from '../../../stores/useChangesReview'

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
const responses: ThreadResponse[] = [
  { id: 'r1', author: 'you', text: 'on purpose', at: 1 },
  { id: 'r2', author: 'codex', text: 'agreed, the defer closes it', at: 2 },
]
const testid = (id: string) => `[data-testid="agent-comment-c1-${id}"]`

describe('AgentCommentCard', () => {
  it('adds a response and saves it with mod+enter', async () => {
    const wrapper = mount(AgentCommentCard, { props: { comment } })
    await wrapper.get(testid('reply-start')).trigger('click')
    const field = wrapper.get(testid('reply-text'))
    await field.setValue('  closed in the defer  ')
    await field.trigger('keydown', { key: 'Enter', metaKey: true })
    expect(wrapper.emitted('respond')).toEqual([['closed in the defer']])
    expect(wrapper.find(testid('reply-text')).exists()).toBe(false)
  })

  it('cancels a response with Escape and saves nothing', async () => {
    const wrapper = mount(AgentCommentCard, { props: { comment } })
    await wrapper.get(testid('reply-start')).trigger('click')
    await wrapper.get(testid('reply-text')).setValue('draft')
    await wrapper.get(testid('reply-text')).trigger('keydown', { key: 'Escape' })
    expect(wrapper.emitted('respond')).toBeUndefined()
    expect(wrapper.find(testid('reply-start')).exists()).toBe(true)
  })

  it('shows every response in order, and still offers to add one', () => {
    const wrapper = mount(AgentCommentCard, { props: { comment, responses } })
    expect(wrapper.get(testid('response-r1')).text()).toContain('You')
    expect(wrapper.get(testid('response-r2')).text()).toContain('codex')
    const html = wrapper.html()
    expect(html.indexOf('on purpose')).toBeLessThan(html.indexOf('agreed'))
    expect(wrapper.get(testid('reply-start')).text()).toContain('Add to the thread')
  })

  it("edits and deletes the user's own response, and not an agent's", async () => {
    const wrapper = mount(AgentCommentCard, { props: { comment, responses } })
    expect(wrapper.find(testid('response-r2-edit')).exists()).toBe(false)

    await wrapper.get(testid('response-r1-edit')).trigger('click')
    const field = wrapper.get(testid('response-r1-text'))
    expect((field.element as HTMLTextAreaElement).value).toBe('on purpose')
    await field.setValue('on purpose, see #12')
    await wrapper.get(testid('response-r1-save')).trigger('click')

    await wrapper.get(testid('response-r1-delete')).trigger('click')
    expect(wrapper.emitted('edit-response')).toEqual([
      ['r1', 'on purpose, see #12'],
      ['r1', ''],
    ])
  })
})
