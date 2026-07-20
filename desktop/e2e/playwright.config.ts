import { defineConfig } from '@playwright/test'
import { dirname } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))

// scripts/serve.sh starts one feed-mode server per browser project so tests
// never share read/action state across concurrently running projects. The
// chromium URL is also Playwright's readiness sentinel; the script starts it
// last after the other mock servers are ready.
const feedPorts = {
  chromium: 8931,
  webkit: 8934,
  pipelineSmoke: 8935,
  actionSmoke: 8936,
} as const

export default defineConfig({
  testDir: './tests',
  timeout: 30_000,
  // Some fixture tests mutate a server-local flow through a watcher. Keep the
  // Docker acceptance suite ordered so a slow WebKit watcher cannot race a
  // concurrent project while it is asserting its own isolated state.
  workers: 1,
  // Real browsers + six concurrent Go servers on a shared CI runner make some
  // timing jitter irreducible (a slow server response, a GC pause). Retry in
  // CI so that jitter does not red a build; a genuine regression still fails
  // every attempt. Local runs get 0 so flakes surface loudly. The onboarding
  // suite opts out (retries: 0) — its device-flow grant is a one-way server
  // state change a retry cannot replay. Set via env so `mise run desktop:e2e`
  // (CI=1 in the image) and ad-hoc local runs differ automatically.
  retries: process.env.CI ? 2 : 0,
  expect: { timeout: 10_000 },
  outputDir: 'test-results',
  use: {
    baseURL: `http://127.0.0.1:${feedPorts.chromium}`,
    viewport: { width: 1360, height: 864 },
  },
  webServer: {
    command: './scripts/serve.sh',
    cwd: here,
    url: `http://127.0.0.1:${feedPorts.chromium}`,
    timeout: 180_000,
    // Never attach to an already-running local server: doing so can reuse a
    // developer or previous pipeline DB and make fixture state order-dependent.
    reuseExistingServer: false,
  },
  projects: [
    {
      name: 'chromium',
      testIgnore: ['**/source-to-commit.spec.ts', '**/actions.spec.ts'],
      use: { browserName: 'chromium', baseURL: `http://127.0.0.1:${feedPorts.chromium}` },
    },
    {
      name: 'webkit',
      testIgnore: ['**/source-to-commit.spec.ts', '**/actions.spec.ts'],
      use: { browserName: 'webkit', baseURL: `http://127.0.0.1:${feedPorts.webkit}` },
    },
    {
      name: 'pipeline-smoke',
      testMatch: '**/source-to-commit.spec.ts',
      use: { browserName: 'chromium', baseURL: `http://127.0.0.1:${feedPorts.pipelineSmoke}` },
    },
    {
      name: 'action-smoke',
      testMatch: '**/actions.spec.ts',
      use: { browserName: 'chromium', baseURL: `http://127.0.0.1:${feedPorts.actionSmoke}` },
    },
  ],
})
