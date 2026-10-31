import type { Page } from '@playwright/test'
import { expect, tag, test } from '../../../fixtures/test.js'

async function openApplet(page: Page, title: string): Promise<void> {
  await page.keyboard.press('Meta+k')
  await page.getByTestId('command-palette-input').fill(title)
  await expect(page.getByTestId('command-palette-command-title').first()).toHaveText(title)
  await page.keyboard.press('Enter')
}

test.describe('applets', { tag: [tag.applets] }, () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/')
    await expect(page.getByTestId('feed-item').first()).toBeVisible()
  })

  test('a table renders its command and runs actions against the selected row', async ({ page }) => {
    await openApplet(page, 'E2E table')
    await expect(page.getByTestId('applet-overlay')).toBeVisible()
    await expect(page.getByTestId('applet-row')).toHaveCount(2)

    await page.keyboard.press('j')
    await expect(page.locator('[data-testid="applet-row"][aria-selected="true"]')).toContainText('beta')

    await page.keyboard.press('n')
    await expect(page.getByText('said beta; touch injected')).toBeVisible()

    await page.keyboard.press('b')
    await expect(page.getByText('kaboom')).toBeVisible()

    await page.keyboard.press('c')
    await expect(page.getByTestId('applet-confirm')).toContainText('Really do it?')
    await page.keyboard.press('Escape')
    await expect(page.getByTestId('applet-confirm')).toHaveCount(0)
    await expect(page.getByTestId('applet-overlay')).toBeVisible()

    await page.keyboard.press('Escape')
    await expect(page.getByTestId('applet-overlay')).toHaveCount(0)
  })

  test('a picker filters as you type and Enter runs the first action', async ({ page }) => {
    await openApplet(page, 'E2E picker')
    await expect(page.getByTestId('applet-row')).toHaveCount(3)

    await page.keyboard.type('bui')
    await expect(page.getByTestId('applet-row')).toHaveCount(1)

    await page.keyboard.press('Enter')
    await expect(page.getByTestId('applet-picker')).toHaveCount(0)
    await expect(page.getByText('picked build')).toBeVisible()
  })

  test('a page reaches the machine through window.hive and keeps its localStorage', async ({ page }) => {
    await openApplet(page, 'E2E page')
    const result = page.frameLocator('[data-testid="applet-frame"]').locator('#result')
    await expect(result).toContainText('"file":"written"')

    const first = JSON.parse((await result.textContent()) ?? '{}') as Record<string, unknown>
    expect(first).toMatchObject({
      fromModule: 'module loaded',
      bypassed: false,
      context: 'e2e-page',
      exec: { stdout: 'out\n', stderr: 'err\n', code: 3 },
      file: 'written',
      spawn: { out: 'got hello\n', code: 5 },
    })
    await expect(page.getByText('page toast')).toBeVisible()

    // The save is debounced, and closing drops a pending one.
    const visits = Number(first.visits)
    await expect
      .poll(async () => (await page.request.get('/__applet/e2e-page/index.html')).text())
      .toContain(`"visits":"${visits + 1}"`)

    await page.getByTestId('applet-page-close').click()
    await expect(page.getByTestId('applet-page-overlay')).toHaveCount(0)

    await openApplet(page, 'E2E page')
    await expect(result).toContainText(`"visits":${visits + 1}`)
  })

  test('a page the frame navigates to gets no answer from the bridge', async ({ page }) => {
    await openApplet(page, 'E2E page')
    const frame = page.frameLocator('[data-testid="applet-frame"]')
    await expect(frame.locator('#result')).toContainText('"file":"written"')

    await frame.locator('#navigate').click()
    await expect(frame.locator('#left')).toHaveText('ignored')

    await page.getByTestId('applet-page-close').click()
    await openApplet(page, 'E2E page')
    await expect(frame.locator('#result')).toContainText('"bypassed":false')
  })

  test('the asset route hides saved storage and gives a page no CORS', async ({ page }) => {
    const hidden = await page.request.get('/__applet/e2e-page/.localstorage.json')
    expect(hidden.status()).toBe(404)
    const outside = await page.request.get('/__applet/e2e-page/..%2fe2e-table%2fapplet.yaml')
    expect(outside.status()).toBe(404)

    const pageFile = await page.request.get('/__applet/e2e-page/index.html')
    expect(pageFile.headers()['access-control-allow-origin']).toBeUndefined()
    const moduleFile = await page.request.get('/__applet/e2e-page/helper.mjs')
    expect(moduleFile.headers()['access-control-allow-origin']).toBe('null')
  })
})
