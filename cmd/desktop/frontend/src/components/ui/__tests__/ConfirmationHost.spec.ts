import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import ConfirmationHost from '../ConfirmationHost.vue'
import { useConfirmation } from '../../../composables/useConfirmation'

function el<T extends HTMLElement>(testid: string): T | null {
  return document.querySelector<T>(`[data-testid="${testid}"]`)
}

function mountHost(testid?: string) {
  const confirmation = useConfirmation()
  const wrapper = mount(ConfirmationHost, { props: { confirmation, testid }, attachTo: document.body })
  return { confirmation, wrapper }
}

beforeEach(() => {
  document.body.innerHTML = ''
})

describe('ConfirmationHost', () => {
  it('renders nothing until a confirmation is requested', async () => {
    const { confirmation } = mountHost()
    expect(el('confirmation-dialog')).toBeNull()

    confirmation.request({ title: 'Delete it?', description: 'Gone for good.', onConfirm: () => {} })
    await flushPromises()
    expect(el('confirmation-dialog')!.textContent).toContain('Gone for good.')
  })

  it('runs onConfirm and closes on success', async () => {
    const onConfirm = vi.fn()
    const { confirmation } = mountHost()
    confirmation.request({ title: 'Delete it?', description: 'x', onConfirm })
    await flushPromises()

    el<HTMLButtonElement>('confirmation-dialog-confirm')!.click()
    await flushPromises()
    expect(onConfirm).toHaveBeenCalledOnce()
    expect(el('confirmation-dialog')).toBeNull()
  })

  it('stays open with the error when onConfirm throws', async () => {
    const { confirmation } = mountHost()
    confirmation.request({
      title: 'Delete it?',
      description: 'x',
      onConfirm: () => Promise.reject(new Error('disk full')),
    })
    await flushPromises()

    el<HTMLButtonElement>('confirmation-dialog-confirm')!.click()
    await flushPromises()
    expect(el('confirmation-dialog-error')!.textContent).toContain('disk full')

    el<HTMLButtonElement>('confirmation-dialog-cancel')!.click()
    await flushPromises()
    expect(el('confirmation-dialog')).toBeNull()
  })

  it('ignores a new request while one is running', async () => {
    let finish = (): void => {}
    const { confirmation } = mountHost()
    confirmation.request({
      title: 'First',
      description: 'first',
      onConfirm: () => new Promise<void>((resolve) => (finish = resolve)),
    })
    void confirmation.confirm()
    confirmation.request({ title: 'Second', description: 'second', onConfirm: () => {} })
    await flushPromises()
    expect(el('confirmation-dialog')!.textContent).toContain('first')
    finish()
  })

  it('takes test ids from the request, then the host, then the default', async () => {
    const { confirmation } = mountHost('host-confirm')
    confirmation.request({ title: 'A', description: 'a', onConfirm: () => {} })
    await flushPromises()
    expect(el('host-confirm')).not.toBeNull()

    confirmation.request({
      title: 'B',
      description: 'b',
      testid: 'own-confirm',
      confirmTestid: 'own-go',
      onConfirm: () => {},
    })
    await flushPromises()
    expect(el('own-confirm')).not.toBeNull()
    expect(el('own-go')).not.toBeNull()
  })
})
