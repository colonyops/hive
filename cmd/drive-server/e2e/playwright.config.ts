import { defineConfig } from '@playwright/test'
import { dirname } from 'node:path'
import { fileURLToPath } from 'node:url'
import { canaryPort, runDir, serverPort } from './tests/env'

const here = dirname(fileURLToPath(import.meta.url))

export default defineConfig({
  testDir: './tests',
  globalSetup: './tests/global-setup.ts',
  timeout: 60_000,
  // The projects share one server and one canary, and the canary assertions
  // are only meaningful when one test at a time touches it.
  workers: 1,
  fullyParallel: false,
  // A retry would sign in again and spend the per-email sign-in limit, and a
  // security check that passes on retry is a failure.
  retries: 0,
  expect: { timeout: 10_000 },
  outputDir: 'test-results',
  use: {
    baseURL: `http://127.0.0.1:${serverPort}`,
    viewport: { width: 1280, height: 800 },
    trace: 'retain-on-failure',
  },
  webServer: [
    {
      command: 'node canary.mjs',
      cwd: here,
      url: `http://127.0.0.1:${canaryPort}/__hits`,
      env: { DRIVE_E2E_CANARY_PORT: String(canaryPort) },
      reuseExistingServer: false,
    },
    {
      command: './scripts/serve.sh',
      cwd: here,
      url: `http://127.0.0.1:${serverPort}/healthz`,
      timeout: 60_000,
      env: { DRIVE_E2E_RUN_DIR: runDir, DRIVE_E2E_PORT: String(serverPort) },
      reuseExistingServer: false,
    },
  ],
  projects: [
    { name: 'chromium', use: { browserName: 'chromium' } },
    { name: 'firefox', use: { browserName: 'firefox' } },
    { name: 'webkit', use: { browserName: 'webkit' } },
  ],
})
