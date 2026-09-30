// The scoped set of glyphs a pop-up terminal launcher may show in the command
// palette. This is the single frontend source of truth: the action editor's
// icon dropdown and the palette row both read from here, and it must stay in
// sync with the Go allow-list in internal/app/icons (launcher).
//
// Icon components are imported statically because `~icons/lucide/<name>` is a
// build-time virtual module — the path can't be constructed dynamically — so
// each supported key maps to an eagerly imported component below.
import type { Component } from 'vue'
import IconActivity from '~icons/lucide/activity'
import IconBug from '~icons/lucide/bug'
import IconCloud from '~icons/lucide/cloud'
import IconContainer from '~icons/lucide/container'
import IconDatabase from '~icons/lucide/database'
import IconFileText from '~icons/lucide/file-text'
import IconFlaskConical from '~icons/lucide/flask-conical'
import IconFolder from '~icons/lucide/folder'
import IconGauge from '~icons/lucide/gauge'
import IconGitBranch from '~icons/lucide/git-branch'
import IconGitCompare from '~icons/lucide/git-compare'
import IconHammer from '~icons/lucide/hammer'
import IconPackage from '~icons/lucide/package'
import IconSearch from '~icons/lucide/search'
import IconTerminal from '~icons/lucide/terminal'
import IconZap from '~icons/lucide/zap'

export interface LauncherIconOption {
  value: string
  label: string
  component: Component
}

// Order here is the order shown in the editor dropdown.
export const launcherIconOptions: LauncherIconOption[] = [
  { value: 'terminal', label: 'Terminal', component: IconTerminal },
  { value: 'git-branch', label: 'Git', component: IconGitBranch },
  { value: 'git-compare', label: 'Diff', component: IconGitCompare },
  { value: 'folder', label: 'Files', component: IconFolder },
  { value: 'file-text', label: 'Editor', component: IconFileText },
  { value: 'search', label: 'Search', component: IconSearch },
  { value: 'database', label: 'Database', component: IconDatabase },
  { value: 'gauge', label: 'Monitor', component: IconGauge },
  { value: 'activity', label: 'Logs', component: IconActivity },
  { value: 'flask-conical', label: 'Tests', component: IconFlaskConical },
  { value: 'hammer', label: 'Build', component: IconHammer },
  { value: 'container', label: 'Containers', component: IconContainer },
  { value: 'cloud', label: 'Cloud', component: IconCloud },
  { value: 'bug', label: 'Debug', component: IconBug },
  { value: 'zap', label: 'Task', component: IconZap },
  { value: 'package', label: 'Packages', component: IconPackage },
]

/** The glyph a launcher with no configured icon shows. */
export const defaultLauncherIcon = 'terminal'

const componentByKey = new Map(launcherIconOptions.map((o) => [o.value, o.component]))

/**
 * Resolves a launcher's icon key to a component, falling back to the terminal
 * glyph for an empty or unrecognized key so a palette row always has one.
 */
export function launcherIconComponent(key?: string): Component {
  return (key && componentByKey.get(key)) || componentByKey.get(defaultLauncherIcon)!
}
