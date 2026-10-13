import { computed, shallowReadonly, shallowRef, watch } from 'vue'
import {
  CreateFolder,
  Delete,
  Move,
  Rename,
  Search,
  Spaces,
  Tree,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/driveservice'
import type {
  EntryWire,
  HitWire,
  SpaceWire,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'
import { useWailsEvent } from '../composables/useWailsEvent'
import { errorText } from '../lib/appError'
import { defineStore } from './defineStore'
import { useResource, type Resource } from './useResource'
import { useWindowFocus } from './useWindowFocus'

/** What a reorganization produced, or the error text when the drive refused it. */
export type DriveChange<T> = { result: T; error: null } | { result: null; error: string }

/**
 * Hive Drive as the frontend reads it: the spaces, and each space's tree once
 * something asked for it. A tree is fetched whole and replaced whole; the Go
 * side keeps the snapshot, so a re-read is cheap.
 */
export const useDrive = defineStore('drive', () => {
  const spaces = useResource(async () => (await Spaces()) ?? [], {
    initial: [] as SpaceWire[],
    errorFallback: 'Could not list the drive.',
  })
  const trees = shallowRef<ReadonlyMap<string, EntryWire>>(new Map())
  const treeErrors = shallowRef<ReadonlyMap<string, string>>(new Map())
  const loaders = new Map<string, Resource<EntryWire | null>>()
  const firstLoads = new Map<string, Promise<void>>()

  function loader(space: string): Resource<EntryWire | null> {
    let resource = loaders.get(space)
    if (!resource) {
      resource = useResource(async () => (await Tree(space)) ?? null, {
        initial: null,
        errorFallback: `Could not read ${space}.`,
      })
      loaders.set(space, resource)
    }
    return resource
  }

  async function reloadTree(space: string): Promise<void> {
    const resource = loader(space)
    await resource.reload()
    const errors = new Map(treeErrors.value)
    if (resource.error.value) errors.set(space, resource.error.value)
    else errors.delete(space)
    treeErrors.value = errors
    if (resource.data.value && trees.value.get(space) !== resource.data.value) {
      trees.value = new Map(trees.value).set(space, resource.data.value)
    }
  }

  /** Loads a space's tree the first time it is asked for; later calls reuse it. */
  function ensureTree(space: string): Promise<void> {
    if (!space) return Promise.resolve()
    let first = firstLoads.get(space)
    if (!first) {
      first = reloadTree(space)
      firstLoads.set(space, first)
    }
    return first
  }

  /** Re-reads one space's tree, or the space list and every tree already loaded. */
  async function reload(space?: string): Promise<void> {
    if (space !== undefined) {
      if (loaders.has(space)) await reloadTree(space)
      return
    }
    await Promise.all([spaces.reload(), ...[...loaders.keys()].map(reloadTree)])
  }

  /** Matches paths and titles across every space. Rejects when the drive cannot be read. */
  async function search(query: string, limit = 50): Promise<HitWire[]> {
    return (await Search(query, limit)) ?? []
  }

  // The space is re-read after a refusal too: a not_found usually means the
  // tree on screen is stale.
  async function attempt<T>(space: string, run: () => Promise<T>, fallback: string): Promise<DriveChange<T>> {
    try {
      return { result: await run(), error: null }
    } catch (err) {
      return { result: null, error: errorText(err, fallback) }
    } finally {
      await reload(space)
    }
  }

  function createFolder(space: string, path: string): Promise<DriveChange<EntryWire>> {
    return attempt(space, () => CreateFolder(space, path), `Could not create ${path}.`)
  }

  /** Renames in place; a document keeps its kind's extension. */
  function rename(space: string, path: string, name: string): Promise<DriveChange<EntryWire>> {
    return attempt(space, () => Rename(space, path, name), `Could not rename ${path}.`)
  }

  /** Moves into dir under the same name; dir "" is the space root. */
  function move(space: string, path: string, dir: string): Promise<DriveChange<EntryWire>> {
    return attempt(space, () => Move(space, path, dir), `Could not move ${path}.`)
  }

  /** Deletes a document or an empty folder. */
  function remove(space: string, path: string): Promise<DriveChange<void>> {
    return attempt(space, () => Delete(space, path), `Could not delete ${path}.`)
  }

  useWailsEvent('drive:updated', (event) => {
    const payload = (Array.isArray(event.data) ? event.data[0] : event.data) as { space?: unknown } | undefined
    const space = typeof payload?.space === 'string' ? payload.space : ''
    if (!space) {
      void reload()
      return
    }
    if (!spaces.data.value.some((known) => known.key === space)) void spaces.reload()
    void reload(space)
  })

  // A hidden window can miss a coalesced wake-up, so regaining focus re-reads
  // everything already on screen.
  const { focused } = useWindowFocus()
  watch(focused, (isFocused) => {
    if (isFocused) void reload()
  })

  void spaces.reload()

  return {
    spaces: shallowReadonly(spaces.data),
    spacesLoaded: spaces.loaded,
    error: spaces.error,
    trees: computed(() => trees.value),
    treeErrors: computed(() => treeErrors.value),
    ensureTree,
    reload,
    search,
    createFolder,
    rename,
    move,
    remove,
  }
})
