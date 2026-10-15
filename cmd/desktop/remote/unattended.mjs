import { chromium, expect } from '@playwright/test'
import { execFileSync, spawn } from 'node:child_process'
import { readFileSync, openSync, closeSync } from 'node:fs'
import { randomUUID } from 'node:crypto'
import assert from 'node:assert/strict'
import { setTimeout as delay } from 'node:timers/promises'

assert.ok(process.env.HIVE_SMOKE_APP_PID, 'run through the Docker smoke harness')
const run = randomUUID()
const name = `unattended-${run}`
const root = `/home/hive/${name}`
const config = '/home/hive/.config/hive/desktop'
const remote = (command, input) => execFileSync('ssh', ['hive-fixture', command], {
  input, encoding: 'utf8', timeout: 20000,
})
function mcp(tool, args = {}) {
  const reply = JSON.parse(remote("curl -fsS http://127.0.0.1:19001/mcp -H 'Content-Type: application/json' -H 'Accept: application/json, text/event-stream' --data-binary @-",
    JSON.stringify({ jsonrpc: '2.0', id: 1, method: 'tools/call', params: { name: tool, arguments: args } })))
  assert.ok(!reply.error && !reply.result.isError, JSON.stringify(reply))
  return reply.result.structuredContent
}
function write(path, body) {
  remote(`cat > '${path}'`, body)
}
async function until(fn, message, timeout = 90000) {
  await expect.poll(fn, { message, timeout, intervals: [1000] }).toBe(true)
}
let browser
let local
let scheduleCreated = false
let workspaceCreated = false
try {
  remote(`mkdir -p '${root}' '${config}/workspaces/${name}'`)
  write(`${root}/agent.py`, readFileSync('/smoke/unattended.py'))
  write(`${config}/workspaces/${name}/agent-workspace.yaml`, JSON.stringify({
    version: 5, name: `Unattended ${run}`, command: `python3 ${root}/agent.py {{ .Prompt | shq }}`,
  }))
  workspaceCreated = true
  write(`${config}/flows/${name}.yaml`, JSON.stringify({
    version: 1, name: `Unattended ${run}`,
    nodes: [
      { id: 'incoming', type: 'sources.webhook', path: `${name}/incoming` },
      { id: 'results', type: 'sources.webhook', path: `${name}/results` },
      { id: 'active', type: 'function', on_message: 'return msg.Payload.state === "done" ? null : msg;' },
      { id: 'worker', type: 'launch-chat', workspace: name, prompt: `unattended:${run}:notification` },
      { id: 'inbox', type: 'feed', name: 'Unattended results' },
    ],
    wires: [
      { from: 'incoming', to: 'active' }, { from: 'active', to: 'worker' },
      { from: 'incoming', to: 'inbox' }, { from: 'results', to: 'inbox' },
    ],
  }))
  await until(() => mcp('list_workspaces').workspaces.some(w => w.dir === name && !w.problem), 'workspace loaded')
  await until(() => mcp('list_profiles').profiles.some(p => p.id === name && p.valid), 'flow loaded')

  browser = await chromium.launch({ headless: true })
  let page = await browser.newPage()
  await page.goto('http://127.0.0.1:8080')
  for (const step of ['hive', 'permissions', 'agent']) await page.getByTestId(`onboarding-${step}-skip`).click()
  await page.evaluate(() => { location.hash = '/settings/remote' })
  await page.getByTestId('remote-name').fill('Unattended Docker')
  await page.getByTestId('remote-host').fill('hive-fixture')
  await page.getByTestId('remote-connection-file').fill('/home/hive/connection.json')
  await page.getByTestId('remote-connect').click()
  await expect(page.getByTestId('remote-code-menu')).toBeVisible()
  await page.getByTestId('titlebar-mode-hub').click()
  await page.getByTestId(`remote-inbox-item-${name}`).click()
  await expect(page.getByTestId('remote-inbox-item')).toHaveCount(0)
  await page.evaluate(() => { location.hash = '/settings/remote' })
  await page.getByTestId('remote-disconnect').click()
  await browser.close()
  browser = undefined
  const pid = Number(process.env.HIVE_SMOKE_APP_PID)
  process.kill(pid, 'SIGTERM')
  await until(() => {
    try { process.kill(pid, 0); return false } catch (error) {
      if (error.code === 'ESRCH') return true
      throw error
    }
  }, 'local Desktop process stopped', 15000)
  const disconnectedAt = Number(remote('date +%s'))
  console.log('Local Desktop and browser stopped; remote background test begins')

  mcp('put_schedule', { workspace: name, id: 'unattended', cron: '@every 30s', prompt: `unattended:${run}:scheduled` })
  scheduleCreated = true
  remote(`curl -fsS http://127.0.0.1:19001/hooks/${name}/incoming -H 'Content-Type: application/json' --data-binary @-`,
    JSON.stringify({ id: run, title: `Incoming ${run}` }))
  await until(() => remote(`test -f '${root}/scheduled.json' && test -f '${root}/notification.json' && echo ready || true`).trim() === 'ready', 'both remote workers completed')
  mcp('put_schedule', { workspace: name, id: 'unattended', disabled: true })
  await until(() => mcp('schedule_runs', { workspace: name, id: 'unattended' }).runs.length > 0, 'schedule launch recorded')
  const runs = mcp('schedule_runs', { workspace: name, id: 'unattended' }).runs
  assert.equal(runs.length, 1, 'one real scheduled launch')
  assert.equal(runs[0].status, 'launched')
  assert.equal(runs[0].reason, 'due')
  assert.ok(Date.parse(runs[0].startedAt) >= disconnectedAt * 1000)
  for (const mode of ['scheduled', 'notification']) {
    const receipt = JSON.parse(remote(`cat '${root}/${mode}.json'`))
    assert.equal(receipt.run, run)
    assert.ok(receipt.completedAt >= disconnectedAt)
  }
  await until(() => mcp('list_inbox', { profile: name, externalId: run }).items.some(i => i.title === `Incoming ${run}` && i.sourceState === 'done' && i.archivedAt > 0), 'notification processed and archived')
  await until(() => mcp('list_inbox', { profile: name, feed: name + '/inbox' }).items.length === 2, 'both result items persisted before reconnect')
  console.log('PASS: scheduled job and webhook-triggered inbox worker completed with no local Desktop')

  const log = openSync('/tmp/unattended-local-restart.log', 'w')
  local = spawn('hive-desktop', [], { stdio: ['ignore', log, log] })
  closeSync(log)
  await until(async () => {
    try { return (await fetch('http://127.0.0.1:8080')).ok } catch { return false }
  }, 'local Desktop restarted')
  browser = await chromium.launch({ headless: true })
  page = await browser.newPage()
  await page.goto('http://127.0.0.1:8080')
  await page.evaluate(() => { location.hash = '/settings/remote' })
  await page.getByTestId('remote-name').fill('Unattended Docker')
  await page.getByTestId('remote-host').fill('hive-fixture')
  await page.getByTestId('remote-connection-file').fill('/home/hive/connection.json')
  await page.getByTestId('remote-connect').click()
  await expect(page.getByTestId('remote-code-menu')).toBeVisible()
  await page.getByTestId('titlebar-mode-hub').click()
  await page.getByTestId(`remote-inbox-item-${name}`).click()
  for (const title of [`Scheduled job ${run}`, `Processed notification ${run}`]) {
    await expect(page.getByTestId('remote-inbox-item').filter({ hasText: title })).toHaveCount(1)
  }
  console.log('PASS: restarted Desktop reconnected through SSH/TLS and displayed both remote results')
} finally {
  await browser?.close()
  if (local) {
    const exited = new Promise(resolve => local.once('exit', resolve))
    local.kill('SIGTERM')
    await Promise.race([exited, delay(5000)])
    if (local.exitCode === null && local.signalCode === null) local.kill('SIGKILL')
  }
  if (scheduleCreated) mcp('remove_schedule', { workspace: name, id: 'unattended' })
  if (workspaceCreated) {
    // Delete only this run's workspace through the authenticated lifecycle API.
    remote('python3 -', `import json,ssl,urllib.request\nc=json.load(open('/home/hive/connection.json'))\nr=urllib.request.Request(c['url']+'/api/terminal/agents/workspaces/delete',json.dumps({'dir':'${name}'}).encode(),headers={'Authorization':'Bearer '+c['token'],'Content-Type':'application/json'})\nwith urllib.request.urlopen(r,context=ssl.create_default_context(cadata=c['certificate']),timeout=15) as response: assert response.status==204\n`)
  }
  mcp('delete_profile', { profileId: name })
  remote(`rm -rf '${root}'`)
}
