import type { Page, Route } from '@playwright/test'

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
