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
} as const

export default defineConfig({
  testDir: './tests',
  timeout: 30_000,
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
    { name: 'chromium', use: { browserName: 'chromium', baseURL: `http://127.0.0.1:${feedPorts.chromium}` } },
    { name: 'webkit', use: { browserName: 'webkit', baseURL: `http://127.0.0.1:${feedPorts.webkit}` } },
  ],
})
