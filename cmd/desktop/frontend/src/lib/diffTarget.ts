const LOCAL_PREFIX = 'refs/heads/'
const REMOTE_PREFIX = 'refs/remotes/'

export function isRemoteRef(ref: string): boolean {
  return ref.startsWith(REMOTE_PREFIX)
}

export function branchName(ref: string): string {
  if (ref.startsWith(LOCAL_PREFIX)) return ref.slice(LOCAL_PREFIX.length)
  return isRemoteRef(ref) ? ref.slice(REMOTE_PREFIX.length) : ref
}

export function shortCommit(id: string): string {
  return id.slice(0, 8)
}
