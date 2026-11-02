import type { Page } from '@playwright/test'
import { expect, tag, test } from '../../../fixtures/test.js'
import { bindingID } from '../../../fixtures/wails.js'

// Server builds cannot run terminals, so the terminal control plane is mocked. The
// layout, the tabs and the applet's frame are real.
const WINDOW = {
  windowId: '@1', name: 'agent', active: true, activePane: '%1', width: 100, height: 30,
  layout: { paneId: '%1', x: 0, y: 0, width: 100, height: 30, cells: [] },
}
const SESSION = {
  id: 'review', name: 'Review', slug: 'review', repo: 'git@github.com:acme/site.git',
  repoIdentity: 'git@github.com:acme/site.git', server: '', state: 'active', canvasOwner: 'acme/site',
  tags: [], createdAt: '2026-10-01T12:00:00Z',
}

async function openSession(page: Page) {
  const availability = ['terminalservice', 'agentsservice', 'popupterminalservice'].map((name) => bindingID(name, 'Available'))
  const answers: Record<number, unknown> = {
    [bindingID('sessionservice', 'ListSessions')]: [SESSION],
    [bindingID('terminalservice', 'Scratch')]: { slug: '', name: '' },
  }
  await page.route('**/wails/runtime', async (route) => {
    const id = route.request().postDataJSON()?.args?.methodID as number | undefined
    if (availability.includes(id as number)) return route.fulfill({ json: { available: true, reason: '' } })
    if (id !== undefined && id in answers) return route.fulfill({ json: answers[id] })
    return route.fallback()
  })
  await page.route('**/api/terminal/**', async (route) => {
    const path = new URL(route.request().url()).pathname.replace('/api/terminal/', '')
    const headers = {
      'Access-Control-Allow-Origin': route.request().headers().origin ?? '*',
      'Access-Control-Allow-Headers': 'Authorization, Content-Type',
      'Access-Control-Allow-Methods': 'POST, OPTIONS',
    }
    if (route.request().method() === 'OPTIONS') return route.fulfill({ status: 204, headers })
    let answer: unknown = {}
    if (path === 'attach') answer = { windows: [WINDOW] }
    else if (path === 'windows/list') answer = { sessions: { review: [WINDOW] } }
    else if (path === 'agents/sessions' || path === 'agents/sessions/all') answer = { sessions: [] }
    else if (path === 'agents/sessions/activity') answer = { items: [] }
    else if (path === 'agents/workspaces') answer = { root: '/w', available: true, workspaces: [], presets: [], editor: { command: '', title: '' } }
    else return route.fulfill({ status: 204, headers })
    await route.fulfill({ json: answer, headers })
  })
  await page.routeWebSocket('**/api/terminal/stream?*', () => {})
  await page.goto('/#/terminal/review')
  await expect(page.getByTestId('panel-tab-window:@1')).toBeVisible()
  await page.getByTestId('terminal-size-constraint-dismiss').click()
}

async function togglePlace(page: Page, surface: string) {
  await page.goto('/#/settings/applets')
  await page.getByTestId('settings-applet-e2e-page-places').click()
  await page.getByTestId(`settings-applet-e2e-page-places-${surface}`).click()
  await page.keyboard.press('Escape')
  await expect(page.getByTestId('settings-applet-e2e-page-places-menu')).toHaveCount(0)
}

test.describe('applet tabs', { tag: [tag.applets] }, () => {
  test('the + menu has only what the user added, and opens it as a tab or a pop-up', async ({ page }) => {
    await page.goto('/')
    await openSession(page)
    await page.getByTestId('panel-tab-add-p0').click()
    await expect(page.getByTestId('session-add-applet-tab-e2e-page')).toHaveCount(0)
    await page.keyboard.press('Escape')

    await togglePlace(page, 'tab')
    await togglePlace(page, 'page')
    await openSession(page)

    await page.getByTestId('panel-tab-add-p0').click()
    await page.getByTestId('session-add-applet-page-e2e-page').click()
    await expect(page.getByTestId('applet-frame')).toBeVisible()
    await page.getByTestId('applet-page-close').click()

    await page.getByTestId('panel-tab-add-p0').click()
    await page.getByTestId('session-add-applet-tab-e2e-page').click()
    const frame = page.getByTestId('applet-tab-frame')
    await expect(frame).toBeVisible()
    await expect(page.getByTestId('panel-tab-applet:e2e-page')).toHaveText(/E2E page/)

    // Moving an iframe in the DOM would reload the page and lose this.
    const content = page.frameLocator('[data-testid="applet-tab-frame"]')
    await content.locator('body').evaluate(() => {
      ;(window as unknown as { marker: number }).marker = 1
    })
    await page.getByTestId('panel-split-row-p0').click()
    const panels = page.locator('[data-testid^="panel-tab-row-"]')
    await expect(panels).toHaveCount(2)
    const target = (await panels.nth(1).boundingBox())!
    const tab = (await page.getByTestId('panel-tab-applet:e2e-page').boundingBox())!
    await page.mouse.move(tab.x + tab.width / 2, tab.y + tab.height / 2)
    await page.mouse.down()
    await page.mouse.move(target.x + 40, target.y + target.height / 2, { steps: 10 })
    await page.mouse.up()
    await expect(panels.nth(1).getByTestId('panel-tab-applet:e2e-page')).toBeVisible()
    await expect(panels.nth(0).getByTestId('panel-tab-applet:e2e-page')).toHaveCount(0)
    const box = (await frame.boundingBox())!
    expect(box.x).toBeGreaterThanOrEqual(target.x - 1)
    expect(
      await content.locator('body').evaluate(() => (window as unknown as { marker?: number }).marker),
    ).toBe(1)

    await frame.click()
    await page.keyboard.press('Escape')
    await expect(frame).toBeVisible()

    await page.getByTestId('panel-tab-close-applet:e2e-page').click({ force: true })
    await expect(frame).toHaveCount(0)

    await togglePlace(page, 'tab')
    await togglePlace(page, 'page')
  })
})
