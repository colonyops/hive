import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import JobsPopover from '../JobsPopover.vue'

const mocks = vi.hoisted(() => ({ Cancel: vi.fn() }))
vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/jobservice', () => ({
  Cancel: mocks.Cancel,
}))

describe('JobsPopover', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.Cancel.mockResolvedValue(undefined)
  })

  it('cancels a running command from its job row', async () => {
    const wrapper = mount(JobsPopover, {
      props: {
        jobs: [
          {
            id: 4,
            createdAt: 1,
            updatedAt: 2,
            status: 'running',
            label: 'Review PR',
            step: 'Running…',
            actionId: 'review',
            target: 'item-1',
            error: '',
            commandId: 42,
          },
        ],
      },
    })

    await wrapper.get('[data-testid="job-cancel-4"]').trigger('click')
    await flushPromises()
    expect(mocks.Cancel).toHaveBeenCalledWith(42)
  })
})
