import type { Component } from 'vue'
import IconContainer from '~icons/lucide/container'
import IconFile from '~icons/lucide/file'
import IconFileArchive from '~icons/lucide/file-archive'
import IconFileAudio from '~icons/lucide/file-audio'
import IconFileBadge from '~icons/lucide/file-badge'
import IconFileCode from '~icons/lucide/file-code'
import IconFileCog from '~icons/lucide/file-cog'
import IconFileDiff from '~icons/lucide/file-diff'
import IconFileImage from '~icons/lucide/file-image'
import IconFileJson from '~icons/lucide/file-json'
import IconFileKey from '~icons/lucide/file-key'
import IconFileLock from '~icons/lucide/file-lock'
import IconFileSpreadsheet from '~icons/lucide/file-spreadsheet'
import IconFileTerminal from '~icons/lucide/file-terminal'
import IconFileText from '~icons/lucide/file-text'
import IconFileType from '~icons/lucide/file-type'
import IconFileVideo from '~icons/lucide/file-video'
import IconFolder from '~icons/lucide/folder'
import IconFolderArchive from '~icons/lucide/folder-archive'
import IconFolderCheck from '~icons/lucide/folder-check'
import IconFolderCode from '~icons/lucide/folder-code'
import IconFolderCog from '~icons/lucide/folder-cog'
import IconFolderGit2 from '~icons/lucide/folder-git-2'
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
const prose: DriveIcon = { icon: IconFileText, color: 'text-node-blue' }
const text: DriveIcon = { icon: IconFileText, color: 'text-text-3' }
const archive: DriveIcon = { icon: IconFileArchive, color: 'text-severity-warning' }
const video: DriveIcon = { icon: IconFileVideo, color: 'text-node-purple' }
const audio: DriveIcon = { icon: IconFileAudio, color: 'text-node-purple' }
const font: DriveIcon = { icon: IconFileType, color: 'text-text-3' }
const lock: DriveIcon = { icon: IconFileLock, color: 'text-text-4' }
const secret: DriveIcon = { icon: IconFileKey, color: 'text-severity-warning' }
const code = (color: string): DriveIcon => ({ icon: IconFileCode, color })
const goCode = code('text-node-teal')
const blueCode = code('text-node-blue')
const yellowCode = code('text-severity-warning')
const greenCode = code('text-node-green')
const orangeCode = code('text-node-orange')
const purpleCode = code('text-node-purple')
const redCode = code('text-severity-error')

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
  fish: script,
  ps1: script,
  go: goCode,
  ts: blueCode,
  tsx: blueCode,
  mts: blueCode,
  cts: blueCode,
  js: yellowCode,
  jsx: yellowCode,
  mjs: yellowCode,
  cjs: yellowCode,
  vue: greenCode,
  svelte: orangeCode,
  py: blueCode,
  rs: orangeCode,
  rb: redCode,
  java: redCode,
  kt: purpleCode,
  swift: orangeCode,
  c: blueCode,
  h: blueCode,
  cc: blueCode,
  cpp: blueCode,
  hpp: blueCode,
  cs: purpleCode,
  php: purpleCode,
  lua: blueCode,
  zig: orangeCode,
  ex: purpleCode,
  exs: purpleCode,
  html: orangeCode,
  htm: orangeCode,
  xml: orangeCode,
  css: purpleCode,
  scss: purpleCode,
  less: purpleCode,
  sql: goCode,
  proto: purpleCode,
  graphql: purpleCode,
  tf: purpleCode,
  md: prose,
  mdx: prose,
  markdown: prose,
  rst: prose,
  txt: text,
  log: text,
  diff: { icon: IconFileDiff, color: 'text-node-green' },
  patch: { icon: IconFileDiff, color: 'text-node-green' },
  ini: config,
  conf: config,
  cfg: config,
  lock,
  sum: lock,
  zip: archive,
  tar: archive,
  gz: archive,
  tgz: archive,
  bz2: archive,
  xz: archive,
  ico: image,
  avif: image,
  mp4: video,
  mov: video,
  webm: video,
  mp3: audio,
  wav: audio,
  ogg: audio,
  woff: font,
  woff2: font,
  ttf: font,
  otf: font,
  pem: secret,
  key: secret,
}

// Files known by their whole name, most of which have no extension to go by.
const fileNames: Record<string, DriveIcon> = {
  dockerfile: { icon: IconContainer, color: 'text-node-blue' },
  'compose.yaml': { icon: IconContainer, color: 'text-node-blue' },
  'docker-compose.yml': { icon: IconContainer, color: 'text-node-blue' },
  makefile: config,
  justfile: config,
  taskfile: config,
  '.gitignore': config,
  '.gitattributes': config,
  '.dockerignore': config,
  '.editorconfig': config,
  '.npmrc': config,
  '.env': secret,
  'go.mod': goCode,
  'package-lock.json': lock,
  'pnpm-lock.yaml': lock,
  license: { icon: IconFileBadge, color: 'text-severity-warning' },
  'license.md': { icon: IconFileBadge, color: 'text-severity-warning' },
  readme: prose,
}

const plainFile: DriveIcon = { icon: IconFile, color: 'text-text-4' }

/** A file's icon: its kind first, then its whole name, then its extension. */
export function documentIcon(name: string, kind: string): DriveIcon {
  const known = kinds[kind]
  if (known) return known
  const lower = name.toLowerCase()
  const named = fileNames[lower]
  if (named) return named
  const dot = lower.lastIndexOf('.')
  return (dot > 0 && extensions[lower.slice(dot + 1)]) || plainFile
}

// Folders stay one muted colour so the documents' colours stand out; the
// names the drive's skills use most get their own shape.
const folderColor = 'text-text-4'
const planning: DriveIcon = { icon: IconFolderKanban, color: folderColor }
const reading: DriveIcon = { icon: IconFolderSearch, color: folderColor }
const drawing: DriveIcon = { icon: IconFolderPen, color: folderColor }
const codeFolder: DriveIcon = { icon: IconFolderCode, color: folderColor }
const testing: DriveIcon = { icon: IconFolderCheck, color: folderColor }

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
  scripts: codeFolder,
  src: codeFolder,
  test: testing,
  tests: testing,
  __tests__: testing,
  e2e: testing,
  '.git': { icon: IconFolderGit2, color: folderColor },
  '.github': { icon: IconFolderGit2, color: folderColor },
  archive: { icon: IconFolderArchive, color: folderColor },
  config: { icon: IconFolderCog, color: folderColor },
}

/** A folder's icon: a named folder keeps its own, any other opens and closes. */
export function folderIcon(name: string, open: boolean): DriveIcon {
  return folders[name.toLowerCase()] ?? { icon: open ? IconFolderOpen : IconFolder, color: folderColor }
}
