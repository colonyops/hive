import type {
  ChangedFile,
  SessionFileDiff,
  SessionFileImage,
  SessionImageVersion,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'

// Formats a WebView draws in an <img>. SVG is text, so it keeps its text diff
// and gets the preview above it.
const IMAGE_EXTENSIONS = new Set(['png', 'jpg', 'jpeg', 'gif', 'webp', 'avif', 'bmp', 'ico', 'svg'])

export function isImagePath(path: string): boolean {
  const dot = path.lastIndexOf('.')
  return dot >= 0 && IMAGE_EXTENSIONS.has(path.slice(dot + 1).toLowerCase())
}

export type ImagePreviewKind = 'binary' | 'text'

/**
 * How a file's image preview sits in the stream: alone, in place of a diff
 * git could not draw, or above the text diff of a format such as SVG. Null
 * for a file that gets no preview.
 */
export function imagePreviewKind(file: ChangedFile, diff: SessionFileDiff | undefined): ImagePreviewKind | null {
  if (!diff || !isImagePath(file.path)) return null
  if (diff.state === 'binary') return 'binary'
  if (diff.state === 'text') return 'text'
  return null
}

/**
 * The placeholder patch an image's preview hangs from. CodeView sizes an item
 * from its rows and measures only annotations, so the preview is an
 * annotation on this one row, and the row itself is hidden by CSS.
 */
export function imagePlaceholderPatch(file: ChangedFile): string {
  if (file.status === 'added' || file.status === 'untracked') return '@@ -0,0 +1 @@\n+\n'
  if (file.status === 'deleted') return '@@ -1 +0,0 @@\n-\n'
  return '@@ -1 +1 @@\n \n'
}

export function imageSrc(version: SessionImageVersion): string | null {
  return version.mime ? `data:${version.mime};base64,${version.data}` : null
}

/** The annotation id of a file's image preview, apart from every comment id. */
export function imagePreviewId(path: string): string {
  return `image:${path}`
}

export interface ImageReads {
  /** The held read of path at version, or a new one. */
  read(path: string, version: string): Promise<SessionFileImage>
  /** Drops every held read whose path keep rejects. */
  prune(keep: (path: string) => boolean): void
}

/**
 * Image reads held per path and version, so a preview CodeView mounts again
 * does not read the same bytes twice. A failed read is not held, so the next
 * mount tries again.
 */
export function imageReads(load: (path: string) => Promise<SessionFileImage>): ImageReads {
  const held = new Map<string, { version: string; read: Promise<SessionFileImage> }>()
  return {
    read(path, version) {
      const current = held.get(path)
      if (current && current.version === version) return current.read
      const read = load(path)
      held.set(path, { version, read })
      read.catch(() => {
        if (held.get(path)?.read === read) held.delete(path)
      })
      return read
    },
    prune(keep) {
      for (const path of held.keys()) if (!keep(path)) held.delete(path)
    },
  }
}

export type ImageBackdrop = 'light' | 'dark'

// Under this share of see-through pixels the backdrop only shows in the margins,
// so the app's own theme decides it.
const SEE_THROUGH_SHARE = 0.01
const LIGHT_CONTENT = 0.6

/**
 * The backdrop an image's visible pixels stand out on: dark behind light
 * content such as a white icon, light behind anything else. Null for an
 * opaque or empty image, where no backdrop shows through.
 */
export function backdropForPixels(rgba: Uint8ClampedArray): ImageBackdrop | null {
  let seeThrough = 0
  let weight = 0
  let luminance = 0
  for (let i = 0; i + 3 < rgba.length; i += 4) {
    const alpha = rgba[i + 3] / 255
    if (alpha < 0.98) seeThrough++
    if (alpha === 0) continue
    weight += alpha
    luminance += alpha * ((0.2126 * rgba[i] + 0.7152 * rgba[i + 1] + 0.0722 * rgba[i + 2]) / 255)
  }
  const pixels = rgba.length / 4
  if (!pixels || !weight || seeThrough / pixels < SEE_THROUGH_SHARE) return null
  return luminance / weight > LIGHT_CONTENT ? 'dark' : 'light'
}
