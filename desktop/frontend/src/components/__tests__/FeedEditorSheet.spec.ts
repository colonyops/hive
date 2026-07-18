import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import FeedEditorSheet from '../FeedEditorSheet.vue'
import type { FeedDef, SourceDef } from '../../types/feed'

const cannedSources: SourceDef[] = [
  { id: 'my-prs', kind: 'search', query: 'is:open is:pr author:@me' },
  { id: 'inbox', kind: 'notifications' },
]

function mountEditor(overrides: Record<string, unknown> = {}) {
  return mount(FeedEditorSheet, {
    props: {
      feedId: null,
      initialDef: null,
      sources: cannedSources,
      config: { path: '/cfg/profiles.yaml', exists: true, yaml: '', valid: true, error: '' },
      busy: false,
      error: null,
      sourceBusy: false,
      sourceError: null,
      ...overrides,
    },
  })
}

function el<T extends HTMLElement>(testid: string): T | null {
  return document.querySelector<T>(`[data-testid="${testid}"]`)
}

async function setValue(input: HTMLInputElement | HTMLTextAreaElement, value: string) {
  input.value = value
  input.dispatchEvent(new Event('input', { bubbles: true }))
  await nextTick()
}

async function check(box: HTMLInputElement) {
  box.click()
  await nextTick()
}

describe('FeedEditorSheet', () => {
  it('disables save until a name is set and at least one source is checked', async () => {
    const wrapper = mountEditor()

    expect(el('feed-editor-title')?.textContent).toBe('New feed')
    const save = el<HTMLButtonElement>('feed-editor-save')!
    expect(save.disabled).toBe(true)

    await setValue(el<HTMLInputElement>('feed-editor-name')!, 'Team PRs')
    expect(save.disabled).toBe(true) // sources requirement still unmet

    await check(el<HTMLInputElement>('feed-editor-source-my-prs')!)
    expect(save.disabled).toBe(false)

    await check(el<HTMLInputElement>('feed-editor-source-my-prs')!)
    expect(save.disabled).toBe(true)

    wrapper.unmount()
  })

  it('lists sources with kind badge and query caption', () => {
    const wrapper = mountEditor()

    const list = el('feed-editor-sources')!
    expect(list.textContent).toContain('my-prs')
    expect(list.textContent).toContain('search')
    expect(list.textContent).toContain('is:open is:pr author:@me')
    expect(list.textContent).toContain('inbox')
    expect(list.textContent).toContain('notifications')

    wrapper.unmount()
  })

  it('renders source rows as selectable cards with a distinct selected state', async () => {
    const wrapper = mountEditor()

    const checkbox = el<HTMLInputElement>('feed-editor-source-my-prs')!
    const card = checkbox.closest('label')!
    expect(card.className).toContain('bg-app')
    expect(card.className).not.toContain('bg-raised')

    await check(checkbox)
    expect(card.className).toContain('bg-raised')
    expect(card.className).not.toContain('bg-app')

    wrapper.unmount()
  })

  it('prefills from the initial definition in edit mode and auto-expands populated reasons', async () => {
    const def: FeedDef = {
      id: 'team-prs',
      name: 'Team PRs',
      sources: ['my-prs'],
      filters: { repos: ['acme/*', 'acme/{a,b}/**'], exclude_authors: ['*[bot]'], types: ['pr'], reasons: ['mention'] },
    }
    const wrapper = mountEditor({ feedId: 'team-prs', initialDef: def })
    await nextTick()

    expect(el('feed-editor-title')?.textContent).toBe('Edit feed')
    expect(el<HTMLInputElement>('feed-editor-name')?.value).toBe('Team PRs')
    expect(el<HTMLInputElement>('feed-editor-source-my-prs')?.checked).toBe(true)
    expect(el<HTMLInputElement>('feed-editor-source-inbox')?.checked).toBe(false)
    // Filters are always visible now; reasons still auto-expand when populated.
    expect(el<HTMLTextAreaElement>('feed-editor-repos')?.value).toBe('acme/*\nacme/{a,b}/**')
    expect(el<HTMLTextAreaElement>('feed-editor-exclude-authors')?.value).toBe('*[bot]')
    expect(el<HTMLInputElement>('feed-editor-type-pr')?.checked).toBe(true)
    expect(el<HTMLInputElement>('feed-editor-reason-mention')?.checked).toBe(true)

    wrapper.unmount()
  })

  it('disables save in edit mode until the definition arrives', async () => {
    const wrapper = mountEditor({ feedId: 'team-prs', initialDef: null })

    expect(el<HTMLButtonElement>('feed-editor-save')?.disabled).toBe(true)

    await wrapper.setProps({ initialDef: { id: 'team-prs', name: 'Team PRs', sources: ['my-prs'], filters: {} } })
    expect(el<HTMLButtonElement>('feed-editor-save')?.disabled).toBe(false)

    wrapper.unmount()
  })

  it('emits a save payload with per-line globs kept intact and empty groups omitted', async () => {
    const wrapper = mountEditor()

    await setValue(el<HTMLInputElement>('feed-editor-name')!, '  Team PRs  ')
    await check(el<HTMLInputElement>('feed-editor-source-my-prs')!)
    await check(el<HTMLInputElement>('feed-editor-source-inbox')!)

    // Brace globs contain commas; a line is one pattern, never comma-split.
    await setValue(el<HTMLTextAreaElement>('feed-editor-repos')!, 'acme/{api,web}/**\n acme/cli \n\n')
    await check(el<HTMLInputElement>('feed-editor-type-pr')!)

    el<HTMLButtonElement>('feed-editor-save')!.click()
    await nextTick()

    expect(wrapper.emitted('save')).toEqual([[{
      id: '',
      name: 'Team PRs',
      sources: ['my-prs', 'inbox'],
      filters: { repos: ['acme/{api,web}/**', 'acme/cli'], types: ['pr'] },
    }]])

    wrapper.unmount()
  })

  it('sends filters as an empty object when nothing is filtered', async () => {
    const wrapper = mountEditor()

    await setValue(el<HTMLInputElement>('feed-editor-name')!, 'Everything')
    await check(el<HTMLInputElement>('feed-editor-source-inbox')!)

    el<HTMLButtonElement>('feed-editor-save')!.click()
    await nextTick()

    expect(wrapper.emitted('save')).toEqual([[{ id: '', name: 'Everything', sources: ['inbox'], filters: {} }]])

    wrapper.unmount()
  })

  it('shows the search-items-excluded hint only while a reason is checked', async () => {
    const wrapper = mountEditor()

    el<HTMLButtonElement>('feed-editor-reasons-toggle')!.click()
    await nextTick()
    expect(el('feed-editor-reasons-hint')).toBeNull()

    await check(el<HTMLInputElement>('feed-editor-reason-review_requested')!)
    expect(el('feed-editor-reasons-hint')?.textContent).toContain('search sources are excluded')

    await check(el<HTMLInputElement>('feed-editor-reason-review_requested')!)
    expect(el('feed-editor-reasons-hint')).toBeNull()

    wrapper.unmount()
  })

  it('renders a live YAML preview of the entry as it will be written', async () => {
    const wrapper = mountEditor()

    await setValue(el<HTMLInputElement>('feed-editor-name')!, 'Team PRs')
    await check(el<HTMLInputElement>('feed-editor-source-my-prs')!)
    await setValue(el<HTMLTextAreaElement>('feed-editor-repos')!, 'acme/{api,web}/**')

    const yaml = el('feed-editor-yaml')!.textContent ?? ''
    expect(yaml).toContain('- name: Team PRs')
    expect(yaml).not.toContain('id:') // backend derives the id on create
    expect(yaml).toContain('sources:')
    expect(yaml).toContain('- my-prs')
    expect(yaml).toContain('repos:')
    expect(yaml).toContain('- "acme/{api,web}/**"')

    wrapper.unmount()
  })

  it('creates a source inline and auto-checks it when the list refreshes', async () => {
    const wrapper = mountEditor()

    el<HTMLButtonElement>('feed-editor-new-source-toggle')!.click()
    await nextTick()

    const add = el<HTMLButtonElement>('feed-editor-source-add')!
    expect(add.disabled).toBe(true)

    await setValue(el<HTMLInputElement>('feed-editor-source-id')!, 'team-prs')
    expect(add.disabled).toBe(true) // search kind needs a query

    await setValue(el<HTMLInputElement>('feed-editor-source-query')!, 'org:acme is:pr is:open')
    await setValue(el<HTMLInputElement>('feed-editor-source-limit')!, '25')
    expect(add.disabled).toBe(false)

    add.click()
    await nextTick()

    expect(wrapper.emitted('create-source')).toEqual([[
      { id: 'team-prs', kind: 'search', query: 'org:acme is:pr is:open', limit: 25 },
    ]])

    // Parent appends the created source (backend may have uniquified the id).
    await wrapper.setProps({ sources: [...cannedSources, { id: 'team-prs-2', kind: 'search', query: 'org:acme is:pr is:open' }] })
    await nextTick()

    expect(el<HTMLInputElement>('feed-editor-source-team-prs-2')?.checked).toBe(true)
    expect(el('feed-editor-new-source')).toBeNull() // expander folded away

    wrapper.unmount()
  })

  it('hides the query field for notifications sources and shows backend errors inline', async () => {
    const wrapper = mountEditor({ sourceError: 'source "inbox": kind "notifications" takes no query' })

    el<HTMLButtonElement>('feed-editor-new-source-toggle')!.click()
    await nextTick()

    const kind = el<HTMLSelectElement>('feed-editor-source-kind')!
    kind.value = 'notifications'
    kind.dispatchEvent(new Event('change', { bubbles: true }))
    await nextTick()

    expect(el('feed-editor-source-query')).toBeNull()
    await setValue(el<HTMLInputElement>('feed-editor-source-id')!, 'inbox-2')
    expect(el<HTMLButtonElement>('feed-editor-source-add')?.disabled).toBe(false)
    expect(el('feed-editor-source-error')?.textContent).toContain('takes no query')

    wrapper.unmount()
  })

  it('shows the save error callout and emits close on Escape', async () => {
    const wrapper = mountEditor({ error: 'profile "x": feed "y": unknown source "gone"' })

    expect(el('feed-editor-error')?.textContent).toContain('unknown source')

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    expect(wrapper.emitted('close')).toHaveLength(1)

    wrapper.unmount()
  })

  it('emits copy-prompt and copy-path from the config row', () => {
    const wrapper = mountEditor()

    expect(el('feed-editor-path')?.textContent).toBe('/cfg/profiles.yaml')
    el<HTMLButtonElement>('feed-editor-copy-prompt')!.click()
    el<HTMLButtonElement>('feed-editor-copy-path')!.click()

    expect(wrapper.emitted('copy-prompt')).toHaveLength(1)
    expect(wrapper.emitted('copy-path')).toHaveLength(1)

    wrapper.unmount()
  })

  it('hides the delete button entirely in create mode', () => {
    const wrapper = mountEditor()

    expect(el('feed-editor-delete')).toBeNull()
    expect(el('feed-editor-delete-confirm')).toBeNull()

    wrapper.unmount()
  })

  it('requires a two-step inline confirm before emitting delete', async () => {
    const def: FeedDef = { id: 'team-prs', name: 'Team PRs', sources: ['my-prs'], filters: {} }
    const wrapper = mountEditor({ feedId: 'team-prs', initialDef: def })
    await nextTick()

    expect(el('feed-editor-delete-confirm')).toBeNull()

    el<HTMLButtonElement>('feed-editor-delete')!.click()
    await nextTick()

    // The initial button swaps for the inline confirm row — no modal.
    expect(el('feed-editor-delete')).toBeNull()
    expect(el('feed-editor-delete-confirm')).not.toBeNull()
    expect(wrapper.emitted('delete')).toBeUndefined()

    el<HTMLButtonElement>('feed-editor-delete-confirm')!.click()
    await nextTick()

    expect(wrapper.emitted('delete')).toEqual([['team-prs']])
    // Confirm collapses back to the plain delete button afterward.
    expect(el('feed-editor-delete')).not.toBeNull()
    expect(el('feed-editor-delete-confirm')).toBeNull()

    wrapper.unmount()
  })

  it('cancels the delete confirm without emitting', async () => {
    const def: FeedDef = { id: 'team-prs', name: 'Team PRs', sources: ['my-prs'], filters: {} }
    const wrapper = mountEditor({ feedId: 'team-prs', initialDef: def })
    await nextTick()

    el<HTMLButtonElement>('feed-editor-delete')!.click()
    await nextTick()
    el<HTMLButtonElement>('feed-editor-delete-cancel')!.click()
    await nextTick()

    expect(el('feed-editor-delete')).not.toBeNull()
    expect(el('feed-editor-delete-confirm')).toBeNull()
    expect(wrapper.emitted('delete')).toBeUndefined()

    wrapper.unmount()
  })
})
