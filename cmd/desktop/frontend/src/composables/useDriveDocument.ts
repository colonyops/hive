import { computed, ref, shallowRef, watch, type Ref, type ShallowRef } from 'vue'
import { Document } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/driveservice'
import type {
  DocumentWire,
  EntryWire,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'
import { errorText } from '../lib/appError'
import { documentsOf, newestDocument, type DriveAuthor } from '../lib/drive'
import { useDrive } from '../stores/useDrive'
import { useDriveActivity } from '../stores/useDriveActivity'
import { usePerf } from './usePerf'

/**
 * One drive surface's document: the space it browses, its tree from the
 * store, and the document on screen. Per-surface, so the pane and the
 * overlay each keep their own pick. The generation guard keeps an older
 * response from replacing a newer request's when a tree reload and a pick
 * race.
 *
 * The document shown is the requested path when there is one, else the
 * newest document the session wrote, else the newest in the space.
 */
export function useDriveDocument() {
  const drive = useDrive()
  const activity = useDriveActivity()
  const perf = usePerf('drive')

  const document: ShallowRef<DocumentWire | null> = shallowRef(null)
  const shown: Ref<string | null> = ref(null)
  const space = ref('')
  const requested: Ref<string | null> = ref(null)
  const preferSession: Ref<DriveAuthor | null> = ref(null)
  const loading = ref(false)
  const error = ref<string | null>(null)

  const tree = computed<EntryWire | null>(() => drive.trees.value.get(space.value) ?? null)
  const treeError = computed(() => drive.treeErrors.value.get(space.value) ?? null)
  const entries = computed(() => new Map(documentsOf(tree.value).map((entry) => [entry.path, entry])))
  const paths = computed(() => new Set(entries.value.keys()))

  let listedSpace = ''
  let listedPaths: ReadonlySet<string> = new Set()
  let generation = 0

  function defaultPath(): string | null {
    const own = activity.lastWrite(preferSession.value, space.value)
    if (own && paths.value.has(own)) return own
    return newestDocument(tree.value)?.path ?? null
  }

  async function reload(): Promise<void> {
    const token = ++generation
    const key = space.value
    loading.value = true
    try {
      await drive.ensureTree(key)
      if (token !== generation) return
      // A pinned path that was listed and no longer is was deleted or moved,
      // so the default takes over. A path never listed stays pinned and blank:
      // an agent can open a document before its first write.
      const pinned = requested.value
      if (pinned && key === listedSpace && listedPaths.has(pinned) && !paths.value.has(pinned)) {
        requested.value = null
      }
      listedSpace = key
      listedPaths = paths.value
      const path = requested.value ?? defaultPath()
      const entry = path ? entries.value.get(path) : undefined
      // A tree reload names the same document more often than not (another
      // document changed, or the window regained focus), and the document on
      // screen is kept rather than fetched and rendered again: a large one
      // with many code fences is the expensive part of the whole surface.
      const current = document.value
      const unchanged =
        !!current &&
        !!entry &&
        current.space.key === key &&
        current.entry.path === path &&
        current.entry.modified === entry.modified &&
        current.entry.size === entry.size
      const loaded = unchanged
        ? current
        : key && path && entry
          ? await perf.track('document:load', () => Document(key, path), { space: key, path })
          : null
      if (token !== generation) return
      shown.value = path
      document.value = loaded
      error.value = null
      if (key && path && loaded) activity.markOpened(key, path)
    } catch (err) {
      if (token !== generation) return
      document.value = null
      error.value = errorText(err, 'Could not read the document.')
    } finally {
      if (token === generation) loading.value = false
    }
  }

  function wake(): void {
    void reload()
  }

  /**
   * Points the surface at a space: path pins one document, null lets the
   * default win, and session is the open chat or Code session the default
   * prefers.
   */
  function show(nextSpace: string, path: string | null, session: DriveAuthor | null): void {
    space.value = nextSpace
    requested.value = path
    preferSession.value = session
    wake()
  }

  // The store replaces a tree on every drive:updated for its space, which is
  // the only wake-up a surface needs.
  watch(tree, (next, before) => {
    if (next && before && next !== before) wake()
  })

  return { document, tree, treeError, paths, shown, space, loading, error, show, wake }
}
