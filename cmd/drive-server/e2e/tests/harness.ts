import { request } from '@playwright/test'
import { readFileSync, writeFileSync } from 'node:fs'
import { get } from 'node:http'
import { canaryOrigin, runDir, serverOrigin } from './env'
import { documents } from './hostile'

export interface Seeded {
  storageState: Awaited<ReturnType<Awaited<ReturnType<typeof request.newContext>>['storageState']>>
  slugs: Record<string, string>
}

const projects = ['chromium', 'firefox', 'webkit']
const seedFile = `${runDir}/seed.json`

export async function seedAll(): Promise<void> {
  const all: Record<string, Seeded> = {}
  for (const project of projects) all[project] = await doSeed(`${project}@example.com`)
  writeFileSync(seedFile, JSON.stringify(all))
}

/** Reads what globalSetup seeded; a worker restart after a failure keeps it. */
export function seed(project: string): Promise<Seeded> {
  const all = JSON.parse(readFileSync(seedFile, 'utf8')) as Record<string, Seeded>
  return Promise.resolve(all[project])
}

async function doSeed(email: string): Promise<Seeded> {
  const api = await request.newContext({ baseURL: serverOrigin })
  const started = await api.post('/api/v1/auth/code', { data: { email } })
  if (!started.ok()) throw new Error(`auth/code: ${started.status()}`)
  const code = await codeFromMailLog(email)
  const verified = await api.post('/api/v1/auth/verify', { data: { email, code } })
  if (!verified.ok()) throw new Error(`auth/verify: ${verified.status()}`)

  const slugs: Record<string, string> = {}
  for (const [name, html] of Object.entries(documents)) {
    const put = await api.put(`/api/v1/shared/docs/${name}`, { data: html, headers: { 'content-type': 'text/html' } })
    if (!put.ok()) throw new Error(`put ${name}: ${put.status()} ${await put.text()}`)
    const share = await api.post(`/api/v1/shared/shares/${name}`, { data: {} })
    if (!share.ok()) throw new Error(`share ${name}: ${share.status()} ${await share.text()}`)
    const { url } = (await share.json()) as { url: string }
    slugs[name] = new URL(url).pathname.split('/').filter(Boolean).pop()!
  }
  const storageState = await api.storageState()
  await api.dispose()
  return { storageState, slugs }
}

async function codeFromMailLog(email: string): Promise<string> {
  const deadline = Date.now() + 15_000
  while (Date.now() < deadline) {
    const log = readFileSync(`${runDir}/server.log`, 'utf8')
    const matches = [...log.matchAll(new RegExp(`mail to ${email}:[\\s\\S]*?\\n\\s+(\\d{8})\\n`, 'g'))]
    if (matches.length > 0) return matches[matches.length - 1][1]
    await new Promise((r) => setTimeout(r, 200))
  }
  throw new Error(`no sign-in code for ${email} in the server log`)
}

export interface CanaryReport {
  hits: { kind: string; method?: string; url?: string }[]
  strayConnections: number
}

// agent:false opens a fresh connection per call; the canary subtracts them.
function control(path: string): Promise<CanaryReport> {
  return new Promise((resolve, reject) => {
    get(`${canaryOrigin}${path}`, { agent: false }, (res) => {
      let body = ''
      res.on('data', (chunk) => (body += chunk))
      res.on('end', () => resolve(JSON.parse(body) as CanaryReport))
    }).on('error', reject)
  })
}

export const resetCanary = () => control('/__reset')
export const canaryReport = () => control('/__hits')
