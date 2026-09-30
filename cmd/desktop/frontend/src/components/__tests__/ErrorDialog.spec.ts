import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import ErrorDialog from '../ErrorDialog.vue'
import type { ErrorDetails } from '../../composables/useErrorDialog'

const mocks = vi.hoisted(() => ({
  SetText: vi.fn(),
  OpenURL: vi.fn(),
  IssueURL: vi.fn(),
}))

vi.mock('@wailsio/runtime', () => ({
  Clipboard: { SetText: mocks.SetText },
  Browser: { OpenURL: mocks.OpenURL },
}))
vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/reportservice', () => ({
  IssueURL: mocks.IssueURL,
}))

const ISSUE_URL = 'https://github.com/hay-kot/hive-desktop/issues/new?template=bug.yml'
const DETAIL = 'flow "inbox": node "notify-me": action "ping" is not defined in actions.yml'

function details(overrides: Partial<ErrorDetails> = {}): ErrorDetails {
  return {
    title: 'Deploy failed',
    summary: 'Inbox was not written. The version already on disk keeps running.',
    detail: DETAIL,
    context: { flow: 'inbox' },
    ...overrides,
  }
}

async function mountDialog(error: ErrorDetails = details()) {
  const wrapper = mount(ErrorDialog, { props: { error } })
  await flushPromises()
  return wrapper
}

function el(testid: string): HTMLElement | null {
  return document.querySelector<HTMLElement>(`[data-testid="${testid}"]`)
}

beforeEach(() => {
  vi.clearAllMocks()
  document.body.innerHTML = ''
  mocks.SetText.mockResolvedValue(undefined)
  mocks.IssueURL.mockResolvedValue(ISSUE_URL)
  mocks.OpenURL.mockResolvedValue(undefined)
})

describe('ErrorDialog', () => {
  it('shows the error text in full rather than a truncated summary', async () => {
    await mountDialog()

    expect(el('error-dialog')?.getAttribute('role')).toBe('alertdialog')
    expect(el('error-dialog-summary')?.textContent).toContain('keeps running')
    expect(el('error-dialog-detail')?.textContent).toBe(DETAIL)
    expect(el('error-dialog-context')?.textContent).toContain('inbox')
  })

  it('stays open when the backdrop is clicked, so the failure cannot be dismissed by a stray click', async () => {
    const wrapper = await mountDialog()

    el('error-dialog-backdrop')?.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await flushPromises()

    expect(wrapper.emitted('close')).toBeUndefined()
  })

  it('copies the title, context and detail as one block', async () => {
    await mountDialog()

    el('error-dialog-copy')?.click()
    await flushPromises()

    expect(mocks.SetText).toHaveBeenCalledWith(
      `Deploy failed\n\nInbox was not written. The version already on disk keeps running.\n\nflow: inbox\n\n${DETAIL}`,
    )
    expect(el('error-dialog-copy')?.getAttribute('aria-label')).toBe('Copied')
  })

  it('puts copy beside the message, not among the dialog actions', async () => {
    await mountDialog()

    expect(el('error-dialog-message')?.contains(el('error-dialog-copy'))).toBe(true)
  })

  // The error text names flows, nodes and repositories, so it reaches a public
  // issue by the user's paste, never by a prefill.
  it('opens a bug form carrying no error text and dismisses itself', async () => {
    const wrapper = await mountDialog()

    el('error-dialog-report')?.click()
    await flushPromises()

    expect(mocks.OpenURL).toHaveBeenCalledWith(ISSUE_URL)
    expect(ISSUE_URL).not.toContain(encodeURIComponent(DETAIL))
    expect(wrapper.emitted('close')).toHaveLength(1)
  })

  it('says so when the failure carried no text', async () => {
    await mountDialog(details({ detail: '   ', summary: undefined, context: undefined }))

    expect(el('error-dialog-detail')?.textContent).toBe('No further detail was reported.')
  })
})
