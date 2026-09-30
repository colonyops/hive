import { request as apiRequest, test as base, expect } from '@playwright/test'

// Shared test base wiring per-test backend state isolation.
//
// Playwright's retry model gives a failed test a fresh worker and browser but
// the same servers: the mock instances live for the whole session, so durable
// SQLite rows and config-file edits written by one attempt leak into the next
// ("retry poisoning"), and burn-in runs (--repeat-each) re-observe earlier
// passes' rows. Every mock-mode server mounts POST /_e2e/reset
// (desktop/state_reset.go): it restores that server instance to its
// post-startup baseline — pipeline database wiped and reseeded in one
// transaction, hive.db action tables (sessions, messages, message_reads)
// cleared, pristine actions.yml/settings.yaml/flows files rewritten — and
// answers 204. Mounting is gated server-side on mock mode plus the Docker
// harness marker, so the call needs no request auth.
//
// Contract: the automatic fixture below POSTs that reset against the
// project's baseURL before every test. Fixture setup precedes beforeEach
// hooks and the test body, so the reset always lands before the test's
// page.goto — no loaded frontend can hold a checkpoint that outruns the
// truncated event log. A failed reset throws (including the endpoint's
// response body) and fails the test loudly instead of letting it run against
// dirty state.
//
// Opt-outs:
//
//  - Serial-journey files (actions.spec.ts), where later tests intentionally
//    read durable rows earlier tests created, disable the per-test reset with
//    `test.use({ serverStateReset: 'per-file' })` and instead call
//    resetServerState() once from test.beforeAll. A serial-group retry — and
//    each --repeat-each pass — runs in a fresh worker where beforeAll re-runs
//    (the worker hash includes the repeat index), so every execution of the
//    file starts from the server baseline while the intra-file state chain
//    stays intact. That is why such files no longer need a `retries: 0`
//    override.
//
//  - onboarding.spec.ts stays on plain @playwright/test and is never reset.
//    Its dedicated per-browser onboarding servers (ports 8932/8933) hinge on
//    the mock device-flow grant, an in-memory, one-way state change that
//    /_e2e/reset does not cover — no reset could return those servers to the
//    signed-out baseline the journey starts from. For the same reason the
//    journey keeps its `retries: 0` opt-out: a retry would meet an
//    already-connected server and could not replay the pre-connect cards.

export type ServerStateResetMode = 'per-test' | 'per-file'

// resetServerState POSTs /_e2e/reset on the server behind baseURL and throws
// (with the response body) unless the server answers 204.
export async function resetServerState(baseURL: string | undefined): Promise<void> {
  if (!baseURL) throw new Error('resetServerState: the project has no baseURL to reset')
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

interface ServerStateResetFixtures {
  serverStateReset: ServerStateResetMode
  autoResetServerState: void
}

export const test = base.extend<ServerStateResetFixtures>({
  serverStateReset: ['per-test', { option: true }],
  autoResetServerState: [
    async ({ serverStateReset, baseURL }, use) => {
      if (serverStateReset === 'per-test') await resetServerState(baseURL)
      await use()
    },
    { auto: true },
  ],
})

export { expect }
