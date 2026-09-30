import type { Component } from 'vue'
import IconKeyboard from '~icons/lucide/keyboard'
import IconPalette from '~icons/lucide/palette'
import IconPlug from '~icons/lucide/plug'
import IconPlay from '~icons/lucide/play'
import IconSquareTerminal from '~icons/lucide/square-terminal'
import IconMessagesSquare from '~icons/lucide/messages-square'
import IconHardDrive from '~icons/lucide/hard-drive'
import IconBell from '~icons/lucide/bell'
import IconPin from '~icons/lucide/pin'
import IconInfo from '~icons/lucide/info'
import IconSliders from '~icons/lucide/sliders-horizontal'
import IconZap from '~icons/lucide/zap'
import IconBoxes from '~icons/lucide/boxes'
import IconActivity from '~icons/lucide/activity'
import type { ApplicationSettingsSection } from '../../router'

// Keyed by section id and ordered by router.ts's applicationSettingsSections,
// so a section added there shows up here (and TypeScript flags the missing
// entry) instead of being routable but absent from the UI.
export const applicationSettingsSectionMeta: Record<
  ApplicationSettingsSection,
  { label: string; title: string; icon: Component }
> = {
  general: { label: 'General', title: 'General', icon: IconSliders },
  appearance: { label: 'Appearance', title: 'Appearance', icon: IconPalette },
  keybindings: { label: 'Keyboard', title: 'Keyboard shortcuts', icon: IconKeyboard },
  terminal: { label: 'Terminal', title: 'Terminal', icon: IconSquareTerminal },
  agents: { label: 'Chats', title: 'Chats', icon: IconMessagesSquare },
  integrations: { label: 'Integrations', title: 'Integrations', icon: IconPlug },
  actions: { label: 'Actions', title: 'Actions', icon: IconPlay },
  launchers: { label: 'Quick terminals', title: 'Quick terminals', icon: IconZap },
  hive: { label: 'Hive CLI', title: 'Hive CLI', icon: IconBoxes },
  notifications: { label: 'Notifications', title: 'Notifications', icon: IconBell },
  menubar: { label: 'Menu bar', title: 'Menu bar', icon: IconPin },
  system: { label: 'System', title: 'System', icon: IconHardDrive },
  observability: { label: 'Observability', title: 'Observability', icon: IconActivity },
  about: { label: 'About', title: 'About', icon: IconInfo },
}
