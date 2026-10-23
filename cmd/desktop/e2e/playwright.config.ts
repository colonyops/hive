import { defineConfig } from '@playwright/test'
import { dirname } from 'node:path'
import { fileURLToPath } from 'node:url'
import { serverURL } from './fixtures/servers.js'

const here = dirname(fileURLToPath(import.meta.url))

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
  // Set PW_FAIL_ON_FLAKY (forwarded into the container by run-docker.sh) for a
  // strict flake hunt: a test that only passes on retry then fails the run
  // instead of hiding as "flaky".
  failOnFlakyTests: !!process.env.PW_FAIL_ON_FLAKY,
  expect: { timeout: 10_000 },
  outputDir: 'test-results',
  // Each spec picks its server with test.use({ server }); fixtures/test.ts
  // derives baseURL from it.
  use: {
    viewport: { width: 1360, height: 864 },
    // Traces are the only forensic record of a flake: run-docker.sh mounts
    // test-results/ out of the container so they survive the run.
    trace: 'retain-on-failure',
  },
  webServer: {
    command: './scripts/serve.sh',
    cwd: here,
    // serve.sh starts the chromium feed server last, so it doubles as the
    // readiness sentinel for every server.
    url: serverURL('feed', 'chromium'),
    timeout: 180_000,
    // Never attach to an already-running local server: doing so can reuse a
    // developer or previous pipeline DB and make fixture state order-dependent.
    reuseExistingServer: false,
  },
  projects: [
    {
      name: 'chromium',
      testIgnore: ['**/source-to-commit.spec.ts', '**/actions.spec.ts'],
      use: { browserName: 'chromium' },
    },
    {
      name: 'webkit',
      testIgnore: ['**/source-to-commit.spec.ts', '**/actions.spec.ts'],
      use: { browserName: 'webkit' },
    },
    {
      name: 'pipeline-smoke',
      testMatch: '**/source-to-commit.spec.ts',
      use: { browserName: 'chromium' },
    },
    {
      name: 'action-smoke',
      testMatch: '**/actions.spec.ts',
      use: { browserName: 'chromium' },
    },
  ],
})
