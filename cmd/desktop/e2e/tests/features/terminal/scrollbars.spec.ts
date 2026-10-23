import type { Locator, Page, WebSocketRoute } from '@playwright/test'
import { screenshotPath } from '../../../fixtures/paths.js'
import { expect, test } from '../../../fixtures/test.js'
import { bindingID } from '../../../fixtures/wails.js'

async function terminalFixture(page: Page) {
  const available = bindingID('terminalservice', 'Available')
  const list = bindingID('sessionservice', 'ListSessions')
  const scratch = bindingID('terminalservice', 'Scratch')
  let socket: WebSocketRoute | undefined
  let cols = 100
  let rows = 30
  let split = false
  let zoomed = false
  const state = () => ({
    windowId: '@1', name: 'Scrollback', active: true, activePane: '%1', width: cols, height: rows, zoomed,
    layout: split ? {
      split: 'leftright', x: 0, y: 0, width: cols, height: rows,
      cells: [
        { paneId: '%1', x: 0, y: 0, width: Math.floor(cols / 2), height: rows },
        { paneId: '%2', x: Math.floor(cols / 2) + 1, y: 0, width: cols - Math.floor(cols / 2) - 1, height: rows },
      ],
    } : { paneId: '%1', x: 0, y: 0, width: cols, height: rows },
  })
  const sendLayout = () => socket?.send(Buffer.concat([Buffer.from([1]), Buffer.from(JSON.stringify({ kind: 'layout-changed', ...state() }))]))
  await page.route('**/wails/runtime', async (route) => {
    const id = route.request().postDataJSON()?.args?.methodID
    let answer: unknown
    if (id === available) answer = { available: true, reason: '' }
    else if (id === list) answer = [{ id: 'scrollbars', name: 'Scrollback', slug: 'scrollbars', repo: '', state: 'active' }]
    else if (id === scratch) answer = { slug: '', name: '' }
    else return route.fallback()
    await route.fulfill({ json: answer })
  })
  await page.route('**/api/terminal/**', async (route) => {
    const path = new URL(route.request().url()).pathname.replace('/api/terminal/', '')
    const headers = {
      'Access-Control-Allow-Origin': route.request().headers().origin ?? '*',
      'Access-Control-Allow-Headers': 'Authorization, Content-Type',
      'Access-Control-Allow-Methods': 'POST, OPTIONS',
    }
    if (route.request().method() === 'OPTIONS') return route.fulfill({ status: 204, headers })
    if (path === 'attach') return route.fulfill({ json: { windows: [state()] }, headers })
    if (path === 'windows/list') return route.fulfill({ json: { sessions: { scrollbars: [state()] } }, headers })
    if (path === 'resize') {
      const size = route.request().postDataJSON()
      cols = size.cols
      rows = size.rows
      sendLayout()
    }
    await route.fulfill({ status: 204, headers })
  })
  await page.routeWebSocket('**/api/terminal/stream?*', (stream) => {
    socket = stream
    stream.send(Buffer.concat([
      Buffer.from([0, 2]), Buffer.from('@1'), Buffer.from([2]), Buffer.from('%1'),
      Buffer.from(Array.from({ length: 200 }, (_, i) => `scrollback line ${i}\r\n`).join('')),
    ]))
  })
  return {
    layout(nextSplit: boolean, nextZoomed: boolean) {
      split = nextSplit
      zoomed = nextZoomed
      sendLayout()
    },
  }
}

async function navigate(page: Page, hash: string) {
  await page.evaluate((next) => { location.hash = next }, hash)
}

async function waitForFittedPane(pane: Locator) {
  await expect.poll(() => pane.evaluate((el) => {
    const scrollbar = el.querySelector('.scrollbar.vertical')!
    const slider = scrollbar.querySelector('.slider')!
    const remaining = el.parentElement!.clientWidth - el.clientWidth
    if (remaining < 0 || remaining >= 14) return false
    if (!el.classList.contains('terminal-pane-scrollbar')) return true
    return slider.clientHeight > 0 && slider.clientHeight < scrollbar.clientHeight
  })).toBe(true)
}

async function wheelUp(page: Page, pane: Locator, bottom: Locator) {
  await pane.hover()
  await expect.poll(async () => {
    await page.mouse.wheel(0, -300)
    return bottom.isVisible()
  }).toBe(true)
}

test('terminal scrollbars persist, drag beside the canvas, and follow single/split/zoom layouts', async ({ page, browserName }) => {
  const terminal = await terminalFixture(page)
  await page.goto('/#/terminal/scrollbars')
  const pane = page.locator('[data-pane-id="%1"]')
  const viewport = pane.locator('.xterm-scrollable-element')
  const scrollbar = viewport.locator('.scrollbar.vertical')
  const slider = scrollbar.locator('.slider')
  await expect(viewport).toBeVisible()
  await expect(pane).not.toHaveClass(/terminal-pane-scrollbar/)

  await navigate(page, '/settings/terminal')
  const toggle = page.getByTestId('settings-terminal-show-scrollbars')
  await expect(toggle).toHaveAttribute('aria-checked', 'false')
  await toggle.click()
  await expect(toggle).toHaveAttribute('aria-checked', 'true')
  await navigate(page, '/terminal/scrollbars')
  await expect(pane).toHaveClass(/terminal-pane-scrollbar/)
  await pane.hover()
  await expect(scrollbar).toBeVisible()
  await expect.poll(() => pane.evaluate((el) => {
    const screen = el.querySelector('.xterm-screen')!.getBoundingClientRect()
    const view = el.querySelector('.scrollbar.vertical')!.getBoundingClientRect()
    return Math.round(view.left - screen.right)
  })).toBe(0)

  const bottom = page.getByTestId('terminal-scroll-to-bottom')
  await waitForFittedPane(pane)
  await page.screenshot({ path: screenshotPath(`terminal-scrollbars-${browserName}.png`) })
  await wheelUp(page, pane, bottom)
  await bottom.click()
  await expect(bottom).toBeHidden()

  await waitForFittedPane(pane)
  await pane.hover()
  const box = (await scrollbar.boundingBox())!
  const thumb = (await slider.boundingBox())!
  await page.mouse.move(thumb.x + thumb.width / 2, thumb.y + thumb.height / 2)
  await page.mouse.down()
  await page.mouse.move(box.x + box.width / 2, box.y + 30, { steps: 12 })
  await page.mouse.up()
  await expect(bottom).toBeVisible()
  await bottom.click()
  await expect(bottom).toBeHidden()

  terminal.layout(true, false)
  await expect(page.getByTestId('terminal-pane-host')).toHaveCount(2)
  await expect(pane).not.toHaveClass(/terminal-pane-scrollbar/)
  terminal.layout(true, true)
  await expect(pane).toHaveClass(/terminal-pane-scrollbar/)
  terminal.layout(false, false)
  await expect(pane).toHaveClass(/terminal-pane-scrollbar/)

  await page.reload()
  await expect(pane).toHaveClass(/terminal-pane-scrollbar/)
  await navigate(page, '/settings/terminal')
  await expect(toggle).toHaveAttribute('aria-checked', 'true')
  await toggle.click()
  await navigate(page, '/terminal/scrollbars')
  await expect(pane).not.toHaveClass(/terminal-pane-scrollbar/)
  await waitForFittedPane(pane)
  await wheelUp(page, pane, bottom)
})
