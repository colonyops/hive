<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import AppMenu from './AppMenu.vue'
import { ActionViews } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/pipelineservice'
import { formatCombo, useKeybindings } from '../composables/useKeybindings'
import { actionTypeMeta } from '../lib/actionPresentation'
import IconArchive from '~icons/lucide/archive'
import IconCopy from '~icons/lucide/copy'
import IconExternalLink from '~icons/lucide/external-link'
import IconEyeOff from '~icons/lucide/eye-off'
import IconLink from '~icons/lucide/link'
import IconMail from '~icons/lucide/mail'
import IconMessagesSquare from '~icons/lucide/messages-square'
import IconSquarePlus from '~icons/lucide/square-plus'
import type { ActionView } from '../types/action'
import type { InboxItem } from '../types/feed'
import type { MenuEntry } from '../types/menu'

// The one "…" menu for an inbox item, shared by every surface that offers it
// (feed rows, the detail pane) so they never drift: triage state, then
// open/copy, then the item's configured actions from actions.yml. When the
// host doesn't already hold the item's ActionViews (feed rows), the menu
// fetches them itself — the menu is ephemeral, so no caching.
const props = defineProps<{
  item: InboxItem
  /** Pre-loaded configured actions; omit to have the menu fetch per item id. */
  actions?: ActionView[]
  flip?: boolean
  ignore?: (HTMLElement | null)[]
  testid?: string
}>()
const emit = defineEmits<{
  close: []
  'set-unread': [unread: boolean]
  'toggle-archive': []
  'toggle-ignored': []
  'open-browser': []
  'copy-link': []
  'copy-contents': []
  'create-session': [target: 'repository' | 'workspace']
  'run-action': [actionId: string]
}>()

const fetchedActions = ref<ActionView[]>([])
const menuActions = computed(() => props.actions ?? fetchedActions.value)
onMounted(async () => {
  if (props.actions !== undefined) return
  try {
    fetchedActions.value = (await ActionViews(props.item.id)) ?? []
  } catch (error) {
    console.warn('Unable to load actions for item menu', error)
  }
})

const { combosFor } = useKeybindings()
function kbdFor(commandId: string): string | undefined {
  const combo = combosFor(commandId)[0]
  return combo ? formatCombo(combo) : undefined
}

const entries = computed<MenuEntry[]>(() => {
  const item = props.item
  const list: MenuEntry[] = [
    {
      kind: 'action',
      id: 'toggle-read',
      label: item.unread ? 'Mark as read' : 'Mark as unread',
      icon: IconMail,
      kbd: item.unread ? undefined : kbdFor('feed.mark-unread'),
      testid: 'menu-toggle-read',
    },
    {
      kind: 'action',
      id: 'toggle-archive',
      label: item.archivedAt ? 'Move to inbox' : 'Archive',
      icon: IconArchive,
      kbd: kbdFor('feed.toggle-archive'),
      testid: 'menu-toggle-archive',
    },
    {
      kind: 'action',
      id: 'toggle-ignored',
      label: item.ignoredAt ? 'Stop ignoring' : 'Ignore',
      icon: IconEyeOff,
      testid: 'menu-toggle-ignored',
    },
    { kind: 'separator' },
    // Link entries only when the item carries a URL — webhook payloads
    // without one have nothing to open or copy.
    ...(item.url
      ? ([
          {
            kind: 'action',
            id: 'open-browser',
            label: 'Open in browser',
            icon: IconExternalLink,
            kbd: kbdFor('feed.open-in-browser'),
            testid: 'menu-open-browser',
          },
          { kind: 'action', id: 'copy-link', label: 'Copy link', icon: IconLink, testid: 'menu-copy-link' },
        ] satisfies MenuEntry[])
      : []),
    { kind: 'action', id: 'copy-contents', label: 'Copy contents', icon: IconCopy, testid: 'menu-copy-contents' },
    { kind: 'separator' },
    {
      kind: 'action',
      id: 'create-session',
      label: 'Create Session',
      icon: IconSquarePlus,
      testid: 'menu-create-session',
    },
    { kind: 'action', id: 'create-chat', label: 'Create Chat', icon: IconMessagesSquare, testid: 'menu-create-chat' },
  ]
  if (menuActions.value.length) {
    list.push({ kind: 'separator' }, { kind: 'label', text: 'Actions' })
    for (const action of menuActions.value) {
      const meta = actionTypeMeta(action.type)
      list.push({
        kind: 'action',
        id: `action:${action.id}`,
        label: action.label,
        iconName: meta.icon,
        iconColor: meta.color,
        testid: `menu-action-${action.id}`,
      })
    }
  }
  return list
})

function onSelect(id: string): void {
  if (id === 'toggle-read') emit('set-unread', !props.item.unread)
  else if (id === 'toggle-archive') emit('toggle-archive')
  else if (id === 'toggle-ignored') emit('toggle-ignored')
  else if (id === 'open-browser') emit('open-browser')
  else if (id === 'copy-link') emit('copy-link')
  else if (id === 'copy-contents') emit('copy-contents')
  else if (id === 'create-session') emit('create-session', 'repository')
  else if (id === 'create-chat') emit('create-session', 'workspace')
  else if (id.startsWith('action:')) emit('run-action', id.slice('action:'.length))
  emit('close')
}
</script>

<template>
  <AppMenu
    :entries="entries"
    :flip="flip"
    :ignore="ignore"
    :testid="testid"
    @select="onSelect"
    @close="emit('close')"
  />
</template>
