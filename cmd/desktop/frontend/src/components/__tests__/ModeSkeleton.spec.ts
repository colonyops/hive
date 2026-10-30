import { mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it } from 'vitest'
import { TERMINAL_SIDEBAR } from '../../lib/modeSidebars'
import ModeSkeleton from '../ModeSkeleton.vue'

function aside() {
  return mount(ModeSkeleton, { props: { sidebar: TERMINAL_SIDEBAR } }).find('aside')
}

describe('ModeSkeleton', () => {
  beforeEach(() => localStorage.clear())

  it('reserves the width the sidebar was left at', () => {
    localStorage.setItem(TERMINAL_SIDEBAR.panel.storageKey, '310')
    expect(aside().attributes('style')).toContain('width: 310px')
  })

  it('falls back to the default width', () => {
    expect(aside().attributes('style')).toContain(`width: ${TERMINAL_SIDEBAR.panel.defaultSize}px`)
  })

  it('reserves nothing for a collapsed sidebar', () => {
    localStorage.setItem(TERMINAL_SIDEBAR.collapsedKey, 'true')
    expect(aside().exists()).toBe(false)
  })
})
