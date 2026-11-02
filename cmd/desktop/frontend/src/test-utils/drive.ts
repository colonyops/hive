// Builders for Hive Drive wire values, so a spec states a tree as the paths
// it holds instead of restating every EntryWire field.
import type {
  CanvasWire,
  DocumentWire,
  EntryWire,
  SpaceWire,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'

function kindOf(path: string): string {
  if (path.endsWith('.canvas.json')) return 'canvas'
  if (path.endsWith('.md')) return 'markdown'
  if (path.endsWith('.html')) return 'html'
  return 'file'
}

/** A document entry; the kind follows the extension, as drive.KindOf does. */
export function doc(path: string, overrides: Partial<EntryWire> = {}): EntryWire {
  return {
    path,
    name: path.split('/').pop() ?? path,
    kind: kindOf(path),
    size: 10,
    modified: '2026-10-01T00:00:00Z',
    created: '2026-10-01T00:00:00Z',
    title: '',
    front: {},
    children: [],
    ...overrides,
  }
}

export function folder(path: string, children: EntryWire[]): EntryWire {
  return { ...doc(path), kind: 'dir', children }
}

export function root(children: EntryWire[]): EntryWire {
  return { ...folder('', children), name: '' }
}

export function space(key: string, name = key): SpaceWire {
  const kind = key === 'shared' ? 'shared' : key.startsWith('workspaces/') ? 'workspace' : 'repository'
  return { kind, key, name }
}

export function documentFor(
  spaceKey: string,
  entry: EntryWire,
  body: { markdown?: string; html?: string; image?: string; canvas?: Partial<CanvasWire> } = {},
): DocumentWire {
  return {
    space: space(spaceKey),
    entry,
    markdown: body.markdown ?? '',
    html: body.html ?? '',
    image: body.image ?? '',
    canvas: body.canvas
      ? {
          space: spaceKey,
          path: entry.path,
          title: '',
          session: '',
          hiveSession: '',
          createdAt: 1,
          updatedAt: 2,
          frontmatter: {},
          blocks: [],
          ...body.canvas,
        }
      : null,
  }
}
