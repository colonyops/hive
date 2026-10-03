import { describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import ConnectedAccountsField from '../ConnectedAccountsField.vue'

function mountField(accounts: string[], disconnect = vi.fn().mockResolvedValue(undefined)) {
  return mount(ConnectedAccountsField, { props: { accounts, noun: 'stack', testid: 'grafana', disconnect } })
}

describe('ConnectedAccountsField', () => {
  it('says nothing is connected, in the provider noun', () => {
    const wrapper = mountField([])
    expect(wrapper.get('label').text()).toBe('Connected stacks')
    expect(wrapper.get('[data-testid="grafana-connected-empty"]').text()).toBe('No stack connected')
  })

  it('lists each account and disconnects the one clicked', async () => {
    const disconnect = vi.fn().mockResolvedValue(undefined)
    const wrapper = mountField(['one', 'two'], disconnect)

    expect(wrapper.find('[data-testid="grafana-connected-one"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="grafana-connected-two"]').exists()).toBe(true)
    await wrapper.get('[data-testid="grafana-disconnect-two"]').trigger('click')
    expect(disconnect).toHaveBeenCalledWith('two')
  })

  it('marks only the account being disconnected as busy', async () => {
    let resolve!: () => void
    const wrapper = mountField(
      ['one', 'two'],
      vi.fn(() => new Promise<void>((r) => (resolve = r))),
    )

    await wrapper.get('[data-testid="grafana-disconnect-one"]').trigger('click')
    expect(wrapper.get('[data-testid="grafana-disconnect-one"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-testid="grafana-disconnect-two"]').attributes('disabled')).toBeUndefined()

    resolve()
    await flushPromises()
    expect(wrapper.get('[data-testid="grafana-disconnect-one"]').attributes('disabled')).toBeUndefined()
  })
})
