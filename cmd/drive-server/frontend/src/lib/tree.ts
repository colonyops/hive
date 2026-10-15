import type { DriveDocument } from '../types'

export interface TreeNode {
  kind: 'dir' | 'doc'
  /** The last path segment. */
  name: string
  path: string
  label: string
  doc?: DriveDocument
  children: TreeNode[]
}

export interface TreeRow {
  node: TreeNode
  depth: number
}

// Numeric so "2-setup" sorts before "10-faq".
const collator = new Intl.Collator('en', { numeric: true, sensitivity: 'base' })

function basename(path: string): string {
  return path.slice(path.lastIndexOf('/') + 1)
}

export function documentLabel(doc: DriveDocument): string {
  return doc.title || basename(doc.path)
}

function sortNodes(nodes: TreeNode[]): void {
  nodes.sort((a, b) => (a.kind === b.kind ? collator.compare(a.name, b.name) : a.kind === 'dir' ? -1 : 1))
  for (const node of nodes) sortNodes(node.children)
}

/** Builds the folder tree of a space from its document paths. Folders come first, then both sort by file name. */
export function buildTree(documents: readonly DriveDocument[]): TreeNode {
  const root: TreeNode = { kind: 'dir', name: '', path: '', label: '', children: [] }
  const dirs = new Map<string, TreeNode>([['', root]])

  for (const doc of documents) {
    const segments = doc.path.split('/').filter(Boolean)
    if (!segments.length) continue
    let parent = root
    for (let i = 0; i < segments.length - 1; i++) {
      const path = segments.slice(0, i + 1).join('/')
      let dir = dirs.get(path)
      if (!dir) {
        dir = { kind: 'dir', name: segments[i], path, label: segments[i], children: [] }
        dirs.set(path, dir)
        parent.children.push(dir)
      }
      parent = dir
    }
    parent.children.push({
      kind: 'doc',
      name: segments[segments.length - 1],
      path: doc.path,
      label: documentLabel(doc),
      doc,
      children: [],
    })
  }
  sortNodes(root.children)
  return root
}

function matches(node: TreeNode, query: string): boolean {
  return node.label.toLowerCase().includes(query) || node.path.toLowerCase().includes(query)
}

/**
 * Keeps the documents whose title or path contains the query, with the folders
 * above them. A folder whose own name matches keeps its whole subtree.
 */
export function filterTree(root: TreeNode, query: string): TreeNode {
  const q = query.trim().toLowerCase()
  if (!q) return root
  const keep = (nodes: TreeNode[]): TreeNode[] =>
    nodes.flatMap((node) => {
      if (node.kind === 'doc') return matches(node, q) ? [node] : []
      if (node.name.toLowerCase().includes(q)) return [node]
      const children = keep(node.children)
      return children.length ? [{ ...node, children }] : []
    })
  return { ...root, children: keep(root.children) }
}

/** The tree as visible rows, descending only into the folders `isOpen` accepts. */
export function flattenTree(root: TreeNode, isOpen: (dirPath: string) => boolean): TreeRow[] {
  const rows: TreeRow[] = []
  const walk = (nodes: TreeNode[], depth: number) => {
    for (const node of nodes) {
      rows.push({ node, depth })
      if (node.kind === 'dir' && isOpen(node.path)) walk(node.children, depth + 1)
    }
  }
  walk(root.children, 0)
  return rows
}

/** The folders only, for a navigation tree. */
export function foldersOnly(root: TreeNode): TreeNode {
  const strip = (nodes: TreeNode[]): TreeNode[] =>
    nodes.filter((node) => node.kind === 'dir').map((node) => ({ ...node, children: strip(node.children) }))
  return { ...root, children: strip(root.children) }
}

/** The node at a folder path ('' is the root), or null when no document lives there. */
export function findFolder(root: TreeNode, folderPath: string): TreeNode | null {
  let node = root
  for (const segment of folderPath.split('/').filter(Boolean)) {
    const next = node.children.find((child) => child.kind === 'dir' && child.name === segment)
    if (!next) return null
    node = next
  }
  return node
}

/** Every document under a node, at any depth. */
export function descendantDocuments(node: TreeNode): DriveDocument[] {
  if (node.kind === 'doc') return node.doc ? [node.doc] : []
  return node.children.flatMap(descendantDocuments)
}

/** The documents whose title or path contains the query, as a flat list in tree order. */
export function matchingDocuments(root: TreeNode, query: string): DriveDocument[] {
  return descendantDocuments(filterTree(root, query))
}

/** The folders above a path, outermost first. */
export function ancestorDirs(path: string): string[] {
  const segments = path.split('/')
  return segments.slice(0, -1).map((_, i) => segments.slice(0, i + 1).join('/'))
}
