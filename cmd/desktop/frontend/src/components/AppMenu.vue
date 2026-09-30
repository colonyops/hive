<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { onClickOutside } from '@vueuse/core'
import IconCheck from '~icons/lucide/check'
import AppIcon from './AppIcon.vue'
import { useEscapeToClose } from '../composables/useEscapeToClose'
import type { MenuEntry } from '../types/menu'

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
}>()
const emit = defineEmits<{ select: [id: string]; close: [] }>()

const root = ref<HTMLElement | null>(null)
onClickOutside(root, () => emit('close'), { ignore: props.ignore ?? [] })
useEscapeToClose(() => emit('close'))

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
watch(() => props.anchor, () => measure())

onMounted(() => {
  if (!props.anchor) return
  measure()
  // Capture phase: scrolling any ancestor moves the anchor, not just window.
  window.addEventListener('scroll', measure, true)
  window.addEventListener('resize', measure)
})
onBeforeUnmount(() => {
  window.removeEventListener('scroll', measure, true)
  window.removeEventListener('resize', measure)
})
</script>

<template>
  <Teleport to="body" :disabled="!anchor">
    <div
      ref="root"
      class="app-menu"
      :class="{ flip: !anchor && flip }"
      :style="anchor ? anchoredStyle : (width ? { width } : undefined)"
      role="menu"
      :data-testid="testid"
    >
      <template v-for="(entry, index) in entries" :key="index">
        <div v-if="entry.kind === 'separator'" class="app-menu-sep" />
        <div v-else-if="entry.kind === 'label'" class="app-menu-label">{{ entry.text }}</div>
        <button
          v-else
          class="app-menu-entry"
          :role="entry.checked === undefined ? 'menuitem' : 'menuitemcheckbox'"
          :aria-checked="entry.checked"
          :disabled="entry.disabled"
          :data-testid="entry.testid"
          @click="emit('select', entry.id)"
        >
          <component :is="entry.icon" v-if="entry.icon" class="size-3.5 shrink-0" :style="entry.iconColor ? { color: entry.iconColor } : undefined" />
          <AppIcon v-else-if="entry.iconName" :name="entry.iconName" class="size-3.5 shrink-0" :style="entry.iconColor ? { color: entry.iconColor } : undefined" />
          <IconCheck v-else-if="entry.checked" class="size-3.5 shrink-0 text-accent" />
          <span v-else-if="entry.checked === false" class="size-3.5 shrink-0" aria-hidden="true" />
          <span class="min-w-0 flex-1 truncate">{{ entry.label }}</span>
          <span v-if="entry.kbd" class="app-menu-kbd">{{ entry.kbd }}</span>
        </button>
      </template>
    </div>
  </Teleport>
</template>

<style scoped>
.app-menu { position: absolute; right: 0; top: calc(100% + 5px); z-index: 30; width: 230px; border: 1px solid var(--color-strong); border-radius: 8px; background: var(--color-pane); padding: 5px; box-shadow: 0 18px 45px -12px rgb(0 0 0 / .55); }
.app-menu.flip { top: auto; bottom: calc(100% + 5px); }
.app-menu-entry { display: flex; width: 100%; align-items: center; gap: 8px; cursor: pointer; border-radius: 6px; padding: 7px 9px; color: var(--color-text-2); font-size: 12px; text-align: left; }
.app-menu-entry:hover:not(:disabled) { background: var(--color-hover); color: var(--color-text); }
.app-menu-entry:disabled { cursor: default; color: var(--color-text-4); }
.app-menu-kbd { margin-left: auto; padding-left: 8px; font-family: var(--font-mono); font-size: 10.5px; color: var(--color-text-4); }
.app-menu-sep { height: 1px; margin: 5px 4px; background: var(--color-row); }
.app-menu-label { padding: 6px 9px 3px; font-family: var(--font-mono); font-size: 9.5px; font-weight: 600; letter-spacing: .1em; text-transform: uppercase; color: var(--color-text-4); }
</style>
