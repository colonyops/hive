import { describe, expect, it } from 'vitest'
import {
  buildFileTree,
  callKind,
  parseFenceBody,
  validateCalls,
  validateForge,
  validateTree,
  type CallData,
} from '../fenceData'

describe('parseFenceBody', () => {
  it.each([
    ['maps', 'a: 1\nb: [x, y]', { a: 1, b: ['x', 'y'] }],
    ['no stays a string', 'answer: no', { answer: 'no' }],
    ['on stays a string', 'mode: on', { mode: 'on' }],
    ['1.10 reads as the number 1.1', 'v: 1.10', { v: 1.1 }],
    ['a quoted 1.10 stays text', 'v: "1.10"', { v: '1.10' }],
    ['a timestamp-like value stays a string', 'at: 2026-11-22T12:30', { at: '2026-11-22T12:30' }],
    ['an empty body is null', '', null],
  ])('%s', (_name, source, expected) => {
    expect(parseFenceBody(source)).toEqual({ ok: true, data: expected })
  })

  it('reports a YAML error on one line', () => {
    const result = parseFenceBody('a: [1, 2\nb: 3')
    expect(result.ok).toBe(false)
    if (!result.ok) {
      expect(result.error).toMatch(/^Invalid YAML: /)
      expect(result.error).not.toContain('\n')
    }
  })

  it('rejects duplicate keys', () => {
    expect(parseFenceBody('a: 1\na: 2').ok).toBe(false)
  })
})

describe('validateTree', () => {
  it('keeps known fields and drops unknown ones', () => {
    expect(
      validateTree({
        root: 'src',
        extra: true,
        files: [
          { path: 'a.ts', status: 'modified', note: 'n', other: 1 },
          { path: 'b.ts', status: 'renamed', from: 'c.ts' },
        ],
      }),
    ).toEqual({
      ok: true,
      data: {
        root: 'src',
        files: [
          { path: 'a.ts', status: 'modified', note: 'n' },
          { path: 'b.ts', status: 'renamed', from: 'c.ts' },
        ],
      },
    })
  })

  it.each([
    ['no files', {}, /files/],
    ['a file without a path', { files: [{ status: 'added' }] }, /files\[0\].*path/],
    ['a bad status', { files: [{ path: 'a', status: 'changed' }] }, /status/],
    ['a non-map file', { files: ['a'] }, /files\[0\]/],
  ])('rejects %s', (_name, input, message) => {
    const result = validateTree(input)
    expect(result.ok).toBe(false)
    if (!result.ok) expect(result.error).toMatch(message)
  })
})

describe('buildFileTree', () => {
  const names = (nodes: ReturnType<typeof buildFileTree>): unknown[] =>
    nodes.map((node) => (node.kind === 'dir' ? { [node.name]: names(node.children) } : node.name))

  it('builds folders from flat paths and shares parents', () => {
    const tree = buildFileTree({
      files: [{ path: 'lib/a.ts' }, { path: 'lib/b.ts' }, { path: 'components/fences/c.vue' }, { path: 'top.ts' }],
    })
    expect(names(tree)).toEqual([{ lib: ['a.ts', 'b.ts'] }, { components: [{ fences: ['c.vue'] }] }, 'top.ts'])
  })

  it('puts everything under the root folder when root is set', () => {
    const tree = buildFileTree({ root: 'web/src/', files: [{ path: 'lib/a.ts' }] })
    expect(names(tree)).toEqual([{ 'web/src': [{ lib: ['a.ts'] }] }])
    expect(tree[0].children[0].children[0].path).toBe('web/src/lib/a.ts')
  })

  it('attaches the file entry to its leaf', () => {
    const file = { path: 'lib/a.ts', status: 'added' as const }
    expect(buildFileTree({ files: [file] })[0].children[0].file).toEqual(file)
  })

  it('skips an empty path', () => {
    expect(buildFileTree({ files: [{ path: '/' }] })).toEqual([])
  })
})

describe('validateCalls', () => {
  it('validates nested calls', () => {
    const result = validateCalls({
      name: 'a',
      at: 'x.ts:1',
      kind: 'computed',
      more: 2,
      focus: true,
      calls: [{ name: 'b', explain: ' why ', unknown: 1 }],
    })
    expect(result).toEqual({
      ok: true,
      data: { name: 'a', at: 'x.ts:1', kind: 'computed', more: 2, focus: true, calls: [{ name: 'b', explain: 'why' }] },
    })
  })

  it.each([
    ['a list', [], /name/],
    ['no name', { at: 'x' }, /name/],
    ['a bad kind', { name: 'a', kind: 'class' }, /kind/],
    ['a non-boolean focus', { name: 'a', focus: 'yes' }, /focus/],
    ['a negative more', { name: 'a', more: -1 }, /more/],
    ['calls that is not a list', { name: 'a', calls: 'b' }, /calls/],
    ['a nested error with its path', { name: 'a', calls: [{ name: 'b', calls: [{}] }] }, /calls\[0\]\.calls\[0\]/],
  ])('rejects %s', (_name, input, message) => {
    const result = validateCalls(input)
    expect(result.ok).toBe(false)
    if (!result.ok) expect(result.error).toMatch(message)
  })
})

describe('callKind', () => {
  it.each<[string, CallData, string]>([
    ['an explicit kind wins', { name: 'x', at: 'a/b.ts', kind: 'method' }, 'method'],
    ['a capitalized name in a .vue file', { name: 'CanvasMarkdown', at: 'a/CanvasMarkdown.vue:3' }, 'component'],
    ['a lowercase name in a .vue file', { name: 'setup', at: 'a/B.vue:3' }, 'function'],
    ['a package name', { name: 'MarkdownIt', at: 'markdown-it' }, 'external'],
    ['a path with a slash', { name: 'f', at: 'lib/a.ts:9' }, 'function'],
    ['a bare file name', { name: 'f', at: 'a.ts' }, 'function'],
    ['no at', { name: 'f' }, 'function'],
  ])('%s', (_name, call, expected) => {
    expect(callKind(call)).toBe(expected)
  })
})

describe('validateForge', () => {
  const item = { kind: 'pr', ref: 'a/b#1', title: 'T', state: 'open' }

  it('keeps known fields, coerces scalars to strings and drops unknown keys', () => {
    expect(
      validateForge({
        snapshot: '2026-11-22T12:30',
        host: 'gitea.example.com',
        items: [{ ...item, url: 'https://x', checks: 14, comments: 3, labels: ['bug', 2], extra: 1 }],
      }),
    ).toEqual({
      ok: true,
      data: {
        snapshot: '2026-11-22T12:30',
        host: 'gitea.example.com',
        items: [{ ...item, url: 'https://x', checks: '14', comments: 3, labels: ['bug', '2'] }],
      },
    })
  })

  it.each([
    ['no items', {}, /items/],
    ['a bad kind', { items: [{ ...item, kind: 'mr' }] }, /items\[0\]\.kind/],
    ['no ref', { items: [{ ...item, ref: '' }] }, /ref/],
    ['no title', { items: [{ ...item, title: undefined }] }, /title/],
    ['a bad state', { items: [{ ...item, state: 'wip' }] }, /state/],
    ['non-numeric comments', { items: [{ ...item, comments: 'many' }] }, /comments/],
    ['labels that is not a list', { items: [{ ...item, labels: 'bug' }] }, /labels/],
  ])('rejects %s', (_name, input, message) => {
    const result = validateForge(input)
    expect(result.ok).toBe(false)
    if (!result.ok) expect(result.error).toMatch(message)
  })
})
