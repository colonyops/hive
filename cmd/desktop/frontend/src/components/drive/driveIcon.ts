import { h, type Component, type FunctionalComponent } from 'vue'
import { aliases, icons } from '@iconify-json/material-icon-theme/icons.json'
import {
  fileExtensions,
  fileNames,
  folderNames,
  folderNamesExpanded,
} from 'material-icon-theme/dist/material-icons.json'

interface IconifyIcon {
  body: string
  left?: number
  top?: number
  width?: number
  height?: number
}

const iconSet = icons as Record<string, IconifyIcon>
const aliasSet = aliases as Record<string, { parent: string }>

// The theme matches names case-insensitively; its keys are not all lowercase.
function lowercaseKeys(map: Record<string, string>): Map<string, string> {
  return new Map(Object.entries(map).map(([key, icon]) => [key.toLowerCase(), icon]))
}

const byFileName = lowercaseKeys(fileNames)
const byExtension = lowercaseKeys(fileExtensions)
const byFolder = lowercaseKeys(folderNames)
const byFolderOpen = lowercaseKeys(folderNamesExpanded)

// The Iconify collection trails the theme, so a variant the theme maps
// (react_ts, go_gopher) can be missing; its base icon (react, go) stands in.
function resolve(name: string): IconifyIcon | undefined {
  for (const candidate of [name, name.split('_')[0]]) {
    const icon = iconSet[candidate] ?? iconSet[aliasSet[candidate]?.parent ?? '']
    if (icon) return icon
  }
  return undefined
}

const components = new Map<string, Component>()

function component(name: string): Component | undefined {
  const cached = components.get(name)
  if (cached) return cached
  const icon = resolve(name)
  if (!icon) return undefined
  const viewBox = `${icon.left ?? 0} ${icon.top ?? 0} ${icon.width ?? 16} ${icon.height ?? 16}`
  const rendered: FunctionalComponent = () =>
    h('svg', { xmlns: 'http://www.w3.org/2000/svg', viewBox, width: '1em', height: '1em', innerHTML: icon.body })
  rendered.displayName = `MaterialIcon(${name})`
  components.set(name, rendered)
  return rendered
}

function required(name: string): Component {
  const icon = component(name)
  if (!icon) throw new Error(`material-icon-theme has no ${name} icon`)
  return icon
}

// Iconify renames the theme's defaults: its `file` is `document` and its
// `folder` is `folder-base`.
const plainFile = required('document')
const plainFolder = required('folder-base')
const plainFolderOpen = required('folder-base-open')

// Drive document kinds, and the folder names the drive's skills use most,
// borrow the icon of what the theme already knows them as.
const kindExtensions: Record<string, string> = { markdown: 'md', html: 'html' }
const folderAliases: Record<string, string> = {
  plans: 'tasks',
  todos: 'tasks',
  research: 'docs',
  references: 'docs',
  'design-docs': 'design',
}

// Every suffix after a dot, longest first, so `foo.spec.ts` tries `spec.ts`
// before `ts` and `.env` tries `env`.
function extensionsOf(name: string): string[] {
  const suffixes: string[] = []
  for (let dot = name.indexOf('.'); dot >= 0; dot = name.indexOf('.', dot + 1)) suffixes.push(name.slice(dot + 1))
  return suffixes
}

/** A file's icon: its drive kind first, then its whole name, then its longest known extension. */
export function documentIcon(name: string, kind: string): Component {
  const kindExtension = kindExtensions[kind]
  if (kindExtension) return component(byExtension.get(kindExtension) ?? '') ?? plainFile
  const lower = name.toLowerCase()
  const named = byFileName.get(lower)
  if (named) return component(named) ?? plainFile
  for (const extension of extensionsOf(lower)) {
    const icon = byExtension.get(extension)
    if (icon) return component(icon) ?? plainFile
  }
  return plainFile
}

/** A folder's icon, open or closed: the theme's icon for its name, or a plain folder. */
export function folderIcon(name: string, open: boolean): Component {
  const lower = name.toLowerCase()
  const key = folderAliases[lower] ?? lower
  const icon = (open ? byFolderOpen : byFolder).get(key)
  return (icon && component(icon)) || (open ? plainFolderOpen : plainFolder)
}
