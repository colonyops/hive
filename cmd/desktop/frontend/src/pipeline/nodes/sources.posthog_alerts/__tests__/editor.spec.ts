import { beforeEach, describe, expect, it, vi } from 'vitest'
import Editor from '../editor.vue'
import { defaults, validate } from '../config'
import { connectAccounts, describeCredentialPicker } from '../../../__tests__/credentialPicker'

const mocks = vi.hoisted(() => ({ List: vi.fn() }))

vi.mock(
  '../../../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/integrationsservice',
  () => ({
    List: mocks.List,
  }),
)

const connect = connectAccounts(mocks.List, {
  key: 'posthog',
  title: 'PostHog',
  stability: 'stable',
  provider: 'posthog',
  types: ['sources.posthog_errors', 'sources.posthog_alerts'],
  envOverride: false,
})

beforeEach(() => connect())

describeCredentialPicker({
  node: 'sources.posthog_alerts',
  Editor,
  connect,
  provider: 'posthog',
  accounts: ['us.posthog.com-1', 'us.posthog.com-2'],
  config: (credential) => ({ ...defaults, credential }),
  emptyMessage: 'No PostHog project is connected',
})

describe('sources.posthog_alerts editor', () => {
  // Firing-only is off by default on purpose: emitting every alert is what
  // lets one that stops firing update the item that was already there.
  it('defaults to emitting every alert', () => {
    expect(defaults.firing_only).toBe(false)
  })
})

describe('sources.posthog_alerts validate', () => {
  it('requires a connected project', () => {
    expect(validate(defaults)).toEqual(['a source needs a connected PostHog project'])
  })

  it('rejects a credential that is not a posthog ref', () => {
    expect(validate({ credential: 'grafana/host-1' })).toEqual(['credential must look like "posthog/<account>"'])
  })

  it('accepts a posthog ref', () => {
    expect(validate({ credential: 'posthog/us.posthog.com-1' })).toEqual([])
  })
})
