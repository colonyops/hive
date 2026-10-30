import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import DrawerSheet from '../DrawerSheet.vue'

describe('DrawerSheet', () => {
  it('draws no header when bare, and still names the sheet with its title', () => {
    const wrapper = mount(DrawerSheet, {
      props: { title: 'Task details', bare: true, testid: 'sheet' },
      slots: { default: '<div data-testid="sheet-content" />' },
    })
    const sheet = document.querySelector('[data-testid="sheet"]')!
    expect(sheet.getAttribute('aria-label')).toBe('Task details')
    expect(sheet.querySelector('header')).toBeNull()
    expect(sheet.querySelector('[data-testid="sheet-content"]')).not.toBeNull()
    wrapper.unmount()
  })
})
