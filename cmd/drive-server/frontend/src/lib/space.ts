export interface SpaceRef {
  key: string
  title: string
  /** The space's `/api/v1/...` prefix. */
  apiPrefix: string
  /** The space's page route, without a trailing slash. */
  routePrefix: string
}

const enc = encodeURIComponent

export function spaceFromKey(key: string): SpaceRef {
  if (key === 'shared') return { key, title: 'Shared', apiPrefix: '/api/v1/shared', routePrefix: '/shared' }
  if (key.startsWith('workspaces/')) {
    const name = key.slice('workspaces/'.length)
    return {
      key,
      title: name,
      apiPrefix: `/api/v1/workspaces/${enc(name)}`,
      routePrefix: `/workspaces/${enc(name)}`,
    }
  }
  const [owner = '', repo = ''] = key.split('/')
  return {
    key,
    title: key,
    apiPrefix: `/api/v1/repos/${enc(owner)}/${enc(repo)}`,
    routePrefix: `/repos/${enc(owner)}/${enc(repo)}`,
  }
}

type Param = string | string[] | undefined

function single(value: Param): string {
  return Array.isArray(value) ? value.join('/') : (value ?? '')
}

/** The space and document path a page route names. `path` is '' for the space's document list. */
export function fromRouteParams(params: Record<string, Param>): { space: SpaceRef; path: string } {
  const path = single(params.path)
  if (params.owner !== undefined) return { space: spaceFromKey(`${single(params.owner)}/${single(params.repo)}`), path }
  if (params.name !== undefined) return { space: spaceFromKey(`workspaces/${single(params.name)}`), path }
  return { space: spaceFromKey('shared'), path }
}

export function documentRoute(space: SpaceRef, path: string): string {
  return `${space.routePrefix}/${path.split('/').map(enc).join('/')}`
}
