import { expect, test, type Browser, type BrowserContext, type Page } from '@playwright/test'
import { canaryOrigin, serverOrigin } from './env'
import { canaryReport, resetCanary, seed, type Seeded } from './harness'
import { mainDocument, navigationDocuments, secretPattern } from './hostile'

type Outcome = { outcome: 'blocked' | 'escaped' | 'unobservable' | 'info'; detail?: string }
type Results = Record<string, Outcome>

const parentSecrets = {
  cookie: 'PARENT_COOKIE_SECRET',
  storage: 'PARENT_STORAGE_SECRET',
  dom: 'PARENT_DOM_SECRET',
}

// Only the embedding page holds these. The document must not read any of them.
async function plantParentSecrets(context: BrowserContext): Promise<void> {
  await context.addCookies([{ name: 'parent_marker', value: parentSecrets.cookie, url: serverOrigin }])
  await context.addInitScript((secrets) => {
    // The embedding page only: the sandboxed document must not carry its own copy.
    if (window !== window.top || window.origin === 'null') return
    try {
      localStorage.setItem('parent_secret', secrets.storage)
    } catch {
      // The raw route opens top level in an opaque origin, which has no storage.
    }
    document.addEventListener('DOMContentLoaded', () => {
      document.documentElement.dataset.parentSecret = secrets.dom
    })
  }, parentSecrets)
}

async function newContext(browser: Browser, signedIn: boolean, storageState: Seeded['storageState']) {
  const context = await browser.newContext(signedIn ? { storageState } : {})
  await plantParentSecrets(context)
  return context
}

interface Observed {
  requests: { url: string; failed: boolean; responded: boolean }[]
}

function observe(page: Page): Observed {
  const seen: Observed = { requests: [] }
  const find = (url: string) => seen.requests.findLast((r) => r.url === url)
  page.on('request', (request) => seen.requests.push({ url: request.url(), failed: false, responded: false }))
  page.on('requestfailed', (request) => {
    const entry = find(request.url())
    if (entry) entry.failed = true
  })
  page.on('response', (response) => {
    const entry = find(response.url())
    if (entry) entry.responded = true
  })
  return seen
}

const localSchemes = ['data:', 'blob:', 'about:']

// Chromium reports a request that CSP blocks as a request followed by a
// failure, so "left the server origin" means it got a response.
function expectNoRequestOutsideServer(seen: Observed): void {
  const outside = seen.requests.filter(
    ({ url }) => !localSchemes.some((s) => url.startsWith(s)) && new URL(url).origin !== serverOrigin,
  )
  expect(
    outside.filter((r) => r.responded || !r.failed),
    'requests outside the server origin that were not blocked',
  ).toEqual([])
}

async function expectCanaryQuiet(): Promise<void> {
  const report = await canaryReport()
  expect(report.hits, 'canary hits').toEqual([])
  expect(report.strayConnections, 'canary connections').toBe(0)
}

function expectNothingEscaped(results: Results): void {
  const escaped = Object.entries(results).filter(([, r]) => r.outcome === 'escaped')
  expect(escaped, 'attacks that succeeded').toEqual([])
  expect(JSON.stringify(results)).not.toMatch(secretPattern)
}

async function readResults(frameOrPage: Page | ReturnType<Page['frameLocator']>): Promise<Results> {
  const body = frameOrPage.locator('body[data-done="1"]')
  await expect(body).toBeAttached({ timeout: 20_000 })
  return JSON.parse((await frameOrPage.locator('#results').textContent()) ?? '{}') as Results
}

const attackNames = [
  'fetch',
  'fetch-server-origin',
  'xhr',
  'websocket',
  'sendBeacon',
  'img-element',
  'img-markup',
  'css-stylesheet-link',
  'css-url-inline',
  'script-src',
  'iframe-src',
  'window.open',
  'own-document.cookie',
  'own-localStorage',
  'parent.document.cookie',
  'parent.document.dom',
  'parent.localStorage',
  'parent.location',
  'top.document',
]

test.beforeEach(async () => {
  await resetCanary()
})

for (const view of ['share', 'signed-in'] as const) {
  test.describe(`framed ${view} view`, () => {
    let context: BrowserContext
    let page: Page
    let seen: Observed
    let target: string

    test.beforeEach(async ({ browser }, info) => {
      const seeded = await seed(info.project.name)
      context = await newContext(browser, view === 'signed-in', seeded.storageState)
      page = await context.newPage()
      seen = observe(page)
      target = view === 'share' ? `/s/${seeded.slugs[mainDocument]}` : `/shared/${mainDocument}`
    })
    test.afterEach(async () => context.close())

    test('the hostile document gets nothing out and reads nothing in', async () => {
      await page.goto(target)
      const frame = page.frameLocator('iframe[title="HTML document"]')
      const results = await readResults(frame)

      for (const name of attackNames) expect(results[name], `${name} ran`).toBeDefined()
      expectNothingEscaped(results)
      expect(results.__origin.detail, 'the frame has an opaque origin').toBe('null')
      await expectCanaryQuiet()
      expectNoRequestOutsideServer(seen)
      expect(page.url()).toBe(`${serverOrigin}${target}`)
      await expect(page.locator('html')).toHaveAttribute('data-parent-secret', parentSecrets.dom)
      expect(await page.evaluate(() => document.cookie)).toContain(parentSecrets.cookie)
    })

    for (const name of navigationDocuments) {
      test(`${name} cannot navigate the frame or the page away`, async ({}, info) => {
        const seeded = await seed(info.project.name)
        const slug = seeded.slugs[name]
        const url = view === 'share' ? `/s/${slug}` : `/shared/${name}`
        await page.goto(url)
        await expect(page.locator('iframe[title="HTML document"]')).toBeAttached()
        await page.waitForLoadState('load')
        await page.waitForTimeout(3_000)

        await expectCanaryQuiet()
        expectNoRequestOutsideServer(seen)
        expect(page.url()).toBe(`${serverOrigin}${url}`)
        const frames = page.frames().map((f) => f.url())
        expect(frames.filter((u) => u.startsWith(canaryOrigin))).toEqual([])
      })
    }
  })
}

for (const route of ['share-raw', 'signed-in-raw'] as const) {
  test.describe(`top-level ${route}`, () => {
    let context: BrowserContext
    let page: Page
    let seen: Observed

    test.beforeEach(async ({ browser }, info) => {
      const seeded = await seed(info.project.name)
      // Signed in even for the share route: the strictest case is one where
      // the browser holds a session cookie it must not hand to the document.
      context = await newContext(browser, true, seeded.storageState)
      page = await context.newPage()
      seen = observe(page)
    })
    test.afterEach(async () => context.close())

    const urlFor = async (name: string, project: string) => {
      const seeded = await seed(project)
      return route === 'share-raw'
        ? `/s/${seeded.slugs[name]}/raw`
        : `/api/v1/shared/docs/${name}?raw=1`
    }

    // Opens the real response in a top-level page by answering the browser's
    // own request with it. The Sec-Fetch-Dest refusal never sees that request,
    // so this isolates what the CSP sandbox alone does.
    async function openWithoutRefusal(url: string): Promise<void> {
      const real = await page.request.get(url)
      expect(real.status()).toBe(200)
      expect(real.headers()['content-security-policy']).toContain('sandbox allow-scripts')
      await page.route(`**${url}`, (r) => r.fulfill({ response: real }))
      await page.goto(url)
    }

    test('the CSP sandbox gives an opaque origin, no cookies and no network', async ({}, info) => {
      await openWithoutRefusal(await urlFor(mainDocument, info.project.name))
      const results = await readResults(page)

      expect(results.__origin.detail).toBe('null')
      expect(await page.evaluate(() => window.origin)).toBe('null')
      expect(
        await page.evaluate(() => {
          try {
            return document.cookie
          } catch (error) {
            return `threw ${(error as Error).name}`
          }
        }),
      ).not.toContain(parentSecrets.cookie)
      expect(results['own-document.cookie'].outcome).toBe('blocked')
      expect(results['own-localStorage'].outcome).toBe('blocked')
      expectNothingEscaped(results)
      await expectCanaryQuiet()
      expectNoRequestOutsideServer(seen)
    })

    test('a browser that opens the route as a page is refused', async ({}, info) => {
      for (const name of [mainDocument, ...navigationDocuments]) {
        const response = await page.goto(await urlFor(name, info.project.name))
        expect(response?.status(), name).toBe(403)
        expect(await page.content()).not.toContain('<script')
      }
      await page.waitForTimeout(1_000)
      await expectCanaryQuiet()
      expectNoRequestOutsideServer(seen)
    })

    // Known limit of the CSP: a sandboxed document may navigate itself. The
    // refusal above is what closes it. If a browser starts blocking this, the
    // refusal can go.
    test('the CSP sandbox alone does not stop a document navigating itself', async ({}, info) => {
      await openWithoutRefusal(await urlFor('nav-location.html', info.project.name))
      await expect.poll(async () => (await canaryReport()).hits.length, { timeout: 10_000 }).toBeGreaterThan(0)
    })
  })
}
