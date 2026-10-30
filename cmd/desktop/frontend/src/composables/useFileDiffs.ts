import { shallowRef, watch, type Ref } from 'vue'
import { SessionFileDiffs as ReadFileDiffs } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/sessionservice'
import type {
  ChangedFile,
  SessionFileDiff,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'
import { errorText } from '../lib/appError'
import type { DiffBase } from './useSessionChanges'

const BATCH_SIZE = 8

export interface DiffEntry {
  /** The version of the file this diff was read for. */
  version: string
  diff: SessionFileDiff
}

/** Changes when a held diff of this file is stale. */
export function fileVersion(file: ChangedFile): string {
  return `${file.status}:${file.additions}:${file.deletions}:${file.stamp}`
}

/**
 * Reads the diffs of the wanted paths, a batch at a time, and keeps them while
 * the view stays open. One request runs at a time, so scrolling through a long
 * list can't stack up git processes. A file whose version moved is read again,
 * and its old diff stays on screen until the new one lands.
 */
export function useFileDiffs(
  sessionId: Ref<string>,
  base: Ref<DiffBase>,
  files: Ref<ChangedFile[]>,
  wanted: Ref<string[]>,
): { entries: Ref<ReadonlyMap<string, DiffEntry>> } {
  const entries = shallowRef<ReadonlyMap<string, DiffEntry>>(new Map())
  let reading = false
  // A read that started before a session or base change must not land after it.
  let generation = 0

  function versions(): Map<string, string> {
    return new Map(files.value.map((file) => [file.path, fileVersion(file)]))
  }

  function stale(version: string | undefined, path: string): boolean {
    const held = entries.value.get(path)
    return version !== undefined && (held?.version !== version || !!held.diff.error)
  }

  async function pump(): Promise<void> {
    if (reading) return
    const current = versions()
    const batch = wanted.value.filter((path) => stale(current.get(path), path)).slice(0, BATCH_SIZE)
    if (!batch.length) return
    const started = generation
    reading = true
    let diffs: SessionFileDiff[]
    let failed: boolean
    try {
      diffs = (await ReadFileDiffs(sessionId.value, base.value, batch)) ?? []
      failed = !diffs.length
    } catch (err) {
      failed = true
      const error = errorText(err, 'Could not read this diff.')
      diffs = batch.map((path) => ({
        path,
        text: '',
        binary: false,
        truncated: false,
        unavailable: false,
        gone: false,
        error,
      }))
    }
    reading = false
    if (started !== generation) return void pump()
    const next = new Map(entries.value)
    for (const diff of diffs) next.set(diff.path, { version: current.get(diff.path) ?? '', diff })
    entries.value = next
    // A failed request waits for the next poll, or it would retry in a tight loop.
    if (!failed && !diffs.some((diff) => diff.error)) void pump()
  }

  watch([sessionId, base], () => {
    generation++
    entries.value = new Map()
    void pump()
  })
  watch([files, wanted], () => void pump(), { immediate: true })

  return { entries }
}
