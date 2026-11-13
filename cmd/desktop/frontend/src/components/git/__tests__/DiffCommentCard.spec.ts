import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import DiffCommentCard from '../DiffCommentCard.vue'
import type { ChangesComment } from '../../../stores/useChangesReview'

vi.mock('../../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/settingsservice', () => ({
  ReviewLabels: vi.fn().mockResolvedValue([]),
  SetReviewLabels: vi.fn(),
}))

const comment: ChangesComment = {
  id: 'c1',
  path: 'a.go',
  startSide: 'additions',
  startLine: 1,
  endSide: 'additions',
  endLine: 1,
  quote: '+x',
  base: 'branch',
  version: 'v1',
  text: 'rename this',
}
const testid = (id: string) => `[data-testid="changes-comment-c1-${id}"]`

describe('DiffCommentCard', () => {
  it('puts the keyboard in the field when editing starts', async () => {
    const wrapper = mount(DiffCommentCard, { props: { comment }, attachTo: document.body })
    await wrapper.get(testid('edit')).trigger('click')
    await flushPromises()
    expect(document.activeElement).toBe(wrapper.get(testid('edit-text')).element)
    wrapper.unmount()
  })
})
