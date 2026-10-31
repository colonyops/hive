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

  test('an applet whose title has the query comes first under its own header', async ({ page }) => {
    await page.keyboard.press('Meta+k')
    await page.getByTestId('command-palette-input').fill('E2E page')
    await expect(page.getByTestId('command-palette-command-title').first()).toHaveText('E2E page')
    await expect(page.getByTestId('command-palette').getByText('Applets', { exact: true }).first()).toBeVisible()
  })

  test('the palette has a tab of only applets', async ({ page }) => {
    await page.keyboard.press('Meta+k')
    await page.getByTestId('command-palette-input').fill('+')
    const titles = page.getByTestId('command-palette-command-title')
    await expect(titles.filter({ hasText: 'Applet Builder' })).toHaveCount(1)
    await expect(titles.filter({ hasText: 'E2E page' })).toHaveCount(1)
    await expect(titles).toHaveCount(2)
  })

  test('the builder fits on one screen and copies a prompt that asks what to build', async ({ page }) => {
    await openApplet(page, 'Applet Builder')
    const frame = page.frameLocator('[data-testid="applet-frame"]')
    await expect(frame.getByTestId('builder-style-hive')).toHaveAttribute('aria-pressed', 'true')

    await frame.getByTestId('builder-style-reference').click()
    await expect(frame.getByTestId('builder-copy')).toBeDisabled()
    await frame.getByTestId('builder-reference').fill('an old Mac')
    await frame.getByTestId('builder-copy').click()

    await expect(page.getByTestId('applet-page-overlay')).toHaveCount(0)
    await expect(page.getByText('Prompt copied')).toBeVisible()
  })

  test('settings lists the applets and what they have running', async ({ page }) => {
    await page.goto('/#/settings/applets')
    await expect(page.getByTestId('settings-applet-applet-builder-state')).toHaveText('Ready')
    await expect(page.getByTestId('settings-applet-e2e-page')).toBeVisible()
    await expect(page.getByTestId('settings-applets-processes-empty')).toBeVisible()

    await page.getByTestId('settings-applet-applet-builder-open').click()
    await expect(page.frameLocator('[data-testid="applet-frame"]').getByTestId('builder-copy')).toBeVisible()
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
    const outside = await page.request.get('/__applet/e2e-page/..%2fe2e-other%2fapplet.yaml')
    expect(outside.status()).toBe(404)

    const pageFile = await page.request.get('/__applet/e2e-page/index.html')
    expect(pageFile.headers()['access-control-allow-origin']).toBeUndefined()
    const moduleFile = await page.request.get('/__applet/e2e-page/helper.mjs')
    expect(moduleFile.headers()['access-control-allow-origin']).toBe('null')
  })
})
