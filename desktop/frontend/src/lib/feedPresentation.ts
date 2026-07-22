// Adapter-to-presentation seam for the inbox. GitHub is the only adapter
// today; future sources branch here instead of widening the inbox wire DTO.
import type { InboxItem } from '../types/feed'

export interface FeedSource { key: 'github'; label: string }
export interface GithubPayload {
  repo: string; num: number; kind: string; author: string; body: string
  branch: string; url: string; labels: string[]; prompt: string; reason?: string
}

export function feedSource(_item?: { sourceKind?: string; url?: string }): FeedSource {
  return { key: 'github', label: 'GitHub' }
}

export function githubPayload(item: InboxItem): GithubPayload {
  if (item.sourceKind !== 'github' || !item.payload || typeof item.payload !== 'object') {
    return { repo: '', num: 0, kind: '', author: '', body: '', branch: '', url: item.url, labels: [], prompt: '' }
  }
  const value = item.payload as Record<string, unknown>
  const string = (key: string): string => typeof value[key] === 'string' ? value[key] as string : ''
  return {
    repo: string('repo'), num: typeof value.num === 'number' ? value.num : 0,
    kind: string('kind'), author: string('author'), body: string('body'),
    branch: string('branch'), url: string('url') || item.url,
    labels: Array.isArray(value.labels) ? value.labels.filter((label): label is string => typeof label === 'string') : [],
    prompt: string('prompt'), reason: string('reason') || undefined,
  }
}

export function typeLabel(kind: string): string {
  if (kind === 'PR') return 'Pull Request'
  if (kind === 'Issue') return 'Issue'
  return kind || 'Item'
}

export function bodySnippet(body: string): string {
  for (const raw of body.split('\n')) {
    const line = raw.replace(/^#{1,6}\s+/, '').replace(/^\s*[-*+]\s+\[[ xX]\]\s+/, '').replace(/^\s*[-*+>]\s+/, '').replace(/!?\[([^\]]*)\]\([^)]*\)/g, '$1').replace(/[*_`~]/g, '').trim()
    if (line) return line
  }
  return ''
}
