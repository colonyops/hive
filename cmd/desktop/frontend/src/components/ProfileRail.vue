<script setup lang="ts">
import { computed } from 'vue'
import IconPause from '~icons/lucide/pause'
import IconPlus from '~icons/lucide/plus'
import { dropEdge, useDragReorder } from '../composables/useDragReorder'
import { moveId, type OrderDropTarget } from '../lib/listOrder'
import type { Profile } from '../types/feed'
import AppTooltip from '@hive/ui/components/ui/AppTooltip.vue'
import IconButton from '@hive/ui/components/ui/IconButton.vue'

const props = defineProps<{ profiles: Profile[]; activeProfileId: string }>()
const emit = defineEmits<{
  select: [profileId: string]
  add: []
  reorder: [profileIds: string[]]
}>()

// The drag target is the whole previewed rail, so the tiles slide into place
// while the pointer moves and the drop commits what the user already sees. A
// drop that ends where the rail started emits nothing, or it would persist
// settings and reload the rail for nothing.
const drag = useDragReorder<string, string[]>({ mime: 'application/x-hive-profile', onDrop: commit })
const { dragging, target: preview } = drag

const profileIds = computed(() => props.profiles.map((profile) => profile.id))
const shown = computed(() => {
  if (!preview.value) return props.profiles
  const byId = new Map(props.profiles.map((profile) => [profile.id, profile]))
  return preview.value.map((id) => byId.get(id)).filter((profile): profile is Profile => !!profile)
})

function commit(_id: string, ids: string[]): void {
  if (ids.length === profileIds.value.length && ids.every((id, at) => id === profileIds.value[at])) return
  emit('reorder', ids)
}

function overTile(event: DragEvent, id: string): void {
  if (dragging.value === null) return
  const next = moveId(preview.value ?? profileIds.value, dragging.value, { id, edge: dropEdge(event) })
  drag.over(event, next ?? preview.value)
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
  const target: OrderDropTarget = { id: props.profiles[to].id, edge: e.key === 'ArrowUp' ? 'before' : 'after' }
  const ids = moveId(profileIds.value, id, target)
  if (ids) emit('reorder', ids)
}
</script>

<template>
  <!-- The rail takes the drop, not the tiles: a release in the gap between two
       tiles still lands where the preview shows. -->
  <aside
    class="flex w-14.5 shrink-0 flex-col items-center gap-2.5 border-r border-border bg-raised py-3"
    @dragover="dragging !== null && $event.preventDefault()"
    @drop.prevent="drag.drop"
  >
    <TransitionGroup name="profile-tile" tag="div" class="contents">
      <AppTooltip
        v-for="profile in shown"
        :key="profile.id"
        :text="profile.enabled ? profile.name : `${profile.name} (disabled)`"
      >
        <button
          :aria-label="profile.enabled ? profile.name : `${profile.name}, disabled`"
          :data-id="profile.id"
          :data-enabled="profile.enabled"
          data-testid="profile-tile"
          draggable="true"
          class="relative flex size-9.5 cursor-pointer items-center justify-center rounded-xl border border-card bg-chip font-mono text-sm font-semibold text-text-2 transition-colors hover:bg-hover hover:text-text"
          :class="{
            'text-text': profile.id === activeProfileId,
            'opacity-55': !profile.enabled,
            'opacity-40': profile.id === dragging,
          }"
          @click="emit('select', profile.id)"
          @keydown="onKeydown($event, profile.id)"
          @dragstart="drag.start($event, profile.id)"
          @dragover="overTile($event, profile.id)"
          @dragend="drag.end"
        >
          <span
            v-if="profile.id === activeProfileId"
            class="absolute bottom-2 left-[-13px] top-2 w-0.75 rounded-sm bg-accent"
          />
          <img
            v-if="profile.image"
            :src="profile.image"
            alt=""
            draggable="false"
            class="size-full rounded-lg object-cover"
          />
          <template v-else>{{ profile.letter }}</template>
          <span
            v-if="!profile.enabled"
            class="absolute -bottom-1 -right-1 flex size-4 items-center justify-center rounded-full border border-border bg-raised text-text-3"
            aria-hidden="true"
            ><IconPause class="size-2.5"
          /></span>
        </button>
      </AppTooltip>
    </TransitionGroup>
    <IconButton
      label="Add profile"
      :icon="IconPlus"
      size="xl"
      variant="outline"
      class="border-dashed"
      data-testid="profile-add"
      @click="emit('add')"
    />
  </aside>
</template>

<style scoped>
.profile-tile-move {
  transition: transform 0.18s cubic-bezier(0.2, 0, 0, 1);
}

@media (prefers-reduced-motion: reduce) {
  .profile-tile-move {
    transition: none;
  }
}
</style>
