<script setup lang="ts">
import { computed } from 'vue'
import AppMenu from '@hive/ui/components/ui/AppMenu.vue'
import IconInfo from '~icons/lucide/info'
import IconPencil from '~icons/lucide/pencil'
import IconPlay from '~icons/lucide/play'
import IconRecycle from '~icons/lucide/recycle'
import IconSquare from '~icons/lucide/square'
import IconTrash from '~icons/lucide/trash-2'
import type { SessionRow } from '../composables/useSessionActions'
import type { MenuEntry } from '@hive/ui/types/menu'

// The one menu for a hive session, opened from the session row. It owns the
// session's own operations; anything a *host* contributes — a new tab per
// configured agent, a user-defined action — arrives as `extra` and lands in its
// own labelled section, so a second menu never has to be built beside this one.
const props = defineProps<{
  session: SessionRow
  /**
   * The scratch terminal, which has no hive session behind it: its tmux session
   * is the whole thing, so there is nothing to edit, recycle, delete or show
   * details of.
   */
  scratch?: boolean
  /**
   * Whether the terminal behind the row runs. Left unset until its state is
   * known, when both Start and Kill are offered rather than guessing one.
   */
  terminal?: 'running' | 'stopped'
  /**
   * A session on a server. Its terminal is started on the server, so Start
   * is not offered.
   */
  remote?: boolean
  /** Host-contributed entries, appended under `extraLabel`. */
  extra?: MenuEntry[]
  extraLabel?: string
  flip?: boolean
  ignore?: (HTMLElement | null)[]
  testid?: string
}>()
const emit = defineEmits<{
  close: []
  start: []
  kill: []
  detail: []
  edit: []
  recycle: []
  delete: []
  extra: [id: string]
}>()

function lifecycleEntries(noun: 'session' | 'terminal'): MenuEntry[] {
  const list: MenuEntry[] = []
  if (props.terminal !== 'running' && !props.remote) {
    list.push({ kind: 'action', id: 'start', label: `Start ${noun}`, icon: IconPlay, testid: 'session-menu-start' })
  }
  if (props.terminal !== 'stopped') {
    list.push({ kind: 'action', id: 'kill', label: `Stop ${noun}…`, icon: IconSquare, testid: 'session-menu-kill' })
  }
  return list
}

const entries = computed<MenuEntry[]>(() => {
  if (props.scratch) return lifecycleEntries('terminal')
  const list: MenuEntry[] = []
  // The terminal's own lifecycle leads: starting a stopped session is also an
  // attach, and killing ends the terminal without touching the session itself —
  // which is what separates it from Recycle and Delete below. Only an active
  // session has a checkout to run one in.
  if (props.session.state === 'active') {
    list.push(...lifecycleEntries('session'), { kind: 'separator' })
  }
  list.push(
    { kind: 'action', id: 'detail', label: 'Session details…', icon: IconInfo, testid: 'session-menu-detail' },
    { kind: 'action', id: 'edit', label: 'Edit session…', icon: IconPencil, testid: 'session-menu-edit' },
    { kind: 'separator' },
  )
  // Only an active session has a clone to reset; recycling a recycled one is
  // rejected by hive, so it is not offered.
  if (props.session.state === 'active') {
    list.push({ kind: 'action', id: 'recycle', label: 'Recycle…', icon: IconRecycle, testid: 'session-menu-recycle' })
  }
  list.push({ kind: 'action', id: 'delete', label: 'Delete…', icon: IconTrash, testid: 'session-menu-delete' })
  if (props.extra?.length) {
    list.push({ kind: 'separator' }, { kind: 'label', text: props.extraLabel ?? 'Actions' }, ...props.extra)
  }
  return list
})

const own = new Set(['start', 'kill', 'detail', 'edit', 'recycle', 'delete'])

function onSelect(id: string): void {
  if (id === 'start') emit('start')
  else if (id === 'kill') emit('kill')
  else if (id === 'detail') emit('detail')
  else if (id === 'edit') emit('edit')
  else if (id === 'recycle') emit('recycle')
  else if (id === 'delete') emit('delete')
  if (!own.has(id)) emit('extra', id)
  emit('close')
}
</script>

<template>
  <AppMenu
    :entries="entries"
    :flip="flip"
    width="min(230px, 100%)"
    :ignore="ignore"
    :testid="testid ?? 'session-row-menu'"
    @select="onSelect"
    @close="emit('close')"
  />
</template>
