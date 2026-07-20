import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import Editor from '../editor.vue'
import { defaults, sink, validate } from '../config'

describe('feed editor', () => {
  it('renders the feed node body (no fields — the node IS the feed)', () => {
    const wrapper = mount(Editor, { props: { config: {} } })
    expect(wrapper.get('[data-testid="feed-node-editor"]').text()).toContain('FEEDS')
  })
})

describe('feed config', () => {
  it('has no config to validate', () => {
    expect(validate(defaults)).toEqual([])
  })

  it('sinks to the flow-qualified node id', () => {
    expect(sink('triage', 'team-feed')).toEqual({ kind: 'feed', targetId: 'triage/team-feed' })
  })
})
