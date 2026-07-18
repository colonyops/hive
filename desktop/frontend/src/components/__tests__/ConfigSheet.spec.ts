import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import ConfigSheet from '../ConfigSheet.vue'
import type { ConfigInfo } from '../../types/feed'

function makeConfig(overrides: Partial<ConfigInfo> = {}): ConfigInfo {
  return {
    path: '/home/u/.config/hive/desktop/profiles.yaml',
    exists: true,
    yaml: '# feeds\nprofiles:\n  - id: work\n    name: Work\n',
    valid: true,
    error: '',
    ...overrides,
  }
}

function mountSheet(config: ConfigInfo | null) {
  return mount(ConfigSheet, { props: { config } })
}

describe('ConfigSheet', () => {
  it('renders the config path and highlighted YAML', () => {
    const wrapper = mountSheet(makeConfig())

    expect(document.querySelector('[data-testid="config-sheet-path"]')?.textContent).toContain('profiles.yaml')
    const yaml = document.querySelector('[data-testid="config-sheet-yaml"]')
    expect(yaml?.textContent).toContain('- id: work')
    expect(yaml?.querySelector('.text-code-comment')?.textContent).toBe('# feeds')
    expect(yaml?.querySelector('.text-code-key')?.textContent).toBe('profiles:')
    expect(document.querySelector('[data-testid="config-sheet-valid"]')).not.toBeNull()
    expect(document.querySelector('[data-testid="config-sheet-error"]')).toBeNull()

    wrapper.unmount()
  })

  it('shows the error callout when the config is invalid', () => {
    const wrapper = mountSheet(makeConfig({ valid: false, error: 'profiles.yaml: line 3: oops' }))

    expect(document.querySelector('[data-testid="config-sheet-error"]')?.textContent).toContain('line 3: oops')
    expect(document.querySelector('[data-testid="config-sheet-valid"]')).toBeNull()

    wrapper.unmount()
  })

  it('labels a missing file as the starting template', () => {
    const wrapper = mountSheet(makeConfig({ exists: false }))

    expect(document.body.textContent).toContain('Not created yet')
    expect(document.body.textContent).toContain('Starting template')

    wrapper.unmount()
  })

  it('emits copy-prompt, copy-path, and close', async () => {
    const wrapper = mountSheet(makeConfig())

    document.querySelector<HTMLButtonElement>('[data-testid="config-sheet-copy-prompt"]')?.click()
    document.querySelector<HTMLButtonElement>('[data-testid="config-sheet-copy-path"]')?.click()
    document.querySelector<HTMLButtonElement>('[data-testid="config-sheet-close"]')?.click()

    expect(wrapper.emitted('copy-prompt')).toHaveLength(1)
    expect(wrapper.emitted('copy-path')).toHaveLength(1)
    expect(wrapper.emitted('close')).toHaveLength(1)

    wrapper.unmount()
  })

  it('closes on Escape', () => {
    const wrapper = mountSheet(makeConfig())

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))

    expect(wrapper.emitted('close')).toHaveLength(1)

    wrapper.unmount()
  })
})
