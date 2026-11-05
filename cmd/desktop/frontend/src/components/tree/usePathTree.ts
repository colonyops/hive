import { FileTree, type FileTreeDirectoryHandle, type FileTreeOptions, type GitStatusEntry } from '@pierre/trees'
import { useStorage } from '@vueuse/core'
import { onScopeDispose, ref, toValue, watch, type MaybeRefOrGetter, type Ref } from 'vue'

export interface PathTreeOptions extends Omit<
  FileTreeOptions,
  'paths' | 'preparedInput' | 'initialExpansion' | 'gitStatus'
> {
  /** Followed: a new list resets the tree and keeps its open folders open. */
  paths: MaybeRefOrGetter<readonly string[]>
  gitStatus?: MaybeRefOrGetter<readonly GitStatusEntry[]>
  /**
   * localStorage key of the fold state, as `{ [folder]: open }`. A folder with
   * no entry takes defaultOpen, so a bulk fold survives the next reload.
   */
  storageKey: string
  /** Prefixes a folder's storage entry, so one key can hold several trees. */
  storagePrefix?: () => string
  defaultOpen: boolean
}

export interface PathTree {
  model: FileTree
  /** Puts back the last paths set, undoing a rename or drop the tree applied before the server refused it. */
  resync: () => void
  folders: () => string[]
  setAllFolders: (open: boolean) => void
  anyFolderOpen: Readonly<Ref<boolean>>
  /** Opens the folders above path, selects it alone, and scrolls it into view. */
  reveal: (path: string) => void
  /** Makes path the only selected row; null clears the selection. */
  selectOnly: (path: string | null) => void
  /** The rendered row of path, for anchoring a menu. */
  rowElement: (path: string) => HTMLElement | null
  /** Re-runs renderRowDecoration on every row, for state the tree cannot see. */
  redecorate: () => void
}

/** Every folder above a path, without the trailing slash the tree adds. */
export function folderPaths(paths: readonly string[]): string[] {
  const out = new Set<string>()
  for (const path of paths) {
    const parts = path.split('/').filter(Boolean)
    const last = path.endsWith('/') ? parts.length : parts.length - 1
    for (let i = 1; i <= last; i++) out.add(parts.slice(0, i).join('/'))
  }
  return [...out]
}

/** The folder handle at path, or null for a file or no row. */
export function directory(model: FileTree, path: string): FileTreeDirectoryHandle | null {
  const item = model.getItem(path)
  return item && 'expand' in item ? item : null
}

/**
 * A @pierre/trees model bound to the component scope. The tree owns fold
 * state while it lives; this mirrors it into localStorage so it outlives a
 * remount, and hands it back on every setPaths, because resetPaths rebuilds
 * the store from initialExpandedPaths alone.
 */
export function usePathTree(options: PathTreeOptions): PathTree {
  const { storageKey, storagePrefix, defaultOpen, paths: pathsSource, gitStatus, ...treeOptions } = options
  const stored = useStorage<Record<string, boolean>>(storageKey, {})
  const key = (folder: string) => (storagePrefix ? `${storagePrefix()}\n${folder}` : folder)
  const storedOpen = (folder: string) => stored.value[key(folder)] ?? defaultOpen

  let paths = toValue(pathsSource)
  const folders = () => folderPaths(paths)
  const anyFolderOpen = ref(false)

  const model = new FileTree({
    density: 'compact',
    icons: { set: 'complete', colored: true },
    flattenEmptyDirectories: true,
    ...treeOptions,
    paths,
    gitStatus: toValue(gitStatus),
    initialExpansion: 'closed',
    initialExpandedPaths: folders().filter(storedOpen),
  })

  function liveOpen(): string[] {
    return folders().filter((folder) => directory(model, folder)?.isExpanded() ?? false)
  }

  // A search opens every folder with a match; that is not the user folding.
  const stop = model.subscribe(() => {
    const open = liveOpen()
    anyFolderOpen.value = open.length > 0
    if (model.getSearchValue()) return
    const openSet = new Set(open)
    for (const folder of folders()) {
      const isOpen = openSet.has(folder)
      if (storedOpen(folder) !== isOpen) stored.value[key(folder)] = isOpen
    }
  })
  anyFolderOpen.value = liveOpen().length > 0

  watch(
    () => toValue(pathsSource),
    (next) => {
      if (next.length === paths.length && next.every((path, i) => path === paths[i])) return
      reset(next)
    },
  )
  watch(
    () => toValue(gitStatus),
    (next) => model.setGitStatus(next),
  )

  onScopeDispose(() => {
    stop()
    model.cleanUp()
  })

  function reset(next: readonly string[]): void {
    const open = new Set(liveOpen())
    paths = next
    model.resetPaths(next, {
      initialExpandedPaths: folders().filter((folder) => open.has(folder) || storedOpen(folder)),
    })
  }

  function selectOnly(path: string | null): void {
    for (const selected of model.getSelectedPaths()) {
      if (selected !== path) model.getItem(selected)?.deselect()
    }
    if (path && !model.getItem(path)?.isSelected()) model.getItem(path)?.select()
  }

  return {
    model,
    folders,
    anyFolderOpen,
    selectOnly,
    resync: () => reset(paths),
    setAllFolders(open) {
      for (const folder of folders()) {
        const dir = directory(model, folder)
        if (open) dir?.expand()
        else dir?.collapse()
      }
    },
    reveal(path) {
      for (const folder of folderPaths([path])) directory(model, folder)?.expand()
      selectOnly(path)
      model.scrollToPath(path, { focus: false, offset: 'nearest' })
    },
    rowElement(path) {
      const root = model.getFileTreeContainer()?.shadowRoot
      const canonical = model.getItem(path)?.getPath() ?? path
      return root?.querySelector<HTMLElement>(`[role="treeitem"][data-item-path=${JSON.stringify(canonical)}]`) ?? null
    },
    redecorate() {
      model.setComposition(model.getComposition())
    },
  }
}
