import { execFileSync } from 'node:child_process'
import { type Page } from '@playwright/test'
import { serverURL } from '../../../fixtures/servers.js'
import { expect, resetServerState, tag, test } from '../../../fixtures/test.js'
import { bindingID } from '../../../fixtures/wails.js'

// The Code view against a real server: server-a from test/remote, reached over
// ssh the way the app reaches it. scripts/serve.sh writes the ssh config the
// app uses, and this file reads the server's state through the same config.
//
// The server build has no terminals, so only terminal availability and the
// terminal control plane are answered here; the session list, create, and
// delete go to server-a. test/remote/terminal_test.go runs the terminal of a
// server session over ssh.
const SSH_CONFIG_GLOB = '/tmp/hive-desktop-e2e.*/remote/ssh/config'
const SESSION = `e2e-remote-${Date.now().toString(36)}`
const SLUG = `server-a:${SESSION}`

function onServer(command: string): string {
  const config = execFileSync('bash', ['-c', `ls ${SSH_CONFIG_GLOB} | head -1`]).toString().trim()
  return execFileSync('ssh', ['-F', config, '-T', '-o', 'BatchMode=yes', 'server-a', '--', command], {
    timeout: 20_000,
  }).toString()
}

function serverGroup(page: Page, name: string) {
  return page.locator(`[data-testid="terminal-server-group"][data-repo="server:${name}"]`)
}

// Records each attach so a test can see which terminal the row opened.
async function stubTerminals(page: Page): Promise<string[]> {
  const attached: string[] = []
  const available = bindingID('terminalservice', 'Available')
  const scratch = bindingID('terminalservice', 'Scratch')
  await page.route('**/wails/runtime', async (route) => {
    const id = route.request().postDataJSON()?.args?.methodID
    if (id === available) return route.fulfill({ json: { available: true, reason: '' } })
    if (id === scratch) return route.fulfill({ json: { slug: '', name: '' } })
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
    if (path === 'attach') {
      attached.push(route.request().postDataJSON().slug)
      return route.fulfill({ json: { windows: [] }, headers })
    }
    if (path === 'windows/list') return route.fulfill({ json: { sessions: {} }, headers })
    return route.fulfill({ status: 204, headers })
  })
  await page.routeWebSocket('**/api/terminal/stream?*', () => {})
  return attached
}

async function expand(page: Page, name: string): Promise<void> {
  const group = serverGroup(page, name)
  if ((await group.getAttribute('aria-expanded')) === 'false') await group.click()
}

test.describe('sessions on a server', { tag: [tag.terminal, tag.critical] }, () => {
  test.use({ server: 'remote', serverStateReset: 'per-file' })
  test.describe.configure({ mode: 'serial', retries: 0 })
  test.beforeAll(async ({ browserName }) => {
    await resetServerState(serverURL('remote', browserName))
  })
  test.afterAll(() => {
    try {
      onServer(`hive session delete --force ${SESSION}`)
    } catch {
      // The journey deletes it; this only catches a run that stopped early.
    }
  })

  test('lists each server, and an offline one does not hold up the tree', async ({ page }) => {
    await stubTerminals(page)
    await page.goto('/#/terminal')

    await expect(page.getByTestId('terminal-mode')).toBeVisible()
    await expect(page.getByTestId('terminal-sessions-loading')).toBeHidden()
    await expect(serverGroup(page, 'server-a')).toHaveAttribute('data-server-state', 'connected', { timeout: 30_000 })
    await expect(serverGroup(page, 'gone')).toHaveAttribute('data-server-state', 'offline', { timeout: 30_000 })
    await expand(page, 'gone')
    await expect(page.getByTestId('terminal-server-error')).toContainText('Connection refused')
  })

  test('creates a session on the server from its group', async ({ page }) => {
    await stubTerminals(page)
    await page.goto('/#/terminal')
    const group = serverGroup(page, 'server-a')
    await expect(group).toHaveAttribute('data-server-state', 'connected', { timeout: 30_000 })
    await group.hover()
    await group.getByTestId('terminal-server-new-session').click()

    await expect(page.getByTestId('new-session-dialog')).toBeVisible()
    await page.getByTestId('new-session-repository').click()
    // windows.git's rule makes a tmux session the app can attach to.
    await page.getByTestId('new-session-repository-search').fill('/home/hive/windows.git')
    await page.getByTestId('new-session-repository-custom').click()
    await page.getByTestId('new-session-name').fill(SESSION)
    await page.getByTestId('new-session-submit').click()
    await expect(page.getByTestId('new-session-dialog')).toBeHidden()

    await expand(page, 'server-a')
    await expect(page.locator(`[data-testid="terminal-session-row"][data-slug="${SLUG}"]`)).toBeVisible({
      timeout: 60_000,
    })
    expect(onServer(`hive session list --json`)).toContain(`"name":"${SESSION}"`)
  })

  test('opens the session as that server\'s terminal', async ({ page }) => {
    const attached = await stubTerminals(page)
    await page.goto('/#/terminal')
    await expand(page, 'server-a')
    await page.locator(`[data-testid="terminal-session-row"][data-slug="${SLUG}"]`).click()

    await expect(page).toHaveURL(new RegExp(`/terminal/${SLUG}$`))
    await expect.poll(() => attached).toContain(SLUG)
  })

  test('deletes the session on the server', async ({ page }) => {
    await stubTerminals(page)
    await page.goto('/#/terminal')
    await expand(page, 'server-a')
    const row = page.locator(`[data-testid="terminal-session-row"][data-slug="${SLUG}"]`)
    await row.hover()
    await row.getByTestId('terminal-session-menu-toggle').click()
    await expect(page.getByTestId('session-menu-start')).toHaveCount(0)
    await page.getByTestId('session-menu-delete').click()
    await page.getByTestId('session-confirmation-confirm').click()

    await expect(row).toBeHidden({ timeout: 60_000 })
    expect(onServer(`hive session list --json`)).not.toContain(`"name":"${SESSION}"`)
  })
})
