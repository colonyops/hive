import { computed, onScopeDispose, ref, toValue, watch } from 'vue'
import type { Component, ComputedRef, MaybeRefOrGetter, Ref } from 'vue'
import {
  scopeForSigil,
  type CommandScope,
  type PaletteScopeId,
  type PaletteScopeSpec,
  paletteScopes,
} from '../palette/scopes'
import { usePaletteRecents } from './usePaletteRecents'

// ─── Types ────────────────────────────────────────────────────────────────────

export interface Command {
  id: string
  /**
   * Palette row text. A command row keeps its verb ("Mark all as read"); an
   * object row is titled after the object itself ("Desktop", not "Switch to
   * profile: Desktop") — its container is `group`, not a hand-built prefix.
   */
  title: string
  /** Section header, e.g. "Profiles", "Settings" — an object row's own container. */
  group?: string
  /**
   * Group placement: lower sorts earlier, default 0, ties broken by group
   * name. The group sorts as early as its earliest row asks, so registrars in
   * different files need not agree on the value and a group never splits.
   */
  order?: number
  /** Extra match terms */
  keywords?: string[]
  /** Icon shown in the row's leading chip; palette falls back to a default */
  icon?: Component
  /**
   * An AppIcon registry name, for rows whose glyph is named rather than
   * imported — a configured action carries its type's icon as a string, the
   * same one its menu entry draws. Ignored when `icon` is set.
   */
  iconName?: string
  /** Tints the named icon, as the action menus tint theirs. */
  iconColor?: string
  /** Right-aligned mono hint, e.g. a shortcut or context label */
  hint?: string
  /**
   * Muted type word at the row's right edge — what Enter lands on ('window',
   * 'feed', 'chat'). Object rows only; a verb row's title already says.
   */
  kind?: string
  /** Palette scope: 'goto' rows browse, 'actions' rows act. Default 'actions'. */
  scope?: CommandScope
  /** Running the row keeps the palette open (sigil-legend rows). */
  keepOpen?: boolean
  run: () => void | Promise<void>
}

interface Registration {
  key: symbol
  source: MaybeRefOrGetter<Command[]>
}

// ─── Module-scoped registry ───────────────────────────────────────────────────

const registrations = ref<Registration[]>([])

// ─── Pure scoring / sorting (exported for unit tests) ─────────────────────────

export interface FuzzyMatch {
  score: number
  /** Indices of the matched characters in `text`, for highlighting. */
  positions: number[]
}

// Weight tiers, each an order of magnitude above the next so a component
// never outweighs one above it: a query long enough to rack up every
// consecutive-run bonus available still can't out-score a single word-start
// hit, and any number of word-start hits still can't out-score the
// whole-prefix bonus.
const PREFIX_BONUS = 500
const WORD_START_BONUS = 40
const CONSECUTIVE_BONUS = 8
const GAP_PENALTY = 1

function isWordChar(code: number): boolean {
  return (code >= 48 && code <= 57) || (code >= 65 && code <= 90) || (code >= 97 && code <= 122)
}

/**
 * Lowercases character-by-character rather than the whole string, keeping a
 * character whose lowercase form isn't length 1 (e.g. İ → i̇) as-is instead.
 * A length-changing mapping would shift every position after it out of step
 * with the original string's indices, which `fuzzyMatch`'s positions and
 * `titleSegments`'s slicing both key off of.
 */
function toComparable(s: string): string {
  let out = ''
  for (let i = 0; i < s.length; i++) {
    const lower = s[i].toLowerCase()
    out += lower.length === 1 ? lower : s[i]
  }
  return out
}

export interface FuzzyMatchOptions {
  /** Reject a match whose first character is not at a word start. */
  wordStart?: boolean
}

function isWordStart(t: string, at: number): boolean {
  return at === 0 || !isWordChar(t.charCodeAt(at - 1))
}

function charScore(t: string, at: number): number {
  return 1 + (isWordStart(t, at) ? WORD_START_BONUS : 0)
}

/**
 * Case-insensitive subsequence match. Null when any query character cannot be
 * placed in order. Score rewards, in weight order: whole-prefix, word-start
 * hits, consecutive runs; penalizes gaps. Deterministic ints, no locale work.
 *
 * Placement is optimal rather than greedy: a DP over (query char, text
 * position) keeps the highest-scoring alignment, so "kb" lands on word starts
 * instead of the first "k" it meets. O(|query| x |text|).
 */
export function fuzzyMatch(query: string, text: string, options: FuzzyMatchOptions = {}): FuzzyMatch | null {
  if (!query) return null
  const q = toComparable(query)
  const t = toComparable(text)
  const n = q.length
  const m = t.length
  if (n > m) return null

  // best[i * m + j]: top score with q[0..i] placed and q[i] at t[j].
  const best = new Float64Array(n * m).fill(-Infinity)
  const from = new Int32Array(n * m).fill(-1)

  for (let i = 0; i < n; i++) {
    const row = i * m
    const prev = row - m
    // Running max of best[i-1][k] + k * GAP_PENALTY over k <= j - 2, so a
    // gapped predecessor costs (j - k - 1) * GAP_PENALTY in O(1) per cell.
    let gapMax = -Infinity
    let gapArg = -1
    for (let j = i; j < m; j++) {
      if (i > 0 && j >= 2) {
        const v = best[prev + j - 2] + (j - 2) * GAP_PENALTY
        if (v > gapMax) {
          gapMax = v
          gapArg = j - 2
        }
      }
      if (t[j] !== q[i]) continue
      if (i === 0) {
        if (!options.wordStart || isWordStart(t, j)) best[j] = charScore(t, j)
        continue
      }
      const adjacent = best[prev + j - 1] + CONSECUTIVE_BONUS
      const gapped = gapMax - (j - 1) * GAP_PENALTY
      if (adjacent === -Infinity && gapped === -Infinity) continue
      best[row + j] = charScore(t, j) + Math.max(adjacent, gapped)
      from[row + j] = adjacent >= gapped ? j - 1 : gapArg
    }
  }

  const last = (n - 1) * m
  let end = -1
  for (let j = n - 1; j < m; j++) {
    if (best[last + j] > (end < 0 ? -Infinity : best[last + end])) end = j
  }
  if (end < 0) return null

  let score = best[last + end]
  let positions: number[] = []
  for (let i = n - 1, j = end; i >= 0; j = from[i * m + j], i--) positions.push(j)
  positions.reverse()

  if (t.startsWith(q)) {
    let prefixScore = PREFIX_BONUS + (n - 1) * CONSECUTIVE_BONUS
    for (let j = 0; j < n; j++) prefixScore += charScore(t, j)
    if (prefixScore > score) {
      score = prefixScore
      positions = Array.from({ length: n }, (_, j) => j)
    }
  }

  return { score, positions }
}

// Bands, each an order of magnitude above the next so a title hit always
// outranks a keyword hit, which always outranks a group hit, while ranking
// within a tier by fuzzy quality instead of treating every hit in a tier as
// equal.
const TITLE_BAND = 1_000_000
const KEYWORD_BAND = 500_000
const GROUP_BAND = 100_000

// "Settings > Keyboard", "settings/keyboard", and "settings keyboard" all
// split into the same terms.
const TOKEN_SEPARATORS = /[\s>/:,]+/

function queryTokens(query: string): string[] {
  return query.split(TOKEN_SEPARATORS).filter(Boolean)
}

// Several short terms, each matched as a loose subsequence, would let nearly
// any row through, so a term in a multi-term query has to start on a word.
function tokenOptions(tokens: string[]): FuzzyMatchOptions {
  return { wordStart: tokens.length > 1 }
}

function scoreToken(token: string, cmd: Command, options: FuzzyMatchOptions): { band: number; score: number } | null {
  const title = fuzzyMatch(token, cmd.title, options)
  if (title) return { band: TITLE_BAND, score: title.score }

  let keyword: FuzzyMatch | null = null
  for (const k of cmd.keywords ?? []) {
    const match = fuzzyMatch(token, k, options)
    if (match && (!keyword || match.score > keyword.score)) keyword = match
  }
  if (keyword) return { band: KEYWORD_BAND, score: keyword.score }

  const group = cmd.group ? fuzzyMatch(token, cmd.group, options) : null
  if (group) return { band: GROUP_BAND, score: group.score }

  return null
}

/**
 * A whole-query title match ranks first. Otherwise the query splits into
 * terms and every term must match the title, a keyword, or the group; the row
 * takes the band of its strongest field plus the sum of the term scores, so
 * "settings keyboard" finds the Keyboard row in the Settings group. -1 =
 * filtered out, 0 = empty query.
 */
export function scoreCommand(query: string, cmd: Command): number {
  if (!query) return 0

  const titleMatch = fuzzyMatch(query, cmd.title)
  if (titleMatch) return TITLE_BAND + titleMatch.score

  const tokens = queryTokens(query)
  if (tokens.length === 0) return 0
  const options = tokenOptions(tokens)

  let band = 0
  let score = 0
  for (const token of tokens) {
    const hit = scoreToken(token, cmd, options)
    if (!hit) return -1
    band = Math.max(band, hit.band)
    score += hit.score
  }
  return band + score
}

/**
 * Title positions to highlight for `query`: the whole-query match when there
 * is one, else the union of each term's title match.
 */
export function titleMatchPositions(query: string, title: string): number[] {
  const whole = fuzzyMatch(query, title)
  if (whole) return whole.positions

  const tokens = queryTokens(query)
  const options = tokenOptions(tokens)
  const positions = new Set<number>()
  for (const token of tokens) {
    for (const p of fuzzyMatch(token, title, options)?.positions ?? []) positions.add(p)
  }
  return [...positions].sort((a, b) => a - b)
}

// A group's placement is the minimum `order` across all its rows, derived
// over the whole input rather than read per row — one group is often
// registered from more than one file (App-level window rows and
// TerminalMode's session ops share the attached session's name), and a
// per-row comparison would split the group whenever the registrars disagree.
function groupPlacement(cmds: Command[]): Map<string, number> {
  const placement = new Map<string, number>()
  for (const cmd of cmds) {
    const group = cmd.group ?? ''
    const order = cmd.order ?? 0
    const known = placement.get(group)
    if (known === undefined || order < known) placement.set(group, order)
  }
  return placement
}

function compareGroupPlacement(placement: Map<string, number>, a: Command, b: Command): number {
  const ga = a.group ?? ''
  const gb = b.group ?? ''
  const delta = (placement.get(ga) ?? 0) - (placement.get(gb) ?? 0)
  if (delta) return delta
  return ga < gb ? -1 : ga > gb ? 1 : 0
}

/**
 * Sort commands by group placement (each group at the earliest `order` any of
 * its rows asks, then group name asc). The sort is stable, so rows keep their
 * registration order within a group — which is how positional lists (a
 * session's windows, the sidebar's feeds) stay in their own order rather than
 * alphabetical.
 */
export function sortCommands(cmds: Command[]): Command[] {
  const placement = groupPlacement(cmds)
  return [...cmds].sort((a, b) => compareGroupPlacement(placement, a, b))
}

/**
 * Filter and rank commands by query.
 *
 * - Empty query: all commands in group placement order.
 * - Non-empty: filtered (score ≥ 0), best score first; group placement only
 *   breaks ties, so a strong title match beats an early group's keyword hit.
 */
export function filterAndScore(query: string, cmds: Command[]): Command[] {
  if (!query) return sortCommands(cmds)

  const placement = groupPlacement(cmds)
  const scored = cmds.map((cmd) => ({ cmd, score: scoreCommand(query, cmd) })).filter(({ score }) => score >= 0)

  scored.sort((a, b) => b.score - a.score || compareGroupPlacement(placement, a.cmd, b.cmd))

  return scored.map((s) => s.cmd)
}

// ─── Registration ─────────────────────────────────────────────────────────────

/**
 * Registers commands for the lifetime of the calling effect scope.
 * Auto-unregisters via onScopeDispose.
 *
 * Accepts a MaybeRefOrGetter so reactive sources (profiles, feeds) stay live:
 *   useCommands(computed(() => profiles.value.map(...)))
 */
export function useCommands(commands: MaybeRefOrGetter<Command[]>): void {
  const key = Symbol()
  registrations.value = [...registrations.value, { key, source: commands }]

  onScopeDispose(() => {
    registrations.value = registrations.value.filter((r) => r.key !== key)
  })
}

// ─── Shell escape (`!` queries) ───────────────────────────────────────────────

interface ShellEscapeRegistration {
  key: symbol
  source: (line: string) => Command[]
  available: () => boolean
}

const shellEscapes = ref<ShellEscapeRegistration[]>([])

/**
 * Claims the Shell scope for the lifetime of the calling effect scope.
 * `available` drives the Shell tab's visibility (default: always shown) so it
 * can hide outside the view it applies to; `source` still returns [] where a
 * given line cannot run (e.g. no attached session) — an empty line is never
 * offered either.
 */
export function useShellEscape(source: (line: string) => Command[], available: () => boolean = () => true): void {
  const key = Symbol()
  shellEscapes.value = [...shellEscapes.value, { key, source, available }]

  onScopeDispose(() => {
    shellEscapes.value = shellEscapes.value.filter((r) => r.key !== key)
  })
}

// ─── Keys scope (`?` queries) ───────────────────────────────────────────────

interface KeysScopeRegistration {
  key: symbol
  source: () => Command[]
}

const keysScopes = ref<KeysScopeRegistration[]>([])

/**
 * Claims the Keys scope for the calling effect scope. Kept catalog-agnostic
 * (like useShellEscape) so this module never imports keybindings/catalog;
 * useAppPaletteRows supplies `source` over keymapRows.
 */
export function useKeysScope(source: () => Command[]): void {
  const key = Symbol()
  keysScopes.value = [...keysScopes.value, { key, source }]

  onScopeDispose(() => {
    keysScopes.value = keysScopes.value.filter((r) => r.key !== key)
  })
}

// ─── Palette state (module singleton) ─────────────────────────────────────────

const _open = ref(false)
const _query = ref('')
const _scope = ref<PaletteScopeId>('all')

/** Registry entries whose tab is currently shown (Shell/Keys only where available). */
const visibleScopes = computed<PaletteScopeSpec[]>(() =>
  paletteScopes.filter((s) => {
    if (s.id === 'shell') return shellEscapes.value.some((r) => r.available())
    if (s.id === 'keys') return keysScopes.value.length > 0
    return true
  }),
)

// A tab that goes away (e.g. Shell on a view switch) must not strand the
// palette on a scope with no way back to it via the tab strip.
watch(
  visibleScopes,
  (visible) => {
    if (_open.value && !visible.some((s) => s.id === _scope.value)) _scope.value = 'all'
  },
  { flush: 'sync' },
)

/**
 * Returns palette state shared across all callers.
 * CommandPalette.vue consumes this directly (exempt from props-in/events-out
 * for the palette itself).
 */
export function useCommandPalette(): {
  open: Ref<boolean>
  query: Ref<string>
  scope: Ref<PaletteScopeId>
  visibleScopes: ComputedRef<PaletteScopeSpec[]>
  results: ComputedRef<Command[]>
  toggle(): void
  openWithScope(scope: PaletteScopeId): void
  setQuery(next: string): void
  setScope(scope: PaletteScopeId): void
  cycleScope(delta: 1 | -1): void
  popScope(): boolean
  run(cmd: Command): void | Promise<void>
} {
  const { recordRun } = usePaletteRecents()

  const results = computed<Command[]>(() => {
    const query = _query.value
    const allCommands = () => registrations.value.flatMap((r) => toValue(r.source))

    switch (_scope.value) {
      case 'shell': {
        const line = query.trim()
        if (!line) return []
        return shellEscapes.value.flatMap((r) => r.source(line))
      }
      case 'keys':
        return filterAndScore(
          query,
          keysScopes.value.flatMap((r) => r.source()),
        )
      case 'goto':
      case 'actions': {
        const wanted = _scope.value
        return filterAndScore(
          query,
          allCommands().filter((cmd) => (cmd.scope ?? 'actions') === wanted),
        )
      }
      default:
        return filterAndScore(query, allCommands())
    }
  })

  function toggle(): void {
    _open.value = !_open.value
    if (!_open.value) {
      _query.value = ''
      _scope.value = 'all'
    }
  }

  function openWithScope(scope: PaletteScopeId): void {
    _open.value = true
    _scope.value = scope
    _query.value = ''
  }

  /**
   * The input's write path. Performs sigil interception: an empty query
   * followed by a visible scope's sigil enters that scope and absorbs the
   * sigil, leaving the rest of `next` as the query — so a paste of "@foo"
   * lands in the goto scope with query "foo". A non-empty prior query means
   * the sigil is mid-edit, not an entry point, so it is left as literal text.
   */
  function setQuery(next: string): void {
    if (!_query.value && next) {
      const entered = scopeForSigil(next[0])
      if (entered && visibleScopes.value.some((s) => s.id === entered)) {
        _scope.value = entered
        _query.value = next.slice(1)
        return
      }
    }
    _query.value = next
  }

  function setScope(scope: PaletteScopeId): void {
    _scope.value = scope
  }

  function cycleScope(delta: 1 | -1): void {
    const visible = visibleScopes.value
    if (visible.length === 0) return
    const at = visible.findIndex((s) => s.id === _scope.value)
    const from = at >= 0 ? at : 0
    _scope.value = visible[(from + delta + visible.length) % visible.length].id
  }

  function popScope(): boolean {
    if (_query.value || _scope.value === 'all') return false
    _scope.value = 'all'
    return true
  }

  function run(cmd: Command): void | Promise<void> {
    // A sigil-legend row switches scope itself; closing around that would undo
    // the very navigation the row exists to offer. It is chrome, not a
    // repeatable entry, so it is never recorded either.
    if (cmd.keepOpen) return cmd.run()
    // Shell lines aren't repeatable entries and Keys rows are reference, so
    // only All/Go to/Actions rows earn a place in Recent.
    if (_scope.value === 'all' || _scope.value === 'goto' || _scope.value === 'actions') recordRun(cmd.id)
    _open.value = false
    _query.value = ''
    _scope.value = 'all'
    return cmd.run()
  }

  return {
    open: _open,
    query: _query,
    scope: _scope,
    visibleScopes,
    results,
    toggle,
    openWithScope,
    setQuery,
    setScope,
    cycleScope,
    popScope,
    run,
  }
}
