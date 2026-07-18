import { computed, onScopeDispose, ref, toValue } from 'vue'
import type { ComputedRef, MaybeRefOrGetter, Ref } from 'vue'

// ─── Types ────────────────────────────────────────────────────────────────────

export interface Command {
  id: string
  /** Palette row text, e.g. "Switch to profile: Desktop" */
  title: string
  /** Section header, e.g. "Profiles", "Feeds", "Window" */
  group?: string
  /** Extra match terms */
  keywords?: string[]
  /** Display-only shortcut hint, e.g. "⌘K" */
  kbd?: string
  run: () => void | Promise<void>
}

interface Registration {
  key: symbol
  source: MaybeRefOrGetter<Command[]>
}

// ─── Module-scoped registry ───────────────────────────────────────────────────

const registrations = ref<Registration[]>([])

// ─── Pure scoring / sorting (exported for unit tests) ─────────────────────────

/**
 * Score a command against a search query.
 *
 * Returns:
 *   3  — prefix match on title
 *   2  — substring match on title
 *   1  — match in keywords or group
 *  -1  — no match (should be filtered out)
 *   0  — empty query (matches everything, no preference)
 */
export function scoreCommand(query: string, cmd: Command): number {
  if (!query) return 0
  const q = query.toLowerCase()
  const t = cmd.title.toLowerCase()
  if (t.startsWith(q)) return 3
  if (t.includes(q)) return 2
  if ((cmd.keywords ?? []).some((k) => k.toLowerCase().includes(q))) return 1
  if (cmd.group?.toLowerCase().includes(q)) return 1
  return -1
}

/** Sort commands by group asc, then title asc. */
export function sortCommands(cmds: Command[]): Command[] {
  return [...cmds].sort((a, b) => {
    const ga = a.group ?? ''
    const gb = b.group ?? ''
    if (ga < gb) return -1
    if (ga > gb) return 1
    return a.title.localeCompare(b.title)
  })
}

/**
 * Filter and rank commands by query.
 *
 * - Empty query: all commands sorted by group then title.
 * - Non-empty: filtered (score ≥ 0), sorted group asc → score desc → title asc.
 */
export function filterAndScore(query: string, cmds: Command[]): Command[] {
  if (!query) return sortCommands(cmds)

  const scored = cmds
    .map((cmd) => ({ cmd, score: scoreCommand(query, cmd) }))
    .filter(({ score }) => score >= 0)

  scored.sort((a, b) => {
    const ga = a.cmd.group ?? ''
    const gb = b.cmd.group ?? ''
    if (ga < gb) return -1
    if (ga > gb) return 1
    if (b.score !== a.score) return b.score - a.score
    return a.cmd.title.localeCompare(b.cmd.title)
  })

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

// ─── Palette state (module singleton) ─────────────────────────────────────────

const _open = ref(false)
const _query = ref('')

/**
 * Returns palette state shared across all callers.
 * CommandPalette.vue consumes this directly (exempt from props-in/events-out
 * for the palette itself).
 */
export function useCommandPalette(): {
  open: Ref<boolean>
  query: Ref<string>
  results: ComputedRef<Command[]>
  toggle(): void
  run(cmd: Command): void | Promise<void>
} {
  const results = computed<Command[]>(() => {
    const all = registrations.value.flatMap((r) => toValue(r.source))
    return filterAndScore(_query.value, all)
  })

  function toggle(): void {
    _open.value = !_open.value
    if (!_open.value) _query.value = ''
  }

  function run(cmd: Command): void | Promise<void> {
    _open.value = false
    _query.value = ''
    return cmd.run()
  }

  return { open: _open, query: _query, results, toggle, run }
}
