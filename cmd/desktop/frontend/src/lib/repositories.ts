import type { SessionLaunchRepository } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/dispatch/models'

/**
 * "git@github.com:owner/name.git" and "https://github.com/owner/name.git" both
 * read as "owner/name"; a bare path keeps its last two segments.
 */
export function repoDisplayName(remote: string): string {
  if (!remote) return ''
  let path = remote.replace(/\.git\/*$/, '').replace(/\/+$/, '')
  const protocol = path.indexOf('://')
  if (protocol !== -1) {
    path = path.slice(protocol + 3)
    const host = path.indexOf('/')
    if (host !== -1) path = path.slice(host + 1)
  } else {
    const colon = path.indexOf(':')
    if (colon !== -1 && path.slice(0, colon).includes('@')) path = path.slice(colon + 1)
  }
  const segments = path.split('/').filter(Boolean)
  if (!segments.length) return remote
  return segments.slice(-2).join('/')
}

/** A repository as the picker shows it: what you read, what the form submits. */
export interface RepositoryChoice {
  /** The git remote — the value a session is created with. */
  remote: string
  /** "owner/name", the primary row text. */
  label: string
  /** The backend's own name for it, shown only when it adds something. */
  hint: string
}

export interface RankedRepository extends RepositoryChoice {
  /** Positions in `label` the query matched, for highlighting. */
  indices: number[]
}

export function toChoices(repositories: SessionLaunchRepository[] | null | undefined): RepositoryChoice[] {
  return (repositories ?? [])
    .filter((repo) => repo.repository.trim() !== '')
    .map((repo) => {
      const label = repoDisplayName(repo.repository) || repo.repository
      const name = repo.name?.trim() ?? ''
      return { remote: repo.repository, label, hint: hintFor(label, name) }
    })
}

// The backend's `name` is a local directory name for a workspace checkout and a
// session name for a Hive clone. The first is almost always the repo name
// already, so it earns a place in the row only when it says something the
// "owner/name" label does not.
function hintFor(label: string, name: string): string {
  if (!name || name === label) return ''
  return name === label.split('/').pop() ? '' : name
}

const CONSECUTIVE_BONUS = 8
const BOUNDARY_BONUS = 6
const START_BONUS = 4
const MAX_GAP_PENALTY = 10

export interface FuzzyMatch {
  score: number
  /** Matched positions in the searched text, ascending. */
  indices: number[]
}

function isBoundary(text: string, index: number): boolean {
  return index === 0 || /[\s/\-_.]/.test(text[index - 1])
}

function matchFrom(text: string, query: string, start: number): FuzzyMatch | null {
  const indices: number[] = []
  let score = 0
  let at = start
  for (let q = 0; q < query.length; q++) {
    const found = text.indexOf(query[q], at)
    if (found === -1) return null
    if (q === 0) {
      if (found === 0) score += START_BONUS
    } else if (found === indices[q - 1] + 1) {
      score += CONSECUTIVE_BONUS
    } else {
      score -= Math.min(found - indices[q - 1] - 1, MAX_GAP_PENALTY)
    }
    if (isBoundary(text, found)) score += BOUNDARY_BONUS
    indices.push(found)
    at = found + 1
  }
  return { score, indices }
}

/**
 * Rank `text` against a lowercased `query` as a subsequence, rewarding runs and
 * segment boundaries so "hd" and "hive-desk" both find "hay-kot/hive-desktop".
 * Returns null when a query character is missing.
 *
 * Every occurrence of the query's first character is tried as a starting point
 * rather than only the leftmost: a greedy scan from the first "h" of
 * "hay-kot/hive" matches "hive" with three gaps and scores it far below the
 * contiguous run the second "h" finds.
 */
export function fuzzyMatch(text: string, query: string): FuzzyMatch | null {
  if (!query) return { score: 0, indices: [] }
  const lower = text.toLowerCase()
  let best: FuzzyMatch | null = null
  for (let start = lower.indexOf(query[0]); start !== -1; start = lower.indexOf(query[0], start + 1)) {
    const match = matchFrom(lower, query, start)
    if (match && (!best || match.score > best.score)) best = match
  }
  return best
}

// A remote hit is worth listing but never worth outranking a label hit: the
// host and ".git" suffix every remote shares would otherwise let a query like
// "git" reorder the whole list.
const REMOTE_MATCH_SCORE = -1000

/**
 * Filter and order `choices` by `query`. An empty query keeps the backend's own
 * order — configured workspaces first, then existing session clones — with
 * `selected` lifted to the top so reopening the picker shows the current choice
 * without a scroll.
 */
export function rankRepositories(choices: RepositoryChoice[], query: string, selected = ''): RankedRepository[] {
  const q = query.trim().toLowerCase()
  if (!q) {
    const ranked = choices.map((choice) => ({ ...choice, indices: [] }))
    const current = ranked.findIndex((choice) => choice.remote === selected)
    if (current > 0) ranked.unshift(...ranked.splice(current, 1))
    return ranked
  }

  return choices
    .map((choice) => {
      const label = fuzzyMatch(choice.label, q)
      if (label) return { choice, score: label.score, indices: label.indices }
      const hint = choice.hint ? fuzzyMatch(choice.hint, q) : null
      if (hint) return { choice, score: hint.score, indices: [] }
      const remote = fuzzyMatch(choice.remote, q)
      if (remote) return { choice, score: REMOTE_MATCH_SCORE + remote.score, indices: [] }
      return null
    })
    .filter((scored): scored is { choice: RepositoryChoice; score: number; indices: number[] } => scored !== null)
    .sort((a, b) => b.score - a.score || a.choice.label.localeCompare(b.choice.label))
    .map(({ choice, indices }) => ({ ...choice, indices }))
}

export interface LabelSegment {
  text: string
  matched: boolean
}

/** Split `label` into alternating plain and matched runs for highlighting. */
export function highlightSegments(label: string, indices: number[]): LabelSegment[] {
  if (!indices.length) return [{ text: label, matched: false }]
  const marked = new Set(indices)
  const segments: LabelSegment[] = []
  for (let i = 0; i < label.length; i++) {
    const matched = marked.has(i)
    const last = segments[segments.length - 1]
    if (last && last.matched === matched) last.text += label[i]
    else segments.push({ text: label[i], matched })
  }
  return segments
}
