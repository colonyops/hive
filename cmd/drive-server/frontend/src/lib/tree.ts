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

/** Folders come first, then both sort by file name. */
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

export function descendantDocuments(node: TreeNode): DriveDocument[] {
  if (node.kind === 'doc') return node.doc ? [node.doc] : []
  return node.children.flatMap(descendantDocuments)
}

/** The folders above a path, outermost first. */
export function ancestorDirs(path: string): string[] {
  const segments = path.split('/')
  return segments.slice(0, -1).map((_, i) => segments.slice(0, i + 1).join('/'))
}
