import { request as apiRequest, test as base, expect } from '@playwright/test'
import { serverURL, type ServerName } from './servers.js'
import { tag, tagProblems, type Kind } from './tags.js'

// A retry gets a fresh worker and browser but the same servers, so durable
// rows and config edits from one attempt would leak into the next. Every
// mock-mode server mounts POST /_e2e/reset (desktop/state_reset.go), which
// restores it to its post-startup baseline. Fixture setup runs before
// beforeEach hooks and the test body, so the reset lands before page.goto.
//
// Reset modes:
//
//  - per-test (default): reset before every test.
//  - per-file: for serial journeys where later tests read rows earlier tests
//    wrote. Call resetServerState() once from beforeAll; a retry or a
//    --repeat-each pass runs in a fresh worker, so beforeAll re-runs.
//  - off: for onboarding. Its mock device-flow grant is in-memory, one-way
//    state that /_e2e/reset does not cover, so no reset can return the server
//    to the signed-out baseline. The journey also keeps retries: 0.

export type ServerStateResetMode = 'per-test' | 'per-file' | 'off'

export async function resetServerState(baseURL: string | undefined): Promise<void> {
  if (!baseURL) throw new Error('resetServerState: no baseURL to reset')
  const url = new URL('/_e2e/reset', baseURL).toString()
  const context = await apiRequest.newContext()
  try {
    const response = await context.post(url)
    if (response.status() !== 204) {
      throw new Error(`backend state reset failed: POST ${url} -> ${response.status()}: ${await response.text()}`)
    }
  } finally {
    await context.dispose()
  }
}

type Options = {
  server: ServerName
  serverStateReset: ServerStateResetMode
}

type InternalFixtures = {
  autoResetServerState: void
  enforceTags: void
}

const kinds: Kind[] = ['smoke', 'workflows', 'features']

function kindOf(projectName: string): Kind {
  const kind = kinds.find((k) => projectName.startsWith(`${k}-`))
  if (!kind) throw new Error(`project '${projectName}' does not start with a test kind (${kinds.join(', ')})`)
  return kind
}

export const test = base.extend<Options & InternalFixtures>({
  server: ['feed', { option: true }],
  serverStateReset: ['per-test', { option: true }],
  baseURL: async ({ server, browserName }, use) => {
    await use(serverURL(server, browserName))
  },
  autoResetServerState: [
    async ({ serverStateReset, baseURL }, use) => {
      if (serverStateReset === 'per-test') await resetServerState(baseURL)
      await use()
    },
    { auto: true },
  ],
  enforceTags: [
    async ({}, use, testInfo) => {
      const problems = tagProblems(testInfo.tags, kindOf(testInfo.project.name))
      if (problems.length > 0) {
        throw new Error(`tags on '${testInfo.title}' break the rules in fixtures/tags.ts:\n- ${problems.join('\n- ')}`)
      }
      await use()
    },
    { auto: true },
  ],
})

export { expect, tag }
