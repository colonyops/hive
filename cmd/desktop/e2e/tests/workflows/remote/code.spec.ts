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

function sessionRow(page: Page) {
  return page.locator(`[data-testid="terminal-session-row"][data-slug="${SLUG}"]`)
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

// A query opens every group it matches, so the session's row renders in
// whichever repository group it joined.
async function findSession(page: Page): Promise<void> {
  await page.getByTestId('terminal-sessions-filter').fill(SESSION)
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

  // A clone on a CI runner can take most of a minute.
  test.setTimeout(120_000)

  test('reports an offline server without holding up the tree', async ({ page }) => {
    await stubTerminals(page)
    await page.goto('/#/terminal')

    await expect(page.getByTestId('terminal-mode')).toBeVisible()
    await expect(page.getByTestId('terminal-sessions-loading')).toBeHidden()
    await expect(page.getByTestId('terminal-server-offline-gone')).toContainText('gone offline', { timeout: 30_000 })
    await expect(page.getByTestId('terminal-server-offline-server-a')).toHaveCount(0)
  })

  test('creates a session on the server chosen in the form', async ({ page }) => {
    await stubTerminals(page)
    await page.goto('/#/terminal')
    await expect(page.getByTestId('terminal-server-offline-gone')).toBeVisible({ timeout: 30_000 })
    await page.getByTestId('terminal-new-session').click()

    await expect(page.getByTestId('new-session-dialog')).toBeVisible()
    await page.getByTestId('new-session-server').click()
    await page.getByTestId('new-session-server-option-server-a').click()
    await page.getByTestId('new-session-repository').click()
    await page.getByTestId('new-session-repository-search').fill('/home/hive/repo.git')
    await page.getByTestId('new-session-repository-custom').click()
    await page.getByTestId('new-session-name').fill(SESSION)
    await page.getByTestId('new-session-submit').click()
    await expect(page.getByTestId('new-session-dialog')).toBeHidden()

    await findSession(page)
    await expect(sessionRow(page)).toBeVisible({ timeout: 60_000 })
    await expect(sessionRow(page).getByTestId('terminal-session-host')).toHaveAttribute('data-server', 'server-a')
    expect(onServer(`hive session list --json`)).toContain(`"name":"${SESSION}"`)
  })

  test('opens the session as that server\'s terminal', async ({ page }) => {
    const attached = await stubTerminals(page)
    await page.goto('/#/terminal')
    await findSession(page)
    await sessionRow(page).click()

    await expect(page).toHaveURL(new RegExp(`/terminal/${SLUG}$`))
    await expect.poll(() => attached).toContain(SLUG)
  })

  test('deletes the session on the server', async ({ page }) => {
    await stubTerminals(page)
    await page.goto('/#/terminal')
    await findSession(page)
    const row = sessionRow(page)
    await row.hover()
    await row.getByTestId('terminal-session-menu-toggle').click()
    await expect(page.getByTestId('session-menu-start')).toHaveCount(0)
    await page.getByTestId('session-menu-delete').click()
    await page.getByTestId('session-confirmation-confirm').click()

    await expect(row).toBeHidden({ timeout: 60_000 })
    expect(onServer(`hive session list --json`)).not.toContain(`"name":"${SESSION}"`)
  })
})
