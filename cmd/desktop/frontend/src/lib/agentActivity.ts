import type { Component } from 'vue'
import IconCircleAlert from '~icons/lucide/circle-alert'
import IconCircleCheck from '~icons/lucide/circle-check'
import IconCircleOff from '~icons/lucide/circle-off'
import IconLoaderCircle from '~icons/lucide/loader-circle'

export interface StatusIndicator {
  icon: Component
  /** A text-color class. */
  color: string
  label: string
  animated?: boolean
}

/** The glyph both sidebars draw for an agent's activity status ('active', 'approval', 'ready', or unknown). */
export function activityIndicator(status: string, tool?: string): StatusIndicator {
  const who = tool || 'Agent'
  switch (status) {
    case 'active':
      return { icon: IconLoaderCircle, color: 'text-severity-success', label: `${who} is working`, animated: true }
    case 'approval':
      return { icon: IconCircleAlert, color: 'text-severity-warning', label: `${who} needs approval` }
    case 'ready':
      return { icon: IconCircleCheck, color: 'text-text-2', label: `${who} is ready` }
    default:
      return { icon: IconCircleOff, color: 'text-text-4', label: `${who} status unavailable` }
  }
}
