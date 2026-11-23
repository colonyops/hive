import { parse } from 'yaml'

export type Validated<T> = { ok: true; data: T } | { ok: false; error: string }

export type TreeStatus = 'added' | 'modified' | 'deleted' | 'renamed'

export interface TreeFile {
  path: string
  status?: TreeStatus
  from?: string
  note?: string
}

export interface TreeData {
  /** Prefix shown as the top folder; paths are relative to it. Default: none. */
  root?: string
  files: TreeFile[]
}

export type CallKind = 'function' | 'method' | 'component' | 'computed' | 'external'

export interface CallData {
  name: string
  /** "path:line", a path, or a package name ("markdown-it"). */
  at?: string
  /** Icon. Default: from `at`. A .vue file with a capitalized name is a component, an `at` with no "/" or "." is an external package, and anything else is a function. */
  kind?: CallKind
  /** One line, shown in the row. */
  note?: string
  /** A few sentences, shown when the row is clicked. Default: note. */
  explain?: string
  focus?: boolean
  /** Calls left out, shown as "… N more". */
  more?: number
  calls?: CallData[]
}

export type ForgeState = 'open' | 'draft' | 'merged' | 'closed'

export interface ForgeItem {
  kind: 'pr' | 'issue'
  /** "owner/repo#123", shown on the card. */
  ref: string
  /** Where a click goes. Default: built from host and ref with GitHub's URL layout. */
  url?: string
  title: string
  state: ForgeState
  author?: string
  checks?: string
  diff?: string
  comments?: number
  labels?: string[]
  updated?: string
}

export interface ForgeData {
  /** When the agent took the data. Shown as "snapshot ..." in the header. */
  snapshot?: string
  /** Default host for items without a url. Default: github.com. */
  host?: string
  items: ForgeItem[]
}

export interface FileNode {
  name: string
  /** Full path from the tree's root, with no trailing slash. */
  path: string
  kind: 'dir' | 'file'
  file?: TreeFile
  children: FileNode[]
}

// The core schema keeps `no`, `on` and `yes` strings and reads `1.10` as the
// number 1.1, which is what an agent writing a version string must expect.
export function parseFenceBody(source: string): Validated<unknown> {
  try {
    return { ok: true, data: parse(source, { schema: 'core', version: '1.2', prettyErrors: false }) }
  } catch (cause) {
    const message = cause instanceof Error ? cause.message : String(cause)
    return { ok: false, error: `Invalid YAML: ${message.split('\n', 1)[0]}` }
  }
}

type Fail = { ok: false; error: string }

const fail = (error: string): Fail => ({ ok: false, error })

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function describe(value: unknown): string {
  if (value === null || value === undefined) return 'empty'
  if (Array.isArray(value)) return 'a list'
  return typeof value === 'object' ? 'a map' : `${typeof value} ${JSON.stringify(value)}`
}

function isScalar(value: unknown): value is string | number | boolean {
  return typeof value === 'string' || typeof value === 'number' || typeof value === 'boolean'
}

function str(value: unknown): string | undefined {
  return isScalar(value) ? String(value) : undefined
}

function oneOf<T extends string>(value: unknown, allowed: readonly T[]): T | undefined {
  return allowed.find((candidate) => candidate === value)
}

const treeStatuses = ['added', 'modified', 'deleted', 'renamed'] as const

export function validateTree(data: unknown): Validated<TreeData> {
  if (!isRecord(data)) return fail(`Expected a map with "files", got ${describe(data)}`)
  if (!Array.isArray(data.files)) return fail('"files" must be a list')
  const files: TreeFile[] = []
  for (const [index, entry] of data.files.entries()) {
    const where = `files[${index}]`
    if (!isRecord(entry)) return fail(`${where} must be a map with "path"`)
    const path = str(entry.path)?.trim()
    if (!path) return fail(`${where} needs a "path"`)
    const file: TreeFile = { path }
    if (entry.status !== undefined) {
      const status = oneOf(entry.status, treeStatuses)
      if (!status) return fail(`${where}.status must be one of ${treeStatuses.join(', ')}`)
      file.status = status
    }
    const from = str(entry.from)
    if (from) file.from = from
    const note = str(entry.note)
    if (note) file.note = note
    files.push(file)
  }
  const tree: TreeData = { files }
  const root = str(data.root)?.trim()
  if (root) tree.root = root
  return { ok: true, data: tree }
}

const callKinds = ['function', 'method', 'component', 'computed', 'external'] as const

function validateCall(value: unknown, where: string): Validated<CallData> {
  if (!isRecord(value)) return fail(`${where} must be a map with "name"`)
  const name = str(value.name)?.trim()
  if (!name) return fail(`${where} needs a "name"`)
  const call: CallData = { name }
  const at = str(value.at)
  if (at) call.at = at
  if (value.kind !== undefined) {
    const kind = oneOf(value.kind, callKinds)
    if (!kind) return fail(`${where}.kind must be one of ${callKinds.join(', ')}`)
    call.kind = kind
  }
  const note = str(value.note)
  if (note) call.note = note
  const explain = str(value.explain)?.trim()
  if (explain) call.explain = explain
  if (value.focus !== undefined) {
    if (typeof value.focus !== 'boolean') return fail(`${where}.focus must be true or false`)
    if (value.focus) call.focus = true
  }
  if (value.more !== undefined) {
    if (typeof value.more !== 'number' || !Number.isInteger(value.more) || value.more < 0)
      return fail(`${where}.more must be a whole number`)
    if (value.more) call.more = value.more
  }
  if (value.calls !== undefined) {
    if (!Array.isArray(value.calls)) return fail(`${where}.calls must be a list`)
    const calls: CallData[] = []
    for (const [index, child] of value.calls.entries()) {
      const checked = validateCall(child, `${where}.calls[${index}]`)
      if (!checked.ok) return checked
      calls.push(checked.data)
    }
    if (calls.length) call.calls = calls
  }
  return { ok: true, data: call }
}

export function validateCalls(data: unknown): Validated<CallData> {
  if (!isRecord(data)) return fail(`Expected a call with "name", got ${describe(data)}`)
  return validateCall(data, 'call')
}

const forgeStates = ['open', 'draft', 'merged', 'closed'] as const

export function validateForge(data: unknown): Validated<ForgeData> {
  if (!isRecord(data)) return fail(`Expected a map with "items", got ${describe(data)}`)
  if (!Array.isArray(data.items)) return fail('"items" must be a list')
  const items: ForgeItem[] = []
  for (const [index, entry] of data.items.entries()) {
    const where = `items[${index}]`
    if (!isRecord(entry)) return fail(`${where} must be a map`)
    const kind = oneOf(entry.kind, ['pr', 'issue'] as const)
    if (!kind) return fail(`${where}.kind must be "pr" or "issue"`)
    const ref = str(entry.ref)?.trim()
    if (!ref) return fail(`${where} needs a "ref" such as "owner/repo#123"`)
    const title = str(entry.title)?.trim()
    if (!title) return fail(`${where} needs a "title"`)
    const state = oneOf(entry.state, forgeStates)
    if (!state) return fail(`${where}.state must be one of ${forgeStates.join(', ')}`)
    const item: ForgeItem = { kind, ref, title, state }
    for (const key of ['url', 'author', 'checks', 'diff', 'updated'] as const) {
      const value = str(entry[key])
      if (value) item[key] = value
    }
    if (entry.comments !== undefined) {
      if (typeof entry.comments !== 'number' || !Number.isFinite(entry.comments))
        return fail(`${where}.comments must be a number`)
      item.comments = entry.comments
    }
    if (entry.labels !== undefined) {
      if (!Array.isArray(entry.labels) || !entry.labels.every(isScalar))
        return fail(`${where}.labels must be a list of strings`)
      item.labels = entry.labels.map(String)
    }
    items.push(item)
  }
  const forge: ForgeData = { items }
  const snapshot = str(data.snapshot)
  if (snapshot) forge.snapshot = snapshot
  const host = str(data.host)?.trim()
  if (host) forge.host = host
  return { ok: true, data: forge }
}

function trimSlashes(path: string): string {
  return path.replace(/^\/+|\/+$/g, '')
}

export function buildFileTree(data: TreeData): FileNode[] {
  const top: FileNode[] = []
  const root = data.root ? trimSlashes(data.root) : ''
  let rootNode: FileNode | null = null
  if (root) {
    rootNode = { name: root, path: root, kind: 'dir', children: [] }
    top.push(rootNode)
  }
  const base = rootNode?.children ?? top
  const basePath = root ? `${root}/` : ''

  for (const file of data.files) {
    const segments = trimSlashes(file.path).split('/').filter(Boolean)
    if (!segments.length) continue
    let siblings = base
    let prefix = basePath
    for (const [index, segment] of segments.entries()) {
      const path = `${prefix}${segment}`
      const leaf = index === segments.length - 1
      let node = siblings.find((candidate) => candidate.name === segment && candidate.kind === (leaf ? 'file' : 'dir'))
      if (!node) {
        node = leaf
          ? { name: segment, path, kind: 'file', file, children: [] }
          : { name: segment, path, kind: 'dir', children: [] }
        siblings.push(node)
      }
      siblings = node.children
      prefix = `${path}/`
    }
  }
  return top
}

function atPath(at: string): string {
  return at.replace(/:\d+(?::\d+)?$/, '')
}

export function callKind(call: CallData): CallKind {
  if (call.kind) return call.kind
  const at = call.at ? atPath(call.at.trim()) : ''
  if (!at) return 'function'
  if (at.endsWith('.vue') && /^[A-Z]/.test(call.name)) return 'component'
  if (!at.includes('/') && !at.includes('.')) return 'external'
  return 'function'
}
