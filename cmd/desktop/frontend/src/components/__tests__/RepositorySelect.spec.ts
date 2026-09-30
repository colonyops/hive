import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import RepositorySelect from '../RepositorySelect.vue'

const repositories = [
  { name: 'site', repository: 'https://github.com/acme/site.git' },
  { name: 'hive-desktop', repository: 'https://github.com/hay-kot/hive-desktop.git' },
  { name: 'fix-crash', repository: 'https://github.com/hay-kot/hive.git' },
]

function mountSelect(modelValue = '') {
  return mount(RepositorySelect, {
    attachTo: document.body,
    props: { modelValue, repositories, testid: 'repo' },
    global: { stubs: { Teleport: true } },
  })
}

async function openList(wrapper: ReturnType<typeof mountSelect>) {
  await wrapper.get('[data-testid="repo"]').trigger('click')
}

describe('RepositorySelect', () => {
  it('shows the selected remote as owner/name', () => {
    const wrapper = mountSelect('https://github.com/hay-kot/hive-desktop.git')
    expect(wrapper.get('[data-testid="repo"]').text()).toContain('hay-kot/hive-desktop')
  })

  it('prompts when nothing is selected', () => {
    expect(mountSelect().get('[data-testid="repo"]').text()).toContain('Choose a repository')
  })

  it('lists every repository on open and emits the remote on click', async () => {
    const wrapper = mountSelect()
    await openList(wrapper)
    const options = wrapper.findAll('[data-testid="repo-option"]')
    expect(options).toHaveLength(3)
    await options[1].trigger('click')
    expect(wrapper.emitted('update:modelValue')).toEqual([['https://github.com/hay-kot/hive-desktop.git']])
  })

  it('filters by fuzzy match on owner/name', async () => {
    const wrapper = mountSelect()
    await openList(wrapper)
    await wrapper.get('[data-testid="repo-search"]').setValue('hivedesk')
    const options = wrapper.findAll('[data-testid="repo-option"]')
    expect(options).toHaveLength(1)
    expect(options[0].text()).toContain('hay-kot/hive-desktop')
  })

  it('puts the selected repository first so it is visible on reopen', async () => {
    const wrapper = mountSelect('https://github.com/hay-kot/hive.git')
    await openList(wrapper)
    expect(wrapper.findAll('[data-testid="repo-option"]')[0].text()).toContain('hay-kot/hive')
  })

  it('offers a typed remote that is not on disk', async () => {
    const wrapper = mountSelect()
    await openList(wrapper)
    await wrapper.get('[data-testid="repo-search"]').setValue('git@github.com:other/repo.git')
    expect(wrapper.findAll('[data-testid="repo-option"]')).toHaveLength(0)
    await wrapper.get('[data-testid="repo-custom"]').trigger('click')
    expect(wrapper.emitted('update:modelValue')).toEqual([['git@github.com:other/repo.git']])
  })

  it('does not offer a half-typed name as a remote', async () => {
    const wrapper = mountSelect()
    await openList(wrapper)
    await wrapper.get('[data-testid="repo-search"]').setValue('hive')
    expect(wrapper.find('[data-testid="repo-custom"]').exists()).toBe(false)
  })

  it('reports an empty search rather than showing nothing', async () => {
    const wrapper = mountSelect()
    await openList(wrapper)
    await wrapper.get('[data-testid="repo-search"]').setValue('zzzz')
    expect(wrapper.get('[data-testid="repo-empty"]').text()).toContain('No matching repository')
  })

  it('commits the top match on Enter', async () => {
    const wrapper = mountSelect()
    await openList(wrapper)
    const search = wrapper.get('[data-testid="repo-search"]')
    await search.setValue('hivedesk')
    await search.trigger('keydown', { key: 'Enter' })
    expect(wrapper.emitted('update:modelValue')).toEqual([['https://github.com/hay-kot/hive-desktop.git']])
  })

  it('moves the active row with the arrow keys', async () => {
    const wrapper = mountSelect()
    await openList(wrapper)
    const search = wrapper.get('[data-testid="repo-search"]')
    await search.trigger('keydown', { key: 'ArrowDown' })
    await search.trigger('keydown', { key: 'Enter' })
    expect(wrapper.emitted('update:modelValue')).toEqual([['https://github.com/hay-kot/hive-desktop.git']])
  })

  it('closes on Escape without choosing', async () => {
    const wrapper = mountSelect()
    await openList(wrapper)
    await wrapper.get('[data-testid="repo-search"]').trigger('keydown', { key: 'Escape' })
    expect(wrapper.find('[data-testid="repo-popover"]').exists()).toBe(false)
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
  })

  it('hands focus back to the trigger after choosing, so Tab reaches the next field', async () => {
    const wrapper = mountSelect()
    await openList(wrapper)
    ;(wrapper.get('[data-testid="repo-search"]').element as HTMLInputElement).focus()

    await wrapper.findAll('[data-testid="repo-option"]')[0].trigger('click')

    expect(document.activeElement).toBe(wrapper.get('[data-testid="repo"]').element)
  })

  it('hands focus back to the trigger on Escape', async () => {
    const wrapper = mountSelect()
    await openList(wrapper)
    const search = wrapper.get('[data-testid="repo-search"]')
    ;(search.element as HTMLInputElement).focus()

    await search.trigger('keydown', { key: 'Escape' })

    expect(document.activeElement).toBe(wrapper.get('[data-testid="repo"]').element)
  })

  it('closes on Tab so focus moves on rather than into the list', async () => {
    const wrapper = mountSelect()
    await openList(wrapper)
    const search = wrapper.get('[data-testid="repo-search"]')
    ;(search.element as HTMLInputElement).focus()

    await search.trigger('keydown', { key: 'Tab' })

    expect(wrapper.find('[data-testid="repo-popover"]').exists()).toBe(false)
    expect(document.activeElement).toBe(wrapper.get('[data-testid="repo"]').element)
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
  })

  it('says so when no repositories are configured', async () => {
    const wrapper = mount(RepositorySelect, {
      attachTo: document.body,
      props: { modelValue: '', repositories: null, testid: 'repo' },
      global: { stubs: { Teleport: true } },
    })
    await openList(wrapper as unknown as ReturnType<typeof mountSelect>)
    expect(wrapper.get('[data-testid="repo-empty"]').text()).toContain('No repositories configured')
  })
})
