<script setup lang="ts">
import IconPause from '~icons/lucide/pause'
import IconPlus from '~icons/lucide/plus'
import IconSettings from '~icons/lucide/settings'
import { dropClass, dropEdge, useDragReorder } from '../composables/useDragReorder'
import { moveId, type OrderDropTarget } from '../lib/listOrder'
import type { Profile } from '../types/feed'

const props = defineProps<{ profiles: Profile[]; activeProfileId: string }>()
const emit = defineEmits<{
  select: [profileId: string]
  add: []
  'open-settings': []
  reorder: [profileIds: string[]]
}>()

// A drop emits the whole rail, top first: profiles.order is one list, not a
// per-profile field. A drop that lands where the tile already was emits
// nothing, or it would persist settings and reload the rail for nothing.
const drag = useDragReorder<string>({ mime: 'application/x-hive-profile', onDrop: reorder })
const { dragging, target: dropTarget } = drag

function reorder(id: string, target: OrderDropTarget): void {
  const ids = moveId(
    props.profiles.map((profile) => profile.id),
    id,
    target,
  )
  if (ids) emit('reorder', ids)
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
  reorder(id, { id: props.profiles[to].id, edge: e.key === 'ArrowUp' ? 'before' : 'after' })
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
      :class="[
        dropClass(dropTarget, profile.id),
        {
          'text-text': profile.id === activeProfileId,
          'opacity-55': !profile.enabled,
          'opacity-40': profile.id === dragging,
        },
      ]"
      @click="emit('select', profile.id)"
      @keydown="onKeydown($event, profile.id)"
      @dragstart="drag.start($event, profile.id)"
      @dragover.prevent="drag.over($event, { id: profile.id, edge: dropEdge($event) })"
      @drop.prevent="drag.drop"
      @dragend="drag.end"
    >
      <span
        v-if="profile.id === activeProfileId"
        class="absolute bottom-2 left-[-13px] top-2 w-[3px] rounded-sm bg-accent"
      />
      <img
        v-if="profile.image"
        :src="profile.image"
        alt=""
        draggable="false"
        class="size-full rounded-[9px] object-cover"
      />
      <template v-else>{{ profile.letter }}</template>
      <span
        v-if="!profile.enabled"
        class="absolute -bottom-1 -right-1 flex size-4 items-center justify-center rounded-full border border-border bg-raised text-text-3"
        aria-hidden="true"
        ><IconPause class="size-2.5"
      /></span>
    </button>
    <button
      class="flex size-[38px] cursor-pointer items-center justify-center rounded-[10px] border border-dashed border-card text-text-4 hover:border-strong hover:text-text-2"
      aria-label="Add profile"
      data-testid="profile-add"
      @click="emit('add')"
    >
      <IconPlus class="size-4" />
    </button>
    <div class="flex-1" />
    <button
      type="button"
      class="flex size-[38px] cursor-pointer items-center justify-center rounded-[10px] text-text-3 hover:bg-hover hover:text-text"
      title="Application settings"
      aria-label="Application settings"
      data-testid="application-settings"
      @click="emit('open-settings')"
    >
      <IconSettings class="size-4" />
    </button>
  </aside>
</template>

<style scoped>
.drop-before {
  box-shadow: inset 0 3px 0 0 var(--color-accent);
}
.drop-after {
  box-shadow: inset 0 -3px 0 0 var(--color-accent);
}
</style>
