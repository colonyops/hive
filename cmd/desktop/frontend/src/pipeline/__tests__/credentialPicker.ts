import { flushPromises, mount } from '@vue/test-utils'
import { chooseOption } from '@hive/ui/test-utils/select'
import { describe, expect, it, type Mock } from 'vitest'
import type { Component } from 'vue'

export type ConnectAccounts = (...accounts: string[]) => void

export function connectAccounts(list: Mock, integration: Record<string, unknown>): ConnectAccounts {
  return (...accounts) => {
    list.mockResolvedValue([{ ...integration, accounts }])
  }
}

interface CredentialPickerOptions {
  node: string
  Editor: Component
  connect: ConnectAccounts
  provider: string
  accounts: [string, string]
  config: (credential: string) => Record<string, unknown>
  emptyMessage: string
}

export function describeCredentialPicker(options: CredentialPickerOptions) {
  const { node, Editor, connect, provider, config, emptyMessage } = options
  const [account, otherAccount] = options.accounts
  const credential = `${provider}/${account}`
  const testid = `${node}-editor-credential`

  async function mountEditor(props: Record<string, unknown>) {
    const wrapper = mount(Editor, { props })
    await flushPromises()
    return wrapper
  }

  describe(`${node} credential picker`, () => {
    it('offers the connected accounts and emits the choice', async () => {
      connect(account, otherAccount)
      const wrapper = await mountEditor({ config: config(credential) })

      expect(wrapper.get(`[data-testid="${testid}"]`).text()).toContain(credential)
      await chooseOption(wrapper, testid, `${provider}/${otherAccount}`)
      expect(wrapper.emitted('update:config')).toEqual([[config(`${provider}/${otherAccount}`)]])
    })

    // Dropping a disconnected account from the list would silently rewrite the
    // node's config on the next edit of any other field.
    it('keeps an account that is no longer connected selectable, and says so', async () => {
      connect(otherAccount)
      const wrapper = await mountEditor({ config: config(credential) })

      expect(wrapper.get(`[data-testid="${testid}"]`).text()).toContain('not connected')
    })

    it('falls back to a text input when nothing is connected', async () => {
      connect()
      const wrapper = await mountEditor({ config: config('') })

      expect(wrapper.get(`[data-testid="${testid}"]`).element.tagName).toBe('INPUT')
      expect(wrapper.text()).toContain(emptyMessage)
    })
  })
}
