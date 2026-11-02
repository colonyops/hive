import { defineConfig } from '@playwright/test'
import { availableParallelism } from 'node:os'
import { dirname } from 'node:path'
import { fileURLToPath } from 'node:url'
import { serverURL } from './fixtures/servers.js'

const here = dirname(fileURLToPath(import.meta.url))
const chromium = { browserName: 'chromium' as const }
const webkit = { browserName: 'webkit' as const }

// Each worker adds a browser and two Go servers, and past this the
// container spends its time on timing jitter rather than on tests.
const MAX_DEFAULT_WORKERS = 6

// The remote lane has one spec, and its fixture holds one set of sessions.
const workers =
  Number(process.env.HIVE_E2E_WORKERS) ||
  (process.env.HIVE_DESKTOP_E2E_REMOTE ? 1 : Math.min(MAX_DEFAULT_WORKERS, Math.max(1, availableParallelism() - 1)))

export default defineConfig({
  timeout: 30_000,
  // A worker has its own feed servers (fixtures/servers.ts), so tests that mutate
  // a flow through a watcher cannot race another worker's state.
  workers,
  // Browsers and seven Go servers in one container leave some timing jitter;
  // a real regression still fails every attempt. The image sets CI=1.
  retries: process.env.CI ? 1 : 0,
  // Tests run one at a time, so a broken helper would otherwise cost every
  // test that calls it a full timeout, on each attempt and in each browser.
  maxFailures: 5,
  // A green run hides a test that passed only on retry, so annotate those too.
  reporter: process.env.GITHUB_ACTIONS ? [['dot'], ['github']] : 'dot',
  failOnFlakyTests: !!process.env.PW_FAIL_ON_FLAKY,
  expect: { timeout: 10_000 },
  outputDir: 'test-results',
  // No baseURL: fixtures/test.ts derives it from the spec's server.
  use: {
    viewport: { width: 1360, height: 864 },
    actionTimeout: 10_000,
    // run-docker.sh mounts test-results/ out of the container.
    // Recording every first attempt costs a CI runner too much CPU.
    trace: 'on-first-retry',
  },
  webServer: {
    command: './scripts/serve.sh',
    cwd: here,
    env: { HIVE_E2E_WORKERS: String(workers) },
    // serve.sh starts the chromium feed server last, so it doubles as the
    // readiness sentinel for every server.
    url: serverURL('feed', 'chromium'),
    timeout: 180_000,
    // Never attach to an already-running local server: doing so can reuse a
    // developer or previous pipeline DB and make fixture state order-dependent.
    reuseExistingServer: false,
  },
  projects: [
    { name: 'smoke-chromium', testDir: './tests/smoke', use: chromium },
    { name: 'smoke-webkit', testDir: './tests/smoke', use: webkit },
    { name: 'features-chromium', testDir: './tests/features', use: chromium },
    // The surfaces where WebKit's pointer, selection and rendering behavior
    // differs. The app ships on WKWebView, and smoke-webkit covers the rest.
    { name: 'features-webkit', testDir: './tests/features', testMatch: '**/{terminal,drive,flows}/*.spec.ts', use: webkit },
    { name: 'workflows-chromium', testDir: './tests/workflows', testIgnore: '**/remote/**', use: chromium },
    // Only onboarding has a webkit server (scripts/serve.sh).
    { name: 'workflows-onboarding-webkit', testDir: './tests/workflows/onboarding', use: webkit },
    // Only the remote lane starts the server these run against, and only it
    // can reach the ssh fixture (scripts/run-remote.sh).
    ...(process.env.HIVE_DESKTOP_E2E_REMOTE
      ? [{ name: 'workflows-remote-chromium', testDir: './tests/workflows/remote', use: chromium }]
      : []),
  ],
})
