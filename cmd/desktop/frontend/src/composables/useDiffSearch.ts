import { refDebounced } from '@vueuse/core'
import { computed, ref, watch, type Ref } from 'vue'
import { diffHasText } from '../lib/diffItems'
import { diffRows, sameMatch, searchDiffRows, type DiffRow, type DiffSearchMatch } from '../lib/diffReview'
import type { ChangedFile } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'
import type { DiffEntry } from './useFileDiffs'

const rowsByEntry = new WeakMap<DiffEntry, DiffRow[]>()

/** A held diff's rows, parsed once for as long as that read is held. */
export function entryRows(entry: DiffEntry | undefined): DiffRow[] {
  if (!entry || !diffHasText(entry.diff)) return []
  let rows = rowsByEntry.get(entry)
  if (!rows) {
    rows = diffRows(entry.diff.text)
    rowsByEntry.set(entry, rows)
  }
  return rows
}

/** Find across the diffs read so far, with one match current. Moving the current one reveals it through `reveal`. */
export function useDiffSearch(
  files: Ref<ChangedFile[]>,
  entries: Ref<ReadonlyMap<string, DiffEntry>>,
  reveal: (match: DiffSearchMatch) => void,
) {
  const open = ref(false)
  const query = ref('')
  const debounced = refDebounced(query, 150)
  const active = ref<DiffSearchMatch | null>(null)

  const searched = computed(() =>
    files.value.flatMap((file) => {
      const entry = entries.value.get(file.path)
      return entry ? [{ path: file.path, rows: entryRows(entry) }] : []
    }),
  )
  const matches = computed(() => searchDiffRows(searched.value, debounced.value))
  const activeIndex = computed(() => {
    const current = active.value
    return current ? matches.value.findIndex((match) => sameMatch(match, current)) : -1
  })

  function moveTo(match: DiffSearchMatch | null): void {
    active.value = match
    if (match) reveal(match)
  }

  function step(delta: 1 | -1): void {
    const count = matches.value.length
    if (!count) return
    const from = activeIndex.value
    moveTo(matches.value[from < 0 ? 0 : (from + delta + count) % count])
  }

  // A new query lands on its first match. A refreshed diff keeps the match the
  // user is on, and does not scroll under them.
  watch(debounced, () => moveTo(matches.value[0] ?? null))
  watch(matches, () => {
    if (activeIndex.value < 0) active.value = matches.value[0] ?? null
  })

  function close(): void {
    open.value = false
    query.value = ''
    active.value = null
  }

  return {
    open,
    query,
    debounced,
    matches,
    activeIndex,
    active,
    step,
    close,
    searchedFiles: computed(() => searched.value.length),
  }
}
