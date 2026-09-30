import { describe, expect, it } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import ToastStack from '../ToastStack.vue'
import type { ToastInstance } from '../../types/toast'

let nextId = 1
function toast(overrides: Partial<ToastInstance> = {}): ToastInstance {
  return {
    id: nextId++,
    message: 'Feed refreshed',
    severity: 'info',
    actions: [],
    duration: 4000,
    ...overrides,
  }
}

function toastEls(wrapper: VueWrapper) {
  return wrapper.findAll('[data-testid="toast"]')
}

describe('ToastStack', () => {
  it('renders nothing when there are no toasts', () => {
    const wrapper = mount(ToastStack, { props: { toasts: [] } })

    expect(wrapper.find('[data-testid="toast-stack"]').exists()).toBe(false)

    wrapper.unmount()
  })

  it('stacks multiple toasts bottom-to-top in insertion order', () => {
    const toasts = [toast({ message: 'First' }), toast({ message: 'Second' }), toast({ message: 'Third' })]
    const wrapper = mount(ToastStack, { props: { toasts } })

    const titles = toastEls(wrapper).map((el) => el.find('[data-testid="toast-title"]').text())
    expect(titles).toEqual(['First', 'Second', 'Third'])

    wrapper.unmount()
  })

  it('renders severity-specific styling, the AUTO badge, and inline actions', () => {
    const undo = { label: 'Undo', onClick: () => {} }
    const view = { label: 'View in activity', onClick: () => {} }
    const toasts = [
      toast({ severity: 'info', message: 'Feed refreshed' }),
      toast({ severity: 'auto-action', message: 'Automatic action taken', actions: [undo, view] }),
      toast({ severity: 'warning', message: 'Rate limit is close' }),
      toast({ severity: 'error', message: "Couldn't reach GitHub" }),
      toast({ severity: 'success', message: 'Session created' }),
    ]
    const wrapper = mount(ToastStack, { props: { toasts } })

    const els = toastEls(wrapper)
    expect(els.map((el) => el.attributes('data-toast-severity'))).toEqual(['auto-action', 'warning', 'error', 'success'])

    const autoToast = els[0]
    expect(autoToast.find('[data-testid="toast-auto-badge"]').text()).toBe('AUTO')
    const actionLabels = autoToast.findAll('[data-testid="toast-action"]').map((el) => el.text())
    expect(actionLabels).toEqual(['Undo', 'View in activity'])

    // Other severities show no AUTO badge.
    expect(els[1].find('[data-testid="toast-auto-badge"]').exists()).toBe(false)

    wrapper.unmount()
  })

  it('shows auto-dismiss progress for error and success toasts', () => {
    const toasts = [toast({ severity: 'error' }), toast({ severity: 'success' })]
    const wrapper = mount(ToastStack, { props: { toasts } })

    const els = toastEls(wrapper)
    expect(els[0].find('[data-testid="toast-progress"]').exists()).toBe(true)
    expect(els[1].find('[data-testid="toast-progress"]').exists()).toBe(true)

    wrapper.unmount()
  })

  it('emits dismiss with the toast id when its close button is clicked', async () => {
    const t = toast({ message: 'Dismiss me' })
    const wrapper = mount(ToastStack, { props: { toasts: [t] } })

    await toastEls(wrapper)[0].find('[data-testid="toast-dismiss"]').trigger('click')

    expect(wrapper.emitted('dismiss')).toEqual([[t.id]])

    wrapper.unmount()
  })

  it('runs a toast action and dismisses the toast', async () => {
    let ran = false
    const t = toast({ actions: [{ label: 'Retry now', onClick: () => { ran = true } }] })
    const wrapper = mount(ToastStack, { props: { toasts: [t] } })

    await toastEls(wrapper)[0].find('[data-testid="toast-action"]').trigger('click')

    expect(ran).toBe(true)
    expect(wrapper.emitted('dismiss')).toEqual([[t.id]])

    wrapper.unmount()
  })

  it('collapses overflow beyond 4 visible toasts into an "N more · Clear all" footer', async () => {
    const toasts = Array.from({ length: 6 }, (_, i) => toast({ message: `Toast ${i + 1}` }))
    const wrapper = mount(ToastStack, { props: { toasts } })

    // Newest 4 stay visible; the oldest 2 roll into the overflow count.
    const titles = toastEls(wrapper).map((el) => el.find('[data-testid="toast-title"]').text())
    expect(titles).toEqual(['Toast 3', 'Toast 4', 'Toast 5', 'Toast 6'])
    expect(wrapper.find('[data-testid="toast-overflow"]').text()).toContain('2 more')

    await wrapper.find('[data-testid="toast-clear-all"]').trigger('click')
    expect(wrapper.emitted('clear-all')).toHaveLength(1)

    wrapper.unmount()
  })

  it('omits the overflow footer at exactly 4 toasts', () => {
    const toasts = Array.from({ length: 4 }, (_, i) => toast({ message: `Toast ${i + 1}` }))
    const wrapper = mount(ToastStack, { props: { toasts } })

    expect(toastEls(wrapper)).toHaveLength(4)
    expect(wrapper.find('[data-testid="toast-overflow"]').exists()).toBe(false)

    wrapper.unmount()
  })
})
