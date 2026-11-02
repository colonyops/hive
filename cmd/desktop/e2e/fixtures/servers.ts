// Ports must match scripts/serve.sh. Each Playwright worker owns one group of
// servers, so its tests cannot see another worker's state: a group's ports are
// the ports below plus GROUP_PORT_STEP per group.
export const GROUP_PORT_STEP = 100

export type ServerName = 'feed' | 'onboarding' | 'pipeline' | 'action-smoke' | 'action-seed' | 'remote'

type BrowserName = 'chromium' | 'webkit'

const ports: Record<ServerName, Partial<Record<BrowserName, number>>> = {
  feed: { chromium: 8931, webkit: 8934 },
  onboarding: { chromium: 8932, webkit: 8933 },
  pipeline: { chromium: 8935 },
  'action-smoke': { chromium: 8936 },
  'action-seed': { chromium: 8937 },
  // Started only in the remote lane (scripts/run-remote.sh).
  remote: { chromium: 8938 },
}

/** Throws when the harness starts no such server for that browser. */
export function serverURL(server: ServerName, browserName: string): string {
  const port = ports[server][browserName as BrowserName]
  if (!port) throw new Error(`scripts/serve.sh starts no ${server} server for ${browserName}`)
  // Playwright sets this in each worker. It stays below the worker count, which
  // serve.sh starts one group per, and a replacement worker reuses it.
  const group = Number(process.env.TEST_PARALLEL_INDEX ?? 0)
  return `http://127.0.0.1:${port + group * GROUP_PORT_STEP}`
}
