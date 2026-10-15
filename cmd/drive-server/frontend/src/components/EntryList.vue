<script setup lang="ts">
import AppMenu from '@hive/ui/components/ui/AppMenu.vue'
import BaseBadge from '@hive/ui/components/ui/BaseBadge.vue'
import IconButton from '@hive/ui/components/ui/IconButton.vue'
import { useListKeyboardNav } from '@hive/ui/composables/useListKeyboardNav'
import { ref, shallowRef, type Component } from 'vue'
import IconEllipsis from '~icons/lucide/ellipsis'
import IconFileCode from '~icons/lucide/file-code'
import IconFileText from '~icons/lucide/file-text'
import IconFolder from '~icons/lucide/folder'
import { entryMenu } from '../lib/entryActions'
import type { ListEntry } from '../lib/entries'
import { formatBytes, formatTime } from '../lib/format'
import type { ViewMode } from '../lib/viewMode'

const props = defineProps<{ entries: ListEntry[]; mode: ViewMode }>()
const emit = defineEmits<{ open: [entry: ListEntry]; action: [id: string, entry: ListEntry, anchor: HTMLElement] }>()

const active = ref(0)
const root = ref<HTMLElement | null>(null)
const menu = shallowRef<{ entry: ListEntry; anchor: HTMLElement; trigger: HTMLElement } | null>(null)

const nav = useListKeyboardNav({
  active,
  count: () => props.entries.length,
  wrap: false,
  homeEnd: () => true,
  row: (index) => root.value?.querySelector(`[data-entry-index="${index}"]`),
})

function icon(entry: ListEntry): Component {
  if (entry.kind === 'dir') return IconFolder
  return entry.doc?.kind === 'html' ? IconFileCode : IconFileText
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
  >
    <div
      v-if="mode === 'list'"
      class="entry-grid sticky top-0 border-b border-row bg-app py-2.5 text-small font-medium text-text-4"
      aria-hidden="true"
    >
      <span>Name</span>
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
        class="group cursor-pointer"
        :class="[
          mode === 'list'
            ? 'entry-grid h-13 border-b border-row'
            : 'relative flex flex-col gap-2 rounded-lg border border-card bg-raised p-3 hover:border-strong',
          index === active
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
          <span class="flex min-w-0 items-center gap-2.5">
            <component
              :is="icon(entry)"
              class="size-5 shrink-0"
              :class="entry.kind === 'dir' ? 'text-accent' : 'text-text-3'"
              aria-hidden="true"
            />
            <span class="min-w-0">
              <span class="block truncate text-body text-text" data-testid="entry-label">{{ entry.label }}</span>
              <span class="block truncate text-small text-text-4" :title="entry.secondary">{{ entry.secondary }}</span>
            </span>
          </span>
          <span class="entry-wide truncate text-small text-text-3">{{
            entry.modified ? formatTime(entry.modified) : '-'
          }}</span>
          <span class="entry-wide text-right text-small text-text-3">{{
            entry.size === null ? '-' : formatBytes(entry.size)
          }}</span>
          <span class="entry-wide">
            <BaseBadge v-if="entry.shared" tone="accent" class="px-2.5 py-0.5 text-small">Shared</BaseBadge>
          </span>
          <IconButton
            :label="`Actions for ${entry.label}`"
            :icon="IconEllipsis"
            size="xl"
            data-testid="entry-menu"
            @click.stop="openMenu(entry, $event)"
          />
        </template>
        <template v-else>
          <div class="flex items-start justify-between">
            <component
              :is="icon(entry)"
              class="size-10"
              :class="entry.kind === 'dir' ? 'text-accent' : 'text-text-3'"
              aria-hidden="true"
            />
            <IconButton
              :label="`Actions for ${entry.label}`"
              :icon="IconEllipsis"
              size="xl"
              data-testid="entry-menu"
              @click.stop="openMenu(entry, $event)"
            />
          </div>
          <div class="min-w-0">
            <span class="block truncate text-body font-medium text-text" data-testid="entry-label">{{
              entry.label
            }}</span>
            <span class="block truncate text-small text-text-4" :title="entry.secondary">{{ entry.secondary }}</span>
          </div>
          <BaseBadge v-if="entry.shared" tone="accent" class="w-fit px-2.5 py-0.5 text-small">Shared</BaseBadge>
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
    grid-template-columns: minmax(0, 1fr) 12rem 6rem 6rem 2.125rem;
  }
  .entry-wide {
    display: block;
  }
}
</style>
