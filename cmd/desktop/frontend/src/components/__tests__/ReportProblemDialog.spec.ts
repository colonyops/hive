import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import ReportProblemDialog from '../ReportProblemDialog.vue'

const mocks = vi.hoisted(() => ({
  Preview: vi.fn(),
  Save: vi.fn(),
  OpenPath: vi.fn(),
}))

vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/reportservice', () => ({
  Preview: mocks.Preview,
  Save: mocks.Save,
}))
vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/systemservice', () => ({
  OpenPath: mocks.OpenPath,
}))
const REPORTS_DIR = '/home/u/.local/share/hive/desktop/reports'
const SAVED = { path: `${REPORTS_DIR}/hive-report-rpt_abc123.json.gz`, dir: REPORTS_DIR }

async function mountDialog() {
  const wrapper = mount(ReportProblemDialog)
  await flushPromises()
  return wrapper
}

function el(testid: string): HTMLElement | null {
  return document.querySelector<HTMLElement>(`[data-testid="${testid}"]`)
}

beforeEach(() => {
  vi.clearAllMocks()
  document.body.innerHTML = ''
  mocks.Preview.mockResolvedValue({ hasSettings: true, flowCount: 2, hasActions: true, hasLogs: true, logBytes: 4096 })
  mocks.Save.mockResolvedValue(SAVED)
  mocks.OpenPath.mockResolvedValue(undefined)
})

describe('ReportProblemDialog', () => {
  it('saves build info only until a surface is switched on', async () => {
    await mountDialog()

    el('report-submit')?.click()
    await flushPromises()

    expect(mocks.Save).toHaveBeenCalledWith({
      includeLogs: false,
      includeSettings: false,
      includeFlows: false,
      includeActions: false,
    })
  })

  it('shows where the bundle landed and opens nothing', async () => {
    await mountDialog()

    el('report-submit')?.click()
    await flushPromises()

    expect(el('report-path')?.textContent).toBe(SAVED.path)
    expect(el('report-open-issue')).toBeNull()
  })

  it('opens the reports folder so the bundle can be read before it is attached', async () => {
    await mountDialog()

    el('report-submit')?.click()
    await flushPromises()
    el('report-reveal')?.click()
    await flushPromises()

    expect(mocks.OpenPath).toHaveBeenCalledWith(REPORTS_DIR)
  })

  it('keeps the dialog open with the error when saving fails', async () => {
    mocks.Save.mockRejectedValue(new Error('disk is full'))
    await mountDialog()

    el('report-submit')?.click()
    await flushPromises()

    expect(el('report-error')?.textContent).toContain('disk is full')
    expect(el('report-success')).toBeNull()
  })
})
