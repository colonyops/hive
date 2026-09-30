import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import Editor, { type WebhookCaptureView, type WebhookEditorClient } from '../editor.vue'
import type { Config } from '../config'

// The transform prompt is assembled by the Go prompts service; this editor
// only supplies the endpoint path and the last captured delivery.
const mocks = vi.hoisted(() => ({ Render: vi.fn(), SetText: vi.fn(), fileToImageBase64: vi.fn() }))

vi.mock('../../../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/promptsservice', () => ({
  Catalog: vi.fn(),
  Render: mocks.Render,
}))

vi.mock('@wailsio/runtime', () => ({ Clipboard: { SetText: mocks.SetText } }))

// The picker's FileReader read is stubbed: its callback is not a microtask, so
// flushPromises would not await it. The bytes are the backend's concern anyway.
vi.mock('../../../../lib/imageUpload', async (importOriginal) => ({
  ...await importOriginal<typeof import('../../../../lib/imageUpload')>(),
  fileToImageBase64: mocks.fileToImageBase64,
}))

beforeEach(() => {
  mocks.Render.mockReset()
  mocks.SetText.mockReset()
  mocks.SetText.mockResolvedValue(undefined)
  mocks.Render.mockResolvedValue({ id: 'webhook-transform', title: '', description: '', target: '', text: 'TRANSFORM PROMPT' })
  mocks.fileToImageBase64.mockReset()
  mocks.fileToImageBase64.mockResolvedValue('PICKED')
})

function fakeClient(capture?: Partial<WebhookCaptureView>, overrides: Partial<WebhookEditorClient> = {}): WebhookEditorClient {
  return {
    async info() {
      return { running: true, port: 4483, baseUrl: 'http://127.0.0.1:4483/hooks/' }
    },
    async capture() {
      return { receivedAt: 0, body: '', feedShaped: false, missingFields: [], ...capture }
    },
    async setMarkImage(data: string) {
      return { hash: 'a'.repeat(32), image: `data:image/png;base64,${data}` }
    },
    async markImage() {
      return undefined
    },
    ...overrides,
  }
}

function mountEditor(config: Config, capture?: Partial<WebhookCaptureView>, overrides?: Partial<WebhookEditorClient>) {
  return mount(Editor, {
    props: { config, flowId: 'triage', nodeId: 'hook', client: fakeClient(capture, overrides) },
  })
}

function selectMarkFile(wrapper: ReturnType<typeof mountEditor>, file: File): Promise<void> {
  const input = wrapper.get('[data-testid="sources.webhook-editor-mark-input"]').element as HTMLInputElement
  Object.defineProperty(input, 'files', { value: [file], configurable: true })
  return wrapper.get('[data-testid="sources.webhook-editor-mark-input"]').trigger('change')
}

describe('sources.webhook editor', () => {
  it('renders the endpoint URL from the listener info and the configured path', async () => {
    const wrapper = mountEditor({ path: 'ci-alerts' })
    await flushPromises()
    expect(wrapper.get('[data-testid="sources.webhook-editor-url"]').text()).toBe('http://127.0.0.1:4483/hooks/ci-alerts')
  })

  it('emits an immutable update:config on path edit', async () => {
    const config: Config = { path: '' }
    const wrapper = mountEditor(config)
    const input = wrapper.get<HTMLInputElement>('[data-testid="sources.webhook-editor-path"]').element
    input.value = 'ci'
    input.dispatchEvent(new Event('input', { bubbles: true }))
    await wrapper.vm.$nextTick()

    expect(config.path).toBe('')
    expect(wrapper.emitted('update:config')).toEqual([[{ path: 'ci' }]])
  })

  it('regenerates the path in place, emitting a valid slug without touching the secret', async () => {
    const config: Config = { path: 'ci', secret: 'keep-me' }
    const wrapper = mountEditor(config)
    await wrapper.get('[data-testid="sources.webhook-editor-path-generate"]').trigger('click')

    const emitted = wrapper.emitted('update:config') as [[Config]]
    expect(emitted[0]![0].path).toMatch(/^hook-[a-z0-9]{8}$/)
    expect(emitted[0]![0].path).not.toBe('ci')
    expect(emitted[0]![0].secret).toBe('keep-me')
    expect(config.path).toBe('ci')
  })

  it('regenerates the secret in place, emitting a printable value without touching the path', async () => {
    const config: Config = { path: 'ci' }
    const wrapper = mountEditor(config)
    await wrapper.get('[data-testid="sources.webhook-editor-secret-generate"]').trigger('click')

    const emitted = wrapper.emitted('update:config') as [[Config]]
    expect(emitted[0]![0].secret).toMatch(/^[!-~]{32}$/)
    expect(emitted[0]![0].path).toBe('ci')
    expect(config.secret).toBeUndefined()
  })

  it('shows the placeholder when nothing was captured yet', async () => {
    const wrapper = mountEditor({ path: 'ci' })
    await flushPromises()
    expect(wrapper.find('[data-testid="sources.webhook-editor-no-capture"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="sources.webhook-editor-capture"]').exists()).toBe(false)
  })

  it('shows the captured payload with the non-blocking shape warning', async () => {
    const wrapper = mountEditor({ path: 'ci' }, {
      receivedAt: 1750000000000,
      body: '{"event":"deploy"}',
      feedShaped: false,
      missingFields: ['id', 'kind', 'repo', 'title', 'url'],
    })
    await flushPromises()
    expect(wrapper.get('[data-testid="sources.webhook-editor-capture"]').text()).toContain('"event": "deploy"')
    const warning = wrapper.get('[data-testid="sources.webhook-editor-shape-warning"]').text()
    expect(warning).toContain('Missing canonical item fields')
    expect(warning).toContain('id, kind, repo, title, url')
    expect(warning).toContain('renders minimally')
    expect(warning).toContain('ADR canonical-item-contract')
    expect(warning).toContain('auto-archive')
  })

  it('hides the shape warning for feed-shaped captures', async () => {
    const wrapper = mountEditor({ path: 'ci' }, {
      receivedAt: 1750000000000,
      body: '{"id":"1","kind":"PR","repo":"o/r","title":"t","url":"https://x"}',
      feedShaped: true,
      missingFields: null,
    })
    await flushPromises()
    expect(wrapper.find('[data-testid="sources.webhook-editor-shape-warning"]').exists()).toBe(false)
  })

  it('copies the service-rendered transform prompt, scoped to this node', async () => {
    const wrapper = mountEditor({ path: 'ci-alerts' }, { receivedAt: 5, body: '{"event":"deploy"}' })
    await flushPromises()

    await wrapper.get('[data-testid="sources.webhook-editor-copy-prompt"]').trigger('click')
    await flushPromises()

    expect(mocks.Render).toHaveBeenCalledWith('webhook-transform', expect.objectContaining({
      webhookPath: 'ci-alerts',
      webhookSample: '{"event":"deploy"}',
    }))
    expect(mocks.SetText).toHaveBeenCalledWith('TRANSFORM PROMPT')
  })

  it('passes an empty sample when nothing has been captured yet', async () => {
    const wrapper = mountEditor({ path: 'ci-alerts' })
    await flushPromises()

    await wrapper.get('[data-testid="sources.webhook-editor-copy-prompt"]').trigger('click')
    await flushPromises()

    expect(mocks.Render).toHaveBeenCalledWith('webhook-transform', expect.objectContaining({ webhookSample: '' }))
  })

  it('uploads a picked mark image and emits its hash into the config', async () => {
    const config: Config = { path: 'ci' }
    const wrapper = mountEditor(config)
    await flushPromises()

    // The preview falls back to the glyph while no image is set.
    expect(wrapper.find('[data-testid="sources.webhook-editor-mark-preview"] img').exists()).toBe(false)

    await selectMarkFile(wrapper, new File([Uint8Array.from([1, 2, 3])], 'logo.png', { type: 'image/png' }))
    await flushPromises()

    const emitted = wrapper.emitted('update:config') as [[Config]]
    expect(emitted.at(-1)![0].image).toBe('a'.repeat(32))
    // The stored PNG returned by the upload previews immediately.
    expect(wrapper.get('[data-testid="sources.webhook-editor-mark-preview"] img').attributes('src')).toContain('data:image/png;base64,')
    // The original config object is never mutated in place.
    expect(config.image).toBeUndefined()
  })

  it('previews an already-configured image by resolving its hash', async () => {
    const wrapper = mountEditor({ path: 'ci', image: 'b'.repeat(32) }, undefined, {
      async markImage() { return 'data:image/png;base64,STORED' },
    })
    await flushPromises()

    expect(wrapper.get('[data-testid="sources.webhook-editor-mark-preview"] img').attributes('src')).toBe('data:image/png;base64,STORED')
  })

  it('removes the mark image, emitting a config with no image', async () => {
    const config: Config = { path: 'ci', image: 'b'.repeat(32) }
    const wrapper = mountEditor(config, undefined, {
      async markImage() { return 'data:image/png;base64,STORED' },
    })
    await flushPromises()

    await wrapper.get('[data-testid="sources.webhook-editor-mark-remove"]').trigger('click')

    const emitted = wrapper.emitted('update:config') as [[Config]]
    expect(emitted.at(-1)![0].image).toBeUndefined()
    expect(wrapper.find('[data-testid="sources.webhook-editor-mark-preview"] img').exists()).toBe(false)
  })
})
