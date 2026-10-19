import { chromium, expect } from '@playwright/test'
import { execFileSync } from 'node:child_process'
import assert from 'node:assert/strict'
import { randomUUID } from 'node:crypto'

const ssh = ['-i', '/tmp/remote-key', '-o', 'UserKnownHostsFile=/tmp/known_hosts', '-o', 'StrictHostKeyChecking=yes', '-o', 'IdentitiesOnly=yes', '-o', 'BatchMode=yes']
const remote = command => execFileSync('ssh', [...ssh, 'hive@remote', command], { encoding: 'utf8' }).trim()
remote('codex login status')
const browser = await chromium.launch({ headless: true })
const page = await browser.newPage({ viewport: { width: 1440, height: 1000 } })
page.setDefaultTimeout(30000)
const name = `codex-${randomUUID().slice(0, 8)}`
let session
let pane
const capture = () => remote(`tmux capture-pane -p -S -200 -t '${pane}'`)
const terminalInput = () => page.locator(`[data-testid="remote-terminal"] [data-pane-id="${pane}"] .xterm-helper-textarea`)
const input = async text => {
  await page.locator(`[data-testid="remote-terminal"] [data-pane-id="${pane}"]`).click({ position: { x: 30, y: 30 } })
  await terminalInput().focus()
  await expect(terminalInput()).toBeFocused()
  await terminalInput().pressSequentially(text, { delay: 30 })
  // Wait for the remote terminal, not just the browser's input queue.
  if (text) {
    await expect.poll(() => capture().replace(/\s+/g, ' '), { timeout: 30000 }).toContain(text.slice(-50))
    await page.waitForTimeout(1000)
  }
  await terminalInput().press('Enter')
}
try {
  await page.goto('http://127.0.0.1:8080')
  for (const step of ['hive', 'permissions', 'agent']) await page.getByTestId(`onboarding-${step}-skip`).click()
  await page.evaluate(() => { location.hash = '/settings/remote' })
  await page.getByTestId('remote-name').fill('Codex fixture')
  await page.getByTestId('remote-host').fill('hive-fixture')
  await page.getByTestId('remote-connection-file').fill('/home/hive/connection.json')
  await page.getByTestId('remote-connect').click()
  await page.getByTestId('remote-code-menu').waitFor()
  console.log('Connected to remote fixture')
  await page.getByTestId('terminal-new-session').click()
  await page.getByTestId('new-session-repository').click()
  await page.getByTestId('new-session-repository-option').filter({ hasText: 'Codex fixture' }).filter({ hasText: 'repos/demo' }).first().click()
  await page.getByTestId('new-session-name').fill(name)
  await page.getByTestId('new-session-submit').click()
  await expect(page.getByTestId('new-session-dialog')).toBeHidden({ timeout: 90000 })
  const row = page.getByTestId('remote-code-menu').getByRole('button', { name: new RegExp(name) })
  console.log('Created remote session', name)
  await row.click()
  await row.getByTestId('remote-new-window').click()
  await page.getByTestId('new-window-agent-codex').click()
  console.log('Launched Codex window')
  await page.locator('[data-testid="remote-terminal"][data-status="live"]').waitFor()
  session = remote('hive session list --json').split('\n').map(JSON.parse).find(item => item.name === name)
  assert.ok(session)
  session = JSON.parse(remote(`hive session show --json ${session.id}`))
  assert.ok(session.path)
  pane = remote(`tmux list-panes -a -F '#{session_name} #{pane_id} #{pane_start_command}'`).split('\n').find(line => line.startsWith(`${name} `) && line.includes('codex --sandbox'))?.split(' ')[1]
  assert.ok(pane, 'Hive launched Codex')
  const windowID = remote(`tmux display-message -p -t '${pane}' '#{window_id}'`)
  const windowRow = page.locator(`[data-testid="remote-code-window"][data-window-id="${windowID}"]`)
  await windowRow.click()
  await expect(windowRow).toHaveClass(/window-row-active/)
  await page.locator('[data-testid="remote-terminal"][data-status="live"]').waitFor()
  await expect.poll(capture, { timeout: 90000 }).toMatch(/Trust this folder|Update available/)
  if (capture().includes('Update available')) {
    await page.locator(`[data-testid="remote-terminal"] [data-pane-id="${pane}"]`).click({ position: { x: 30, y: 30 } })
    await terminalInput().press('Escape')
    console.log('Skipped CLI update prompt')
  }
  await expect.poll(capture, { timeout: 90000 }).toContain('Trust this folder')
  console.log('Fresh folder trust prompt:', capture())
  await input('')
  await expect.poll(() => remote(`python3 -c 'import tomllib; c=tomllib.load(open("/home/hive/.codex/config.toml", "rb")); print(c.get("projects", {}).get("${session.path}", {}).get("trust_level", ""))'`), { timeout: 30000 }).toBe('trusted')
  console.log('PASS: interactive folder trust persisted')
  const marker = randomUUID()
  const prompt = `Use a sandboxed shell tool, without escalation, to read README.md and copy its text to codex-read.txt, and write ${marker} to codex-edit.txt. Also run a shell command that attempts to write /home/hive/${name}-outside.txt once; it must fail under workspace-write sandboxing. Write the exit status of that failed command to codex-denied.txt. Do not change permissions or sandbox mode. Then stop.`
  await input(prompt)
  await expect.poll(() => remote(`cat '${session.path}/codex-edit.txt' 2>/dev/null || true`), { timeout: 240000, intervals: [2000] }).toBe(marker)
  await expect.poll(() => remote(`cat '${session.path}/codex-denied.txt' 2>/dev/null || true`), { timeout: 120000, intervals: [2000] }).toMatch(/^[1-9][0-9]*$/)
  assert.equal(remote(`cat '${session.path}/codex-read.txt'`), remote(`cat '${session.path}/README.md'`))
  remote(`test ! -e /home/hive/${name}-outside.txt`)
  console.log('PASS: sandboxed shell read, workspace edit, outside write denied')
  execFileSync('pkill', ['-f', '^ssh .* -W '])
  await page.getByTestId('remote-terminal-disconnected').waitFor()
  await page.getByTestId('remote-server-reconnect').click()
  await page.locator('[data-testid="remote-terminal"][data-status="live"]').waitFor()
  await input(`Use the shell to read codex-edit.txt and copy it to codex-reconnected.txt. Then stop.`)
  await expect.poll(() => remote(`cat '${session.path}/codex-reconnected.txt' 2>/dev/null || true`), { timeout: 240000, intervals: [2000] }).toBe(marker)
  assert.doesNotMatch(capture(), /remote app-server worker channel is closed/)
  console.log('PASS: real Codex shell call after Hive SSH reconnect')
} catch (error) {
  console.error('Page:', await page.locator('body').innerText())
  if (pane) console.error('Codex terminal:', capture())
  throw error
} finally {
  if (session) remote(`hive session delete --force ${session.id}`)
  await browser.close()
}
