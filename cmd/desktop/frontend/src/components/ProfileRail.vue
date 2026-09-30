<script setup lang="ts">
import { ref } from 'vue'
import IconPause from '~icons/lucide/pause'
import IconPlus from '~icons/lucide/plus'
import IconSettings from '~icons/lucide/settings'
import type { Profile } from '../types/feed'

const props = defineProps<{ profiles: Profile[]; activeProfileId: string }>()
const emit = defineEmits<{ select: [profileId: string]; add: []; 'open-settings': []; reorder: [profileIds: string[]] }>()

// Same native HTML5 DnD approach as the sidebar: the dragged tile is tracked in
// a local ref (dataTransfer can't be read during dragover) and the drop target
// drives the insertion indicator. A drop emits the whole rail, top first —
// profiles.order is one list, not a per-profile field.
const PROFILE_DRAG_MIME = 'application/x-hive-profile'

const dragging = ref<string | null>(null)
const dropTarget = ref<{ id: string; edge: 'before' | 'after' } | null>(null)

function onDragStart(e: DragEvent, id: string): void {
  dragging.value = id
  if (e.dataTransfer) {
    e.dataTransfer.effectAllowed = 'move'
    e.dataTransfer.setData(PROFILE_DRAG_MIME, id)
  }
}

function onDragOver(e: DragEvent, id: string): void {
  if (!dragging.value) return
  if (e.dataTransfer) e.dataTransfer.dropEffect = 'move'
  const rect = (e.currentTarget as HTMLElement).getBoundingClientRect()
  dropTarget.value = { id, edge: e.clientY < rect.top + rect.height / 2 ? 'before' : 'after' }
}

function onDrop(): void {
  const target = dropTarget.value
  if (dragging.value && target) emitReorder(moved(dragging.value, target.id, target.edge))
  onDragEnd()
}

function onDragEnd(): void {
  dragging.value = null
  dropTarget.value = null
}

// moved is the rail with id lifted out and reinserted at target's edge.
function moved(id: string, targetID: string, edge: 'before' | 'after'): string[] {
  const ids = props.profiles.map((p) => p.id).filter((other) => other !== id)
  const at = ids.indexOf(targetID)
  if (at === -1) return []
  ids.splice(edge === 'before' ? at : at + 1, 0, id)
  return ids
}

// A drop that lands where the tile already was is not a reorder — emitting it
// would persist settings and reload the rail for nothing.
function emitReorder(ids: string[]): void {
  if (ids.length !== props.profiles.length) return
  if (ids.every((id, i) => id === props.profiles[i].id)) return
  emit('reorder', ids)
}

// Alt+Up/Down moves the focused tile, so the rail can be reordered without a
// pointer. Alt is what keeps it off the plain arrow keys the feed binds.
function onKeydown(e: KeyboardEvent, id: string): void {
  if (!e.altKey || (e.key !== 'ArrowUp' && e.key !== 'ArrowDown')) return
  const at = props.profiles.findIndex((p) => p.id === id)
  const to = e.key === 'ArrowUp' ? at - 1 : at + 1
  if (at === -1 || to < 0 || to >= props.profiles.length) return
  e.preventDefault()
  e.stopPropagation()
  emitReorder(moved(id, props.profiles[to].id, e.key === 'ArrowUp' ? 'before' : 'after'))
}

function dropClass(id: string): string {
  const target = dropTarget.value
  if (!target || target.id !== id) return ''
  return target.edge === 'before' ? 'drop-before' : 'drop-after'
}
</script>

<template>
  <aside class="flex w-[58px] shrink-0 flex-col items-center gap-2.5 border-r border-border bg-raised py-3">
    <button
      v-for="profile in profiles"
      :key="profile.id"
      :title="profile.enabled ? profile.name : `${profile.name} (disabled)`"
      :aria-label="profile.enabled ? profile.name : `${profile.name}, disabled`"
      :data-id="profile.id"
      :data-enabled="profile.enabled"
      data-testid="profile-tile"
      draggable="true"
      class="relative flex size-[38px] cursor-pointer items-center justify-center rounded-[10px] border border-card bg-chip font-mono text-sm font-semibold text-text-2 transition-colors hover:bg-hover hover:text-text"
      :class="[dropClass(profile.id), { 'text-text': profile.id === activeProfileId, 'opacity-55': !profile.enabled, 'opacity-40': profile.id === dragging }]"
      @click="emit('select', profile.id)"
      @keydown="onKeydown($event, profile.id)"
      @dragstart="onDragStart($event, profile.id)"
      @dragover.prevent="onDragOver($event, profile.id)"
      @drop.prevent="onDrop"
      @dragend="onDragEnd"
    >
      <span v-if="profile.id === activeProfileId" class="absolute bottom-2 left-[-13px] top-2 w-[3px] rounded-sm bg-accent" />
      <img v-if="profile.image" :src="profile.image" alt="" draggable="false" class="size-full rounded-[9px] object-cover">
      <template v-else>{{ profile.letter }}</template>
      <span v-if="!profile.enabled" class="absolute -bottom-1 -right-1 flex size-4 items-center justify-center rounded-full border border-border bg-raised text-text-3" aria-hidden="true"><IconPause class="size-2.5" /></span>
    </button>
    <button class="flex size-[38px] cursor-pointer items-center justify-center rounded-[10px] border border-dashed border-card text-text-4 hover:border-strong hover:text-text-2" aria-label="Add profile" data-testid="profile-add" @click="emit('add')"><IconPlus class="size-4" /></button>
    <div class="flex-1" />
    <button
      type="button"
      class="flex size-[38px] cursor-pointer items-center justify-center rounded-[10px] text-text-3 hover:bg-hover hover:text-text"
      title="Application settings"
      aria-label="Application settings"
      data-testid="application-settings"
      @click="emit('open-settings')"
    ><IconSettings class="size-4" /></button>
  </aside>
</template>

<style scoped>
.drop-before { box-shadow: inset 0 3px 0 0 var(--color-accent); }
.drop-after { box-shadow: inset 0 -3px 0 0 var(--color-accent); }
</style>
