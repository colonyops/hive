<script setup lang="ts">
import AppMenu from '@hive/ui/components/ui/AppMenu.vue'
import BaseBadge from '@hive/ui/components/ui/BaseBadge.vue'
import IconButton from '@hive/ui/components/ui/IconButton.vue'
import { useListKeyboardNav } from '@hive/ui/composables/useListKeyboardNav'
import { computed, ref, shallowRef, type Component } from 'vue'
import IconCode from '~icons/material-symbols/code-blocks'
import IconFile from '~icons/material-symbols/description'
import IconFolder from '~icons/material-symbols/folder'
import IconMore from '~icons/material-symbols/more-vert'
import { entryMenu } from '../lib/entryActions'
import type { ListEntry } from '../lib/entries'
import { formatBytes, formatDate, formatTime } from '../lib/format'
import type { ViewMode } from '../lib/viewMode'

const props = defineProps<{ entries: ListEntry[]; mode: ViewMode }>()
const emit = defineEmits<{ open: [entry: ListEntry]; action: [id: string, entry: ListEntry, anchor: HTMLElement] }>()

const active = ref(0)
// The cursor only means something while the list has focus; drawn at rest it reads as a selection nobody made.
const focused = ref(false)
const root = ref<HTMLElement | null>(null)
const menu = shallowRef<{ entry: ListEntry; anchor: HTMLElement; trigger: HTMLElement } | null>(null)

const nav = useListKeyboardNav({
  active,
  count: () => props.entries.length,
  wrap: false,
  homeEnd: () => true,
  row: (index) => root.value?.querySelector(`[data-entry-index="${index}"]`),
})

const showLocation = computed(() => props.entries.some((entry) => entry.location !== null))

function icon(entry: ListEntry): Component {
  if (entry.kind === 'dir') return IconFolder
  return entry.doc?.kind === 'html' ? IconCode : IconFile
}

function iconTone(entry: ListEntry): string {
  if (entry.kind === 'dir') return 'text-text-3'
  return entry.doc?.kind === 'html' ? 'text-node-orange' : 'text-node-blue'
}

function onFocusOut(event: FocusEvent): void {
  if (!root.value?.contains(event.relatedTarget as Node | null)) focused.value = false
}

function onKeydown(event: KeyboardEvent): void {
  if (nav.onKeydown(event)) return
  if (props.mode === 'grid' && (event.key === 'ArrowRight' || event.key === 'ArrowLeft')) {
    nav.step(event.key === 'ArrowRight' ? 1 : -1)
    event.preventDefault()
    return
  }
  const entry = props.entries[active.value]
  if (event.key === 'Enter' && entry) {
    event.preventDefault()
    emit('open', entry)
  }
}

// The menu anchors to the whole row: an anchored menu is as wide as its anchor, and a button is too narrow.
function openMenu(entry: ListEntry, event: MouseEvent): void {
  const target = event.currentTarget as HTMLElement
  menu.value = { entry, anchor: target.closest<HTMLElement>('[data-entry-index]') ?? target, trigger: target }
}

function onSelect(id: string): void {
  const target = menu.value
  menu.value = null
  if (target) emit('action', id, target.entry, target.trigger)
}
</script>

<template>
  <div
    ref="root"
    role="listbox"
    tabindex="0"
    aria-label="Contents"
    class="outline-none"
    data-testid="entries"
    :data-mode="mode"
    @keydown="onKeydown"
    @focusin="focused = true"
    @focusout="onFocusOut"
  >
    <div
      v-if="mode === 'list'"
      class="entry-grid sticky top-0 border-b border-row bg-app py-2.5 text-small font-medium text-text-4"
      :class="{ 'has-location': showLocation }"
      aria-hidden="true"
    >
      <span>Name</span>
      <span v-if="showLocation" class="entry-wide">Location</span>
      <span class="entry-wide">Modified</span>
      <span class="entry-wide text-right">Size</span>
      <span class="entry-wide">Shared</span>
      <span />
    </div>
    <div :class="mode === 'grid' ? 'grid grid-cols-[repeat(auto-fill,minmax(14rem,1fr))] gap-4 p-6' : ''">
      <div
        v-for="(entry, index) in entries"
        :key="entry.key"
        role="option"
        :aria-selected="index === active"
        :data-entry-index="index"
        :data-kind="entry.kind"
        data-testid="entry"
        :title="entry.secondary"
        class="group cursor-pointer"
        :class="[
          mode === 'list'
            ? ['entry-grid h-13 border-b border-row', { 'has-location': showLocation }]
            : 'relative flex h-36 flex-col rounded-lg border border-card bg-raised p-3 hover:border-strong',
          focused && index === active
            ? mode === 'list'
              ? 'bg-selection'
              : 'border-accent'
            : mode === 'list'
              ? 'hover:bg-chip'
              : '',
        ]"
        @click="active = index"
        @dblclick="emit('open', entry)"
        @contextmenu.prevent="openMenu(entry, $event)"
      >
        <template v-if="mode === 'list'">
          <span class="flex min-w-0 items-center gap-3">
            <span class="flex size-6 shrink-0 items-center justify-center">
              <component :is="icon(entry)" class="size-6" :class="iconTone(entry)" aria-hidden="true" />
            </span>
            <span class="min-w-0 truncate text-body text-text" data-testid="entry-label">{{ entry.label }}</span>
          </span>
          <span v-if="showLocation" class="entry-wide truncate text-small text-text-3" data-testid="entry-location">{{
            entry.location
          }}</span>
          <span
            class="entry-wide truncate text-small text-text-3"
            :title="entry.modified ? formatTime(entry.modified) : undefined"
            >{{ entry.modified ? formatDate(entry.modified) : '—' }}</span
          >
          <span class="entry-wide text-right text-small text-text-3">{{
            entry.kind === 'dir' || entry.size === null ? '—' : formatBytes(entry.size)
          }}</span>
          <span class="entry-wide">
            <BaseBadge v-if="entry.shared" tone="accent" class="px-2.5 py-0.5 text-small">Shared</BaseBadge>
          </span>
          <IconButton
            :label="`Actions for ${entry.label}`"
            :icon="IconMore"
            size="xl"
            data-testid="entry-menu"
            @click.stop="openMenu(entry, $event)"
          />
        </template>
        <template v-else>
          <div class="flex items-start justify-between">
            <component :is="icon(entry)" class="size-10" :class="iconTone(entry)" aria-hidden="true" />
            <IconButton
              :label="`Actions for ${entry.label}`"
              :icon="IconMore"
              size="xl"
              data-testid="entry-menu"
              @click.stop="openMenu(entry, $event)"
            />
          </div>
          <span class="mt-auto block truncate text-body font-medium text-text" data-testid="entry-label">{{
            entry.label
          }}</span>
          <div class="mt-1 flex h-6 items-center gap-2">
            <span class="min-w-0 flex-1 truncate text-small text-text-4">{{
              entry.modified ? formatDate(entry.modified) : '—'
            }}</span>
            <BaseBadge v-if="entry.shared" tone="accent" class="px-2.5 py-0.5 text-small">Shared</BaseBadge>
          </div>
        </template>
      </div>
    </div>
    <AppMenu
      v-if="menu"
      :entries="entryMenu(menu.entry)"
      :anchor="menu.anchor"
      testid="entry-menu-panel"
      @select="onSelect"
      @close="menu = null"
    />
  </div>
</template>

<style scoped>
.entry-grid {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 2.125rem;
  align-items: center;
  column-gap: 0.75rem;
  padding-inline: 1.5rem;
}
.entry-wide {
  display: none;
}
@media (min-width: 768px) {
  .entry-grid {
    grid-template-columns: minmax(0, 1fr) 8rem 6rem 6rem 2.125rem;
  }
  .entry-grid.has-location {
    grid-template-columns: minmax(0, 1fr) minmax(0, 14rem) 8rem 6rem 6rem 2.125rem;
  }
  .entry-wide {
    display: block;
  }
}
</style>
