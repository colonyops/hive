import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import Editor from '../editor.vue'
import { checkSyntax, compile, defaults, recipes, validate, type Config } from '../config'

function fire(el: Element, type: string) {
  el.dispatchEvent(new Event(type, { bubbles: true }))
}

describe('function editor', () => {
  it('shows on_message and nothing else — it is the node\'s whole lifecycle', () => {
    const wrapper = mount(Editor, { props: { config: defaults } })
    expect(wrapper.find('[data-testid="function-editor-on-message"]').exists()).toBe(true)
    expect(wrapper.findAll('[role="tab"]')).toHaveLength(0)
  })

  it('emits an immutable update:config on an on_message edit, without mutating the config prop', async () => {
    const config: Config = { on_message: 'return msg' }
    const wrapper = mount(Editor, { props: { config } })

    const textarea = wrapper.get<HTMLTextAreaElement>('[data-testid="function-editor-on-message"]').element
    textarea.value = 'return null'
    fire(textarea, 'input')
    await wrapper.vm.$nextTick()

    expect(config.on_message).toBe('return msg')
    expect(wrapper.emitted('update:config')).toEqual([[{ on_message: 'return null' }]])
  })

  it('edits outputs as a number', async () => {
    const config: Config = { on_message: 'return msg' }
    const wrapper = mount(Editor, { props: { config } })

    const input = wrapper.get<HTMLInputElement>('[data-testid="function-editor-outputs"]').element
    input.value = '3'
    fire(input, 'input')
    await wrapper.vm.$nextTick()

    expect(wrapper.emitted('update:config')).toEqual([[{ on_message: 'return msg', outputs: 3 }]])
  })

  it('displays the default timeout as "5s" and parses a typed duration back into milliseconds', async () => {
    const config: Config = { on_message: 'return msg' }
    const wrapper = mount(Editor, { props: { config } })

    expect(wrapper.get<HTMLInputElement>('[data-testid="function-editor-timeout"]').element.value).toBe('5s')

    const input = wrapper.get<HTMLInputElement>('[data-testid="function-editor-timeout"]').element
    input.value = '10s'
    fire(input, 'input')
    await wrapper.vm.$nextTick()

    expect(wrapper.emitted('update:config')).toEqual([[{ on_message: 'return msg', timeout: 10000 }]])
  })

  it('ignores an unparsable timeout instead of emitting a bad value', async () => {
    const config: Config = { on_message: 'return msg' }
    const wrapper = mount(Editor, { props: { config } })

    const input = wrapper.get<HTMLInputElement>('[data-testid="function-editor-timeout"]').element
    input.value = 'not a duration'
    fire(input, 'input')
    await wrapper.vm.$nextTick()

    expect(wrapper.emitted('update:config')).toBeUndefined()
  })

  it('shows a green "no syntax errors" chip for valid source, and a red error count for invalid source', () => {
    const valid = mount(Editor, { props: { config: { on_message: 'return msg' } } })
    expect(valid.get('[data-testid="function-editor-syntax-status"]').text()).toBe('✓ no syntax errors')

    const invalid = mount(Editor, { props: { config: { on_message: 'return msg(' } } })
    expect(invalid.get('[data-testid="function-editor-syntax-status"]').text()).toContain('syntax error')
  })
})

describe('function validate', () => {
  it('requires on_message', () => {
    expect(validate({ on_message: '' })).toEqual(['on_message is required'])
  })

  it('surfaces a syntax error from on_message', () => {
    const errors = validate({ on_message: 'return msg(' })
    expect(errors).toHaveLength(1)
    expect(errors[0]).toEqual(expect.any(String))
  })

  it('passes for valid source with defaults', () => {
    expect(validate(defaults)).toEqual([])
  })

  it('flags outputs out of the 1..16 range', () => {
    expect(validate({ on_message: 'return msg', outputs: 0 })).toContain('outputs must be between 1 and 16')
    expect(validate({ on_message: 'return msg', outputs: 17 })).toContain('outputs must be between 1 and 16')
  })

  it('flags timeout out of the 100ms..60s range', () => {
    expect(validate({ on_message: 'return msg', timeout: 50 })).toContain('timeout must be between 100ms and 60s')
    expect(validate({ on_message: 'return msg', timeout: 70000 })).toContain('timeout must be between 100ms and 60s')
  })
})

describe('function recipes', () => {
  it('a recipe button sets on_message to its snippet', async () => {
    const wrapper = mount(Editor, { props: { config: { on_message: 'return msg' } } })
    await wrapper.get('[data-testid="function-editor-recipe-once"]').trigger('click')
    const emitted = wrapper.emitted('update:config')?.at(-1)?.[0] as Config
    expect(emitted.on_message).toBe(recipes[0].code)
  })

  it('every recipe compiles cleanly through checkSyntax, which now includes kv', () => {
    for (const recipe of recipes) {
      expect(checkSyntax(recipe.code), recipe.id).toEqual([])
    }
  })

  it('a script referencing kv is not flagged by the live check', () => {
    expect(checkSyntax('kv.set("k", 1); return msg')).toEqual([])
  })

  // The "meaningful change" recipe must degrade, not crash, when a source
  // emits a scalar payload: field reads come back undefined and the digest
  // is stable.
  it('recipe #2 tolerates a scalar payload', () => {
    const onChange = recipes.find((r) => r.id === 'on-change')
    expect(onChange).toBeDefined()
    const rows: Record<string, string> = {}
    const kv = {
      get: (k: string) => rows[k],
      set: (k: string, v: unknown) => {
        rows[k] = JSON.stringify(v)
      },
      has: (k: string) => k in rows,
      delete: (k: string) => delete rows[k],
      keys: () => Object.keys(rows),
    }
    const fn = compile(onChange!.code)
    const msg = { SourceKind: 'github', SourceScope: 's', Key: 'k', Payload: 42 }

    const digests: string[] = []
    for (let i = 0; i < 2; i++) {
      expect(() => fn(msg, {}, {}, kv)).not.toThrow()
      digests.push(String(rows[JSON.stringify(['github', 's', 'k'])]))
    }
    expect(digests[0]).toBe(digests[1])
  })
})
