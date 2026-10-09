import { chromium, expect } from '@playwright/test'
import { readFileSync } from 'node:fs'
import { execFileSync, spawn } from 'node:child_process'
import assert from 'node:assert/strict'
import { randomUUID } from 'node:crypto'

const { token } = JSON.parse(readFileSync('/tmp/connection.json', 'utf8'))
const browser = await chromium.launch({ headless: true })
const page = await browser.newPage({ viewport: { width: 1280, height: 850 } })
const ssh = ['-i', '/tmp/remote-key', '-o', 'UserKnownHostsFile=/tmp/known_hosts', '-o', 'StrictHostKeyChecking=yes', '-o', 'IdentitiesOnly=yes', '-o', 'BatchMode=yes']
let replacement
try {
  await page.goto('http://127.0.0.1:8080')
  await page.getByTestId('onboarding-hive-skip').click()
  await page.getByTestId('onboarding-permissions-skip').click()
  await page.getByTestId('onboarding-agent-skip').click()
  await page.getByTestId('titlebar-mode-terminal').click()
  await page.getByTestId('terminal-unavailable').waitFor()
  await page.getByTestId('code-installation').getByRole('button', { name: 'Remote (preview)' }).click()
  await page.getByTestId('remote-token').fill('wrong-token')
  await page.getByTestId('remote-connect').click()
  await page.getByTestId('remote-error').filter({ hasText: 'rejected' }).waitFor()
  await page.getByTestId('remote-token').fill(token)
  await page.getByTestId('remote-connect').click()
  await page.getByTestId('remote-session-remote-demo').click()
  await page.getByTestId('remote-terminal-status').filter({ hasText: 'live' }).waitFor()
  const input = page.locator('[data-testid="remote-terminal"] .xterm-helper-textarea').first()
  await input.focus()
  const marker = randomUUID()
  await page.keyboard.type(`printf '${marker}\\n' > /home/hive/smoke-result`)
  await page.keyboard.press('Enter')
  await expect.poll(() => execFileSync('ssh', [...ssh, 'hive@remote', 'cat /home/hive/smoke-result'], { encoding: 'utf8' }).trim()).toBe(marker)
  const terminalWidth = () => execFileSync('ssh', [...ssh, 'hive@remote', "tmux display-message -p -t =remote-demo: '#{window_width}'"], { encoding: 'utf8' }).trim()
  const previousWidth = terminalWidth()
  await page.setViewportSize({ width: 1000, height: 700 })
  await expect.poll(terminalWidth).not.toBe(previousWidth)
  const windows = await page.evaluate(async (token) => {
    const response = await fetch('http://127.0.0.1:19001/api/terminal/windows/list', { method: 'POST', headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' }, body: JSON.stringify({ slugs: ['remote-demo'] }) })
    return response.json()
  }, token)
  assert.ok(JSON.stringify(windows).includes('second'), 'multiple remote windows are listed')
  await page.getByTestId('remote-terminal').getByRole('button', { name: 'second', exact: true }).click()
  process.kill(Number(process.env.REMOTE_TUNNEL_PID), 'SIGTERM')
  await page.getByTestId('remote-terminal-status').filter({ hasText: 'ended' }).waitFor()
  replacement = spawn('ssh', [...ssh, '-N', '-T', '-o', 'ExitOnForwardFailure=yes', '-L', '127.0.0.1:19001:127.0.0.1:19001', 'hive@remote'])
  await expect.poll(async () => {
    try { return (await fetch('http://127.0.0.1:19001/api/status')).ok } catch { return false }
  }).toBe(true)
  await page.getByTestId('remote-reconnect').click()
  await page.getByTestId('remote-terminal-status').filter({ hasText: 'live' }).waitFor()
  await page.getByTestId('remote-disconnect').click()
  execFileSync('ssh', [...ssh, 'hive@remote', 'tmux has-session -t =remote-demo'])
  console.log('PASS: browser discovery, auth, remote terminal input, windows, resize, tunnel loss/reconnect and detach survival')
} catch (error) {
  console.error('Page:', await page.locator('body').innerText())
  throw error
} finally {
  replacement?.kill()
  await browser.close()
}
