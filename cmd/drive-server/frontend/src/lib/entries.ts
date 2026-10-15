import type { DriveDocument } from '../types'
import { documentRoute, spaceFromKey, type SpaceRef } from './space'
import { buildTree, descendantDocuments, documentLabel, findFolder, type TreeNode } from './tree'

/** One row or tile in a listing: a folder, a document, or a space. */
export interface ListEntry {
  kind: 'dir' | 'doc'
  key: string
  label: string
  /** Row tooltip: the file name of a document or what a folder holds. */
  secondary: string
  /** Space and folder of a document in a cross-space or search listing; null in a folder listing. */
  location: string | null
  path: string
  space: SpaceRef
  doc?: DriveDocument
  /** ISO time, or '' when there is none. */
  modified: string
  size: number | null
  shared: boolean
}

export function entryRoute(entry: ListEntry): string {
  return entry.path ? documentRoute(entry.space, entry.path) : entry.space.routePrefix
}

function plural(count: number): string {
  return `${count} ${count === 1 ? 'document' : 'documents'}`
}

function fileEntry(doc: DriveDocument, secondary: string, location: string | null = null): ListEntry {
  return {
    kind: 'doc',
    key: `${doc.space}\n${doc.path}`,
    label: documentLabel(doc),
    secondary,
    location,
    path: doc.path,
    space: spaceFromKey(doc.space),
    doc,
    modified: doc.updated_at,
    size: doc.size,
    shared: !!doc.shared,
  }
}

function folderEntry(node: TreeNode, space: SpaceRef): ListEntry {
  const docs = descendantDocuments(node)
  return {
    kind: 'dir',
    key: `${space.key}\n${node.path}/`,
    label: node.name,
    secondary: plural(docs.length),
    location: null,
    path: node.path,
    space,
    modified: docs.reduce((newest, doc) => (doc.updated_at > newest ? doc.updated_at : newest), ''),
    size: docs.reduce((total, doc) => total + doc.size, 0),
    shared: false,
  }
}

/** What a folder holds: subfolders first, then documents. Null when no document lives under the path. */
export function folderContents(
  documents: readonly DriveDocument[],
  space: SpaceRef,
  folderPath: string,
): ListEntry[] | null {
  const folder = findFolder(buildTree(documents), folderPath)
  if (!folder) return null
  return folder.children.map((node) => (node.doc ? fileEntry(node.doc, node.name) : folderEntry(node, space)))
}

/** Documents as a flat listing whose Location column names the space and folder. */
export function documentEntries(documents: readonly DriveDocument[]): ListEntry[] {
  return documents.map((doc) => {
    const folder = doc.path.includes('/') ? doc.path.slice(0, doc.path.lastIndexOf('/')) : ''
    const title = spaceFromKey(doc.space).title
    return fileEntry(doc, doc.path, folder ? `${title} / ${folder}` : title)
  })
}
