import { defineComponent, h, type Component } from 'vue'
import TaskStatusIcon from './TaskStatusIcon.vue'

const icons = new Map<string, Component>()

/**
 * A status icon for a block that takes an icon component (AppSelect, AppMenu,
 * IconButton). Those blocks tint and size the component they get; the wrapper
 * takes that class so the icon keeps its own color. Cached because a new
 * component on every render would remount the icon.
 */
export function taskStatusIcon(status: string): Component {
  let icon = icons.get(status)
  if (!icon) {
    icon = defineComponent({
      name: 'StatusOptionIcon',
      render: () => h('span', { class: 'inline-flex items-center justify-center' }, [h(TaskStatusIcon, { status })]),
    })
    icons.set(status, icon)
  }
  return icon
}
