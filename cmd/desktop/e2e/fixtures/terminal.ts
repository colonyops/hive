import type { Page, Route } from '@playwright/test'
import { expect } from './test.js'
import { bindingID } from './wails.js'

// A body, or a function of the request that returns one. No body answers 204.
type TerminalAnswer = unknown | ((route: Route) => unknown)

export const noAgents = {
  'agents/sessions': { sessions: [] },
  'agents/sessions/all': { sessions: [] },
  'agents/sessions/activity': { items: [] },
  'agents/workspaces': { root: '/w', available: true, workspaces: [], presets: [], editor: { command: '', title: '' } },
}

// Mocks the terminal HTTP API by path below /api/terminal/. The app calls it
// cross-origin, so every answer carries CORS headers and preflights succeed. A
// path with no answer gets a 204.
export async function mockTerminalAPI(page: Page, answers: Record<string, TerminalAnswer>): Promise<void> {
  await page.route('**/api/terminal/**', async (route) => {
    const path = new URL(route.request().url()).pathname.replace('/api/terminal/', '')
    const headers = {
      'Access-Control-Allow-Origin': route.request().headers().origin ?? '*',
      'Access-Control-Allow-Headers': 'Authorization, Content-Type',
      'Access-Control-Allow-Methods': 'POST, OPTIONS',
    }
    if (route.request().method() === 'OPTIONS') return route.fulfill({ status: 204, headers })
    const answer = answers[path]
    const body = typeof answer === 'function' ? answer(route) : answer
    if (body === undefined) return route.fulfill({ status: 204, headers })
    return route.fulfill({ json: body, headers })
  })
}

// Server builds cannot run terminals, so a spec that needs a session mocks the terminal
// control plane. The tabs, the splits and the drags on top of it are real.

export function windowState(windowId: string, paneId: string, name: string, active: boolean) {
  return {
    windowId, name, active, activePane: paneId, width: 100, height: 30,
    layout: { paneId, x: 0, y: 0, width: 100, height: 30, cells: [] },
  }
}

export const terminalSession = (id: string, name: string) => ({
  id, name, slug: id, repo: 'git@github.com:acme/site.git', repoIdentity: 'git@github.com:acme/site.git',
  server: '', state: 'active', canvasOwner: 'acme/site', tags: [], createdAt: '2026-10-01T12:00:00Z',
})

/** Answers the app's calls for these sessions, each with these windows. `answers` adds other bindings. */
export async function mockTerminal(
  page: Page,
  options: {
    sessions: { id: string; name: string }[]
    windows: ReturnType<typeof windowState>[]
    answers?: Record<number, unknown>
  },
) {
  const availability = ['terminalservice', 'agentsservice', 'popupterminalservice'].map((name) => bindingID(name, 'Available'))
  const answers: Record<number, unknown> = {
    [bindingID('sessionservice', 'ListSessions')]: options.sessions.map(({ id, name }) => terminalSession(id, name)),
    [bindingID('terminalservice', 'Scratch')]: { slug: '', name: '' },
    ...options.answers,
  }
  await page.route('**/wails/runtime', async (route) => {
    const id = route.request().postDataJSON()?.args?.methodID as number | undefined
    if (availability.includes(id as number)) return route.fulfill({ json: { available: true, reason: '' } })
    if (id !== undefined && id in answers) return route.fulfill({ json: answers[id] })
    return route.fallback()
  })
  await mockTerminalAPI(page, {
    ...noAgents,
    attach: { windows: options.windows },
    'windows/list': { sessions: Object.fromEntries(options.sessions.map(({ id }) => [id, options.windows])) },
  })
  await page.routeWebSocket('**/api/terminal/stream?*', () => {})
}

// The mocked windows stay 100x30 whatever size the panes ask for, which raises the
// tmux size banner 750ms after the last resize. It pushes the tab rows down 55px, so
// a coordinate read before it lands misses after, and a drag can straddle the shift.
export async function dismissSizeBanner(page: Page) {
  await page.getByTestId('terminal-size-constraint-dismiss').click()
  await expect(page.getByTestId('terminal-size-constraint')).toHaveCount(0)
}
