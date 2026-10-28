import type { Component } from 'vue'
import IconFile from '~icons/lucide/file'
import IconFileCode from '~icons/lucide/file-code'
import IconFileText from '~icons/lucide/file-text'
import IconLayoutDashboard from '~icons/lucide/layout-dashboard'

const icons: Record<string, Component> = { markdown: IconFileText, html: IconFileCode, canvas: IconLayoutDashboard }

export function documentIcon(kind: string): Component {
  return icons[kind] ?? IconFile
}
