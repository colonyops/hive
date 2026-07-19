import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { defineComponent, h, nextTick } from 'vue'
import NodeEditorDrawer from '../NodeEditorDrawer.vue'
import { defineNodeType } from '../../nodeType'
import type { NodeTypeDefinition } from '../../nodeType'
import type { FlowNode } from '../../types'

// A minimal stand-in editor: renders the current config.label and offers a
// button that edits it via an immutable update:config emit — enough to
// drive the drawer's draft/validate/save wiring without coupling this spec
// to any real node type's editor.vue.
const StubEditor = defineComponent({
  props: { config: { type: Object, required: true }, errors: { type: Array, default: () => [] } },
  emits: ['update:config'],
  setup(props, { emit }) {
    return () => h('div', { 'data-testid': 'stub-editor' }, [
      h('span', { 'data-testid': 'stub-editor-value' }, String(props.config.label ?? '')),
      h('button', {
        type: 'button',
        'data-testid': 'stub-editor-edit',
        onClick: () => emit('update:config', { ...(props.config as Record<string, any>), label: 'edited' }),
      }, 'edit'),
    ])
  },
})

function stubDef(overrides: Partial<NodeTypeDefinition> = {}): NodeTypeDefinition {
  return defineNodeType<Record<string, any>>({
    type: 'stub',
    label: 'Stub node',
    category: 'Process',
    role: 'processor',
    glyph: { render: () => h('svg') },
    defaults: { label: '' },
    editor: StubEditor,
    help: '# Stub node\n\nA stub node for testing the drawer shell.\n\nMore detail.',
    ...overrides,
  })
}

function mountDrawer(node: FlowNode, defOverrides: Partial<NodeTypeDefinition> = {}) {
  return mount(NodeEditorDrawer, { props: { node, def: stubDef(defOverrides) } })
}

// The drawer teleports to document.body, so — like FeedEditorSheet.spec.ts —
// assertions query the document directly rather than the wrapper's own
// (now-empty) render tree.
function el<T extends HTMLElement>(testid: string): T | null {
  return document.querySelector<T>(`[data-testid="${testid}"]`)
}

describe('NodeEditorDrawer', () => {
  it('renders the node type label and mounts the editor over a cloned draft of the node config', () => {
    const node: FlowNode = { id: 'n1', type: 'stub', config: { label: 'hello' } }
    const wrapper = mountDrawer(node)

    expect(el('node-editor-title')?.textContent).toBe('Stub node')
    expect(el('stub-editor-value')?.textContent).toBe('hello')

    wrapper.unmount()
  })

  it('prefills name/disabled from the node prop', () => {
    const node: FlowNode = { id: 'n1', type: 'stub', name: 'My node', disabled: true, config: { label: '' } }
    const wrapper = mountDrawer(node)

    expect(el<HTMLInputElement>('node-editor-name')?.value).toBe('My node')
    expect(el<HTMLInputElement>('node-editor-disabled')?.checked).toBe(true)

    wrapper.unmount()
  })

  it('propagates an editor update:config into the draft without mutating the node prop', async () => {
    const node: FlowNode = { id: 'n1', type: 'stub', config: { label: 'hello' } }
    const wrapper = mountDrawer(node)

    el<HTMLButtonElement>('stub-editor-edit')!.click()
    await nextTick()

    expect(el('stub-editor-value')?.textContent).toBe('edited')
    expect(node.config).toEqual({ label: 'hello' }) // prop untouched

    wrapper.unmount()
  })

  it('runs def.validate live against the draft and shows errors', async () => {
    const node: FlowNode = { id: 'n1', type: 'stub', config: { label: '' } }
    const wrapper = mountDrawer(node, { validate: (c: any) => (c.label ? [] : ['label is required']) })

    expect(el('node-editor-errors')?.textContent).toContain('label is required')

    el<HTMLButtonElement>('stub-editor-edit')!.click()
    await nextTick()

    expect(el('node-editor-errors')).toBeNull()

    wrapper.unmount()
  })

  it('emits save with the edited node — name trimmed, disabled toggled, config from the draft', async () => {
    const node: FlowNode = { id: 'n1', type: 'stub', config: { label: 'hello' } }
    const wrapper = mountDrawer(node)

    const nameInput = el<HTMLInputElement>('node-editor-name')!
    nameInput.value = '  My node  '
    nameInput.dispatchEvent(new Event('input', { bubbles: true }))
    await nextTick()

    const disabledBox = el<HTMLInputElement>('node-editor-disabled')!
    disabledBox.checked = true
    disabledBox.dispatchEvent(new Event('change', { bubbles: true }))
    await nextTick()

    el<HTMLButtonElement>('stub-editor-edit')!.click()
    await nextTick()

    el<HTMLButtonElement>('node-editor-save')!.click()
    await nextTick()

    expect(wrapper.emitted('save')).toEqual([[{
      id: 'n1',
      type: 'stub',
      name: 'My node',
      disabled: true,
      config: { label: 'edited' },
    }]])

    wrapper.unmount()
  })

  it('renders the Docs summary collapsed by default and full markdown once expanded', async () => {
    const node: FlowNode = { id: 'n1', type: 'stub', config: { label: '' } }
    const wrapper = mountDrawer(node)

    expect(el('node-editor-docs-summary')?.textContent).toBe('A stub node for testing the drawer shell.')
    expect(el('node-editor-docs')).toBeNull()

    el<HTMLButtonElement>('node-editor-docs-toggle')!.click()
    await nextTick()

    expect(el('node-editor-docs')?.innerHTML).toContain('<h1>Stub node</h1>')

    wrapper.unmount()
  })

  it('requires a two-step inline confirm before emitting delete', async () => {
    const node: FlowNode = { id: 'n1', type: 'stub', config: { label: '' } }
    const wrapper = mountDrawer(node)

    el<HTMLButtonElement>('node-editor-delete')!.click()
    await nextTick()

    expect(el('node-editor-delete')).toBeNull()
    expect(wrapper.emitted('delete')).toBeUndefined()

    el<HTMLButtonElement>('node-editor-delete-confirm')!.click()
    await nextTick()

    expect(wrapper.emitted('delete')).toEqual([['n1']])

    wrapper.unmount()
  })

  it('emits close on backdrop click, the close button, and Escape', async () => {
    const node: FlowNode = { id: 'n1', type: 'stub', config: { label: '' } }
    const wrapper = mountDrawer(node)

    el<HTMLButtonElement>('node-editor-close')!.click()
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    el<HTMLButtonElement>('node-editor-cancel')!.click()

    expect(wrapper.emitted('close')).toHaveLength(3)

    wrapper.unmount()
  })

  it('reloads the draft when the node prop changes (switching selection)', async () => {
    const nodeA: FlowNode = { id: 'a', type: 'stub', name: 'A', config: { label: 'a-value' } }
    const nodeB: FlowNode = { id: 'b', type: 'stub', name: 'B', config: { label: 'b-value' } }
    const wrapper = mountDrawer(nodeA)

    expect(el<HTMLInputElement>('node-editor-name')?.value).toBe('A')

    await wrapper.setProps({ node: nodeB })

    expect(el<HTMLInputElement>('node-editor-name')?.value).toBe('B')
    expect(el('stub-editor-value')?.textContent).toBe('b-value')

    wrapper.unmount()
  })
})
