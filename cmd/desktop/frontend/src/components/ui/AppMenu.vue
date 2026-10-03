<script setup lang="ts">
import { computed, nextTick, onMounted, ref, shallowRef, watch } from 'vue'
import { onClickOutside, useEventListener } from '@vueuse/core'
import IconCheck from '~icons/lucide/check'
import IconChevronRight from '~icons/lucide/chevron-right'
import IconSearch from '~icons/lucide/search'
import AppIcon from '../AppIcon.vue'
import { useEscapeToClose } from '../../composables/useEscapeToClose'
import type { MenuEntry, MenuSearch } from '../../types/menu'
import EmptyState from './EmptyState.vue'
import Kbd from './Kbd.vue'

// The shared dropdown menu. Owns the chrome (panel, entries, separators,
// group labels, shortcut hints) and dismissal (Escape, click-outside); the
// host owns the open flag and anchors the menu inside a `relative` wrapper —
// or passes `anchor` to escape a clipping ancestor instead.
// `ignore` should list the toggle button so its click doesn't close-then-reopen.
const props = defineProps<{
  entries: MenuEntry[]
  /** Open upward from the anchor — for hosts near the bottom of a scroll area. */
  flip?: boolean
  /** CSS width override — for hosts narrower than the default panel (the sidebar). */
  width?: string
  /** Anchor row — when set, the menu teleports to <body> and takes a fixed
   * position spanning the anchor's right edge, escaping `overflow` ancestors
   * (an absolute panel inside a capped scroll section would extend the scroll
   * range and scroll the list instead of floating over it). Flip is computed
   * from the viewport; the `flip` and `width` props are ignored. */
  anchor?: HTMLElement | null
  ignore?: (HTMLElement | null)[]
  testid?: string
  /** A filter box over the entries. */
  search?: MenuSearch
  /** Set on the panel a submenu entry opens: it hangs off its entry's left side. */
  nested?: boolean
}>()
const emit = defineEmits<{ select: [id: string]; close: [] }>()

const root = ref<HTMLElement | null>(null)
onClickOutside(root, () => emit('close'), { ignore: () => props.ignore ?? [] })
useEscapeToClose(() => emit('close'))

const query = ref('')
const searchInput = ref<HTMLInputElement | null>(null)
const shown = computed(() => {
  const q = query.value.trim().toLowerCase()
  if (!props.search || !q) return props.entries
  return props.entries.filter((entry) => entry.kind === 'action' && entry.label.toLowerCase().includes(q))
})
onMounted(() => searchInput.value?.focus())

// One submenu open at a time. Its own Escape closes only it (the Escape stack
// fires the topmost caller), and focus goes back to the entry that opened it.
const openSubmenu = ref('')
const submenuToggle = shallowRef<HTMLElement | null>(null)
function toggleSubmenu(id: string, event: MouseEvent): void {
  submenuToggle.value = event.currentTarget as HTMLElement
  openSubmenu.value = openSubmenu.value === id ? '' : id
}
function closeSubmenu(): void {
  openSubmenu.value = ''
  void nextTick(() => submenuToggle.value?.focus())
}

// ── Anchored (teleported) placement ──────────────────────────────────────
const ANCHOR_GAP = 5
// Entry menus stay under ~180px tall, so the hosts' inline heuristic holds
// here too: within that of the viewport bottom, open upward.
const FLIP_WITHIN = 180

const anchoredStyle = ref<Record<string, string>>({})

function measure(): void {
  const el = props.anchor
  if (!el) return
  const rect = el.getBoundingClientRect()
  const width = Math.min(230, rect.width)
  const flip = rect.bottom > window.innerHeight - FLIP_WITHIN
  anchoredStyle.value = {
    position: 'fixed',
    right: 'auto',
    left: `${rect.right - width}px`,
    width: `${width}px`,
    top: flip ? 'auto' : `${rect.bottom + ANCHOR_GAP}px`,
    bottom: flip ? `${window.innerHeight - rect.top + ANCHOR_GAP}px` : 'auto',
  }
}

// A host that resolves its anchor after this mounts — a template ref settling,
// a row re-keyed under the panel — would otherwise keep the placement measured
// from whatever was there at mount, and an anchor that was null or detached
// then measures as a zero rect: the panel takes `left: 0; width: 0` and draws
// nothing.
watch(
  () => props.anchor,
  () => measure(),
)

onMounted(measure)
const anchoredWindow = () => (props.anchor ? window : null)
// Capture phase: scrolling any ancestor moves the anchor, not just window.
useEventListener(anchoredWindow, 'scroll', measure, { capture: true })
useEventListener(anchoredWindow, 'resize', measure)
</script>

<template>
  <Teleport to="body" :disabled="!anchor">
    <div
      ref="root"
      class="app-menu"
      :class="{ flip: !anchor && flip, 'app-menu-nested': nested, 'app-menu-searchable': search }"
      :style="anchor ? anchoredStyle : width ? { width } : undefined"
      role="menu"
      :data-testid="testid"
    >
      <label v-if="search" class="flex shrink-0 items-center gap-2 border-b border-row px-2.5 py-2">
        <IconSearch class="size-3.5 shrink-0 text-text-4" />
        <input
          ref="searchInput"
          v-model="query"
          type="text"
          :placeholder="`${search.label}…`"
          :aria-label="search.label"
          class="w-0 min-w-0 flex-1 bg-transparent text-body text-text outline-none placeholder:text-text-4"
          :data-testid="search.testid"
        />
      </label>
      <div :class="{ 'hive-scroll app-menu-scroll': search }">
        <template v-for="(entry, index) in shown" :key="index">
          <div v-if="entry.kind === 'separator'" class="app-menu-sep" />
          <div v-else-if="entry.kind === 'label'" class="app-menu-label">{{ entry.text }}</div>
          <div v-else-if="entry.kind === 'submenu'" class="relative">
            <button
              class="app-menu-entry"
              role="menuitem"
              aria-haspopup="menu"
              :aria-expanded="openSubmenu === entry.id"
              :data-testid="entry.testid"
              @click="toggleSubmenu(entry.id, $event)"
            >
              <component :is="entry.icon" v-if="entry.icon" class="size-3.5 shrink-0" />
              <span class="min-w-0 flex-1 truncate">{{ entry.label }}</span>
              <IconChevronRight class="size-3.5 shrink-0 text-text-4" />
            </button>
            <AppMenu
              v-if="openSubmenu === entry.id"
              nested
              :entries="entry.entries"
              :search="entry.search"
              :ignore="[submenuToggle]"
              :testid="entry.panelTestid"
              @select="emit('select', $event)"
              @close="closeSubmenu"
            />
          </div>
          <button
            v-else
            class="app-menu-entry"
            :role="entry.checked === undefined ? 'menuitem' : 'menuitemcheckbox'"
            :aria-checked="entry.checked"
            :disabled="entry.disabled"
            :data-testid="entry.testid"
            @click="emit('select', entry.id)"
          >
            <component
              :is="entry.icon"
              v-if="entry.icon"
              class="size-3.5 shrink-0"
              :style="entry.iconColor ? { color: entry.iconColor } : undefined"
            />
            <AppIcon
              v-else-if="entry.iconName"
              :name="entry.iconName"
              class="size-3.5 shrink-0"
              :style="entry.iconColor ? { color: entry.iconColor } : undefined"
            />
            <IconCheck v-else-if="entry.checked" class="size-3.5 shrink-0 text-accent" />
            <span v-else-if="entry.checked === false" class="size-3.5 shrink-0" aria-hidden="true" />
            <span class="min-w-0 flex-1 truncate">{{ entry.label }}</span>
            <Kbd v-if="entry.kbd" class="ml-auto pl-2 text-micro text-text-4">{{ entry.kbd }}</Kbd>
          </button>
        </template>
        <EmptyState
          v-if="search && !shown.length"
          class="px-3"
          message="No matches"
          :data-testid="testid ? `${testid}-empty` : undefined"
        />
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
.app-menu {
  position: absolute;
  right: 0;
  top: calc(100% + 5px);
  z-index: 30;
  width: 230px;
  border: 1px solid var(--color-strong);
  border-radius: var(--radius-lg);
  background: var(--color-pane);
  padding: 5px;
  box-shadow: var(--shadow-popover);
}
.app-menu.flip {
  top: auto;
  bottom: calc(100% + 5px);
}
.app-menu-nested {
  top: -5px;
  right: calc(100% + 7px);
}
.app-menu-searchable {
  display: flex;
  max-height: 16rem;
  flex-direction: column;
  overflow: hidden;
  padding: 0;
}
.app-menu-scroll {
  min-height: 0;
  overflow-y: auto;
  padding: 5px;
}
.app-menu-entry {
  display: flex;
  width: 100%;
  align-items: center;
  gap: 8px;
  cursor: pointer;
  border-radius: var(--radius-md);
  padding: 7px 9px;
  color: var(--color-text-2);
  font-size: var(--text-small);
  text-align: left;
}
.app-menu-entry:hover:not(:disabled) {
  background: var(--color-hover);
  color: var(--color-text);
}
.app-menu-entry:disabled {
  cursor: default;
  color: var(--color-text-4);
}
.app-menu-sep {
  height: 1px;
  margin: 5px 4px;
  background: var(--color-row);
}
.app-menu-label {
  padding: 6px 9px 3px;
  font-family: var(--font-mono);
  font-size: var(--text-micro);
  font-weight: 600;
  letter-spacing: 0.1em;
  text-transform: uppercase;
  color: var(--color-text-4);
}
</style>
