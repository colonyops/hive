import { readFileSync } from 'node:fs'
import type { Page } from '@playwright/test'
import { expect, test } from './fixtures.js'

// Server builds cannot run terminals. Only the terminal control/stream is
// mocked here; clipboard uploads exercise the actual authenticated Go endpoint.
function binding(service: string, method: string): number {
  const source = readFileSync(new URL(`../../frontend/bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/${service}.ts`, import.meta.url), 'utf8')
  const match = source.match(new RegExp(`function ${method}\\([^]*?ByID\\((\\d+)\\)`))
  if (!match) throw new Error(`Missing binding ${service}.${method}`)
  return Number(match[1])
}

const windowState = {
  windowId: '@1', name: 'Images', active: true, activePane: '%1', width: 100, height: 30,
  layout: { paneId: '%1', x: 0, y: 0, width: 100, height: 30, cells: [] },
}
const workspace = { dir: 'images', name: 'Images', command: 'claude', mcps: [], skills: [], schedules: [], problem: '', danger: false, notice: '' }
const session = { id: '42', name: 'Image chat', workspace: 'images', agent: 'claude', lastOpenedAt: 0, slug: 'agentws-42', terminalId: 'agentws-42', windowId: '@1', paneId: '%1', cols: 100, rows: 30, resumeAttempted: false, notice: '', scheduleId: '' }

async function terminalFixture(page: Page) {
  const pastes: { slug: string; paneId: string; text: string }[] = []
  const ptyInput: Buffer[] = []
  const availability = ['terminalservice', 'agentsservice', 'popupterminalservice'].map((name) => binding(name, 'Available'))
  const list = binding('sessionservice', 'ListSessions')
  const scratch = binding('terminalservice', 'Scratch')
  await page.route('**/wails/runtime', async (route) => {
    const id = route.request().postDataJSON()?.args?.methodID
    let answer: unknown
    if (availability.includes(id)) answer = { available: true, reason: '' }
    else if (id === list) answer = [{ id: 'image-session', name: 'Images', slug: 'image-session', repo: '', state: 'active' }]
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
    let answer: unknown = {}
    if (path === 'images/upload' || path === 'images/paths') return route.fallback()
    if (route.request().method() === 'OPTIONS') return route.fulfill({ status: 204, headers })
    if (path === 'attach') answer = { windows: [windowState] }
    else if (path === 'windows/list') answer = { sessions: { 'image-session': [windowState] } }
    else if (path === 'panes/paste') {
      pastes.push(route.request().postDataJSON())
      return route.fulfill({ status: 204, headers })
    } else if (path === 'agents/workspaces') answer = { root: '/images', available: true, workspaces: [workspace], presets: [], editor: { command: '', title: '' } }
    else if (path === 'agents/workspaces/open') answer = { workspace, sessions: [session], missingMcps: [], missingPackages: [] }
    else if (path === 'agents/sessions' || path === 'agents/sessions/all') answer = { sessions: [session] }
    else if (path === 'agents/sessions/resume') answer = session
    else if (path === 'agents/sessions/activity') answer = { items: [] }
    else if (path === 'popup/open') answer = { id: 'popup-images', title: 'Images', dir: '/tmp', command: '', cols: 100, rows: 30 }
    else return route.fulfill({ status: 204, headers })
    await route.fulfill({ json: answer, headers })
  })
  await page.routeWebSocket('**/api/terminal/stream?*', (socket) => {
    socket.send(Buffer.concat([Buffer.from([0, 2]), Buffer.from('@1'), Buffer.from([2]), Buffer.from('%1Image prompt> ')]))
  })
  await page.routeWebSocket('**/api/terminal/pty/stream?*', (socket) => {
    socket.onMessage((message) => ptyInput.push(Buffer.from(message)))
    socket.send(Buffer.concat([Buffer.from([0]), Buffer.from('\x1b[?2004hImage prompt> ')]))
  })
  return { pastes, ptyInput }
}

async function imageGesture(page: Page, kind: 'paste' | 'drop') {
  const png = readFileSync(new URL('./testdata/terminal-image.png', import.meta.url)).toString('base64')
  const pane = page.locator('[data-file-drop-target]:visible').last()
  await expect(pane).toBeVisible()
  await pane.evaluate((host, { kind, png }) => {
    const transfer = new DataTransfer()
    const bytes = Uint8Array.from(atob(png), (char) => char.charCodeAt(0))
    transfer.items.add(new File([bytes], 'screenshot.png', { type: 'image/png' }))
    if (kind === 'paste') host.dispatchEvent(new ClipboardEvent('paste', { bubbles: true, cancelable: true, clipboardData: transfer }))
    else host.dispatchEvent(new DragEvent('drop', { bubbles: true, cancelable: true, dataTransfer: transfer }))
  }, { kind, png })
}

for (const surface of ['Code', 'Chats', 'popup'] as const) {
  test(`${surface} image input uploads bytes and pastes a path without submitting (mock terminal)`, async ({ page }) => {
    const { pastes, ptyInput } = await terminalFixture(page)
    await page.goto(surface === 'Code' ? '/#/terminal/image-session' : surface === 'Chats' ? '/#/workspaces/images?chat=42' : '/')
    if (surface === 'popup') await page.keyboard.press('Control+Backquote')
    await imageGesture(page, 'paste')
    if (surface === 'popup') {
      await expect.poll(() => Buffer.concat(ptyInput).toString()).toContain('terminal-images/')
      const input = Buffer.concat(ptyInput).toString()
      expect(input).toContain('\x1b[200~')
      expect(input).not.toContain('\r')
      expect(input).not.toContain('\n')
    } else {
      await expect.poll(() => pastes.length).toBe(1)
      expect(pastes[0].slug).toBe(surface === 'Code' ? 'image-session' : session.terminalId)
      expect(pastes[0].paneId).toBe('%1')
      expect(pastes[0].text).toContain('terminal-images/')
      expect(pastes[0].text).not.toMatch(/[\r\n]/)
    }
    await imageGesture(page, 'drop')
    if (surface !== 'popup') await expect.poll(() => pastes.length).toBe(2)
    await page.route('**/api/terminal/images/upload', (route) => route.fulfill({ status: 400, headers: { 'Access-Control-Allow-Origin': '*' }, json: { kind: 'invalid', message: 'Image rejected for testing' } }))
    await imageGesture(page, 'paste')
    await expect(page.getByText('Image rejected for testing')).toBeVisible()
  })
}
