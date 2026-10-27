// Ports must match scripts/serve.sh.
export type ServerName = 'feed' | 'onboarding' | 'pipeline' | 'action-smoke' | 'action-seed'

type BrowserName = 'chromium' | 'webkit'

const ports: Record<ServerName, Partial<Record<BrowserName, number>>> = {
  feed: { chromium: 8931, webkit: 8934 },
  onboarding: { chromium: 8932, webkit: 8933 },
  pipeline: { chromium: 8935 },
  'action-smoke': { chromium: 8936 },
  'action-seed': { chromium: 8937 },
}

/** Throws when the harness starts no such server for that browser. */
export function serverURL(server: ServerName, browserName: string): string {
  const port = ports[server][browserName as BrowserName]
  if (!port) throw new Error(`scripts/serve.sh starts no ${server} server for ${browserName}`)
  return `http://127.0.0.1:${port}`
}
