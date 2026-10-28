import type { Component } from 'vue'
import IconFile from '~icons/lucide/file'
import IconFileCode from '~icons/lucide/file-code'
import IconFileCog from '~icons/lucide/file-cog'
import IconFileImage from '~icons/lucide/file-image'
import IconFileJson from '~icons/lucide/file-json'
import IconFileSpreadsheet from '~icons/lucide/file-spreadsheet'
import IconFileTerminal from '~icons/lucide/file-terminal'
import IconFileText from '~icons/lucide/file-text'
import IconFileType from '~icons/lucide/file-type'
import IconFolder from '~icons/lucide/folder'
import IconFolderArchive from '~icons/lucide/folder-archive'
import IconFolderCode from '~icons/lucide/folder-code'
import IconFolderCog from '~icons/lucide/folder-cog'
import IconFolderKanban from '~icons/lucide/folder-kanban'
import IconFolderOpen from '~icons/lucide/folder-open'
import IconFolderPen from '~icons/lucide/folder-pen'
import IconFolderSearch from '~icons/lucide/folder-search'
import IconLayoutDashboard from '~icons/lucide/layout-dashboard'

/** An icon and the text colour class that tints it, so a row reads by its type at a glance. */
export interface DriveIcon {
  icon: Component
  color: string
}

const kinds: Record<string, DriveIcon> = {
  markdown: { icon: IconFileText, color: 'text-node-blue' },
  html: { icon: IconFileCode, color: 'text-node-orange' },
  canvas: { icon: IconLayoutDashboard, color: 'text-node-purple' },
}

const json: DriveIcon = { icon: IconFileJson, color: 'text-severity-warning' }
const config: DriveIcon = { icon: IconFileCog, color: 'text-severity-error' }
const image: DriveIcon = { icon: IconFileImage, color: 'text-node-teal' }
const sheet: DriveIcon = { icon: IconFileSpreadsheet, color: 'text-node-green' }
const script: DriveIcon = { icon: IconFileTerminal, color: 'text-node-green' }

const extensions: Record<string, DriveIcon> = {
  json,
  jsonl: json,
  yaml: config,
  yml: config,
  toml: config,
  png: image,
  jpg: image,
  jpeg: image,
  gif: image,
  webp: image,
  svg: image,
  pdf: { icon: IconFileType, color: 'text-severity-error' },
  csv: sheet,
  tsv: sheet,
  sh: script,
  bash: script,
  zsh: script,
}

const plainFile: DriveIcon = { icon: IconFile, color: 'text-text-4' }

/** A document's icon: its kind first, then its extension. */
export function documentIcon(name: string, kind: string): DriveIcon {
  const known = kinds[kind]
  if (known) return known
  const dot = name.lastIndexOf('.')
  return (dot > 0 && extensions[name.slice(dot + 1).toLowerCase()]) || plainFile
}

// Folders stay one muted colour so the documents' colours stand out; the
// names the drive's skills use most get their own shape.
const folderColor = 'text-text-4'
const planning: DriveIcon = { icon: IconFolderKanban, color: folderColor }
const reading: DriveIcon = { icon: IconFolderSearch, color: folderColor }
const drawing: DriveIcon = { icon: IconFolderPen, color: folderColor }
const code: DriveIcon = { icon: IconFolderCode, color: folderColor }

const folders: Record<string, DriveIcon> = {
  plans: planning,
  tasks: planning,
  todos: planning,
  research: reading,
  references: reading,
  docs: reading,
  'design-docs': drawing,
  designs: drawing,
  canvases: drawing,
  scripts: code,
  src: code,
  archive: { icon: IconFolderArchive, color: folderColor },
  config: { icon: IconFolderCog, color: folderColor },
}

/** A folder's icon: a named folder keeps its own, any other opens and closes. */
export function folderIcon(name: string, open: boolean): DriveIcon {
  return folders[name.toLowerCase()] ?? { icon: open ? IconFolderOpen : IconFolder, color: folderColor }
}
