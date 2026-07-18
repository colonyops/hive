<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import type { ComponentPublicInstance } from 'vue'
import IconSearch from '~icons/lucide/search'
import { useCommandPalette, type Command } from '../composables/useCommands'

const { open, query, results, toggle, run } = useCommandPalette()

// ── Selection tracking ────────────────────────────────────────────────────────

const selectedIndex = ref(0)
const inputRef = ref<HTMLInputElement | null>(null)
const rowElements = new Map<number, HTMLElement>()

// Reset selection and row map when results change
watch(results, () => {
  selectedIndex.value = 0
  rowElements.clear()
})

// Autofocus input when palette opens
watch(open, async (v) => {
  if (v) {
    selectedIndex.value = 0
    rowElements.clear()
    await nextTick()
    inputRef.value?.focus()
  }
})

// Scroll selected row into view
watch(selectedIndex, (idx) => {
  nextTick(() => rowElements.get(idx)?.scrollIntoView({ block: 'nearest' }))
})

function setRowRef(el: Element | ComponentPublicInstance | null, index: number): void {
  if (el instanceof HTMLElement) rowElements.set(index, el)
  else rowElements.delete(index)
}

// ── Display list (section headers + commands interleaved) ──────────────────────

interface HeaderEntry { kind: 'header'; group: string }
interface CmdEntry { kind: 'cmd'; cmd: Command; index: number }
type DisplayEntry = HeaderEntry | CmdEntry

const displayList = computed<DisplayEntry[]>(() => {
  const entries: DisplayEntry[] = []
  let lastGroup: string | undefined = undefined
  results.value.forEach((cmd, i) => {
    const group = cmd.group ?? ''
    if (group !== lastGroup) {
      if (group) entries.push({ kind: 'header', group })
      lastGroup = group
    }
    entries.push({ kind: 'cmd', cmd, index: i })
  })
  return entries
})

// ── Keyboard navigation ───────────────────────────────────────────────────────

function onKeydown(e: KeyboardEvent): void {
  const len = results.value.length
  if (e.key === 'ArrowDown') {
    e.preventDefault()
    selectedIndex.value = len ? (selectedIndex.value + 1) % len : 0
  } else if (e.key === 'ArrowUp') {
    e.preventDefault()
    selectedIndex.value = len ? (selectedIndex.value - 1 + len) % len : 0
  } else if (e.key === 'Enter') {
    // preventDefault so a focused row button doesn't also fire its click.
    e.preventDefault()
    const cmd = results.value[selectedIndex.value]
    if (cmd) run(cmd)
  } else if (e.key === 'Escape') {
    toggle()
  }
}
</script>

<template>
  <Teleport to="body">
    <Transition name="palette">
      <div v-if="open" class="palette-backdrop" @click.self="toggle">
        <!-- Dimmed backdrop — click outside to close -->
        <div class="palette-backdrop-fill" @click="toggle" />
        <!-- Panel -->
        <!-- Keydown lives on the panel (not the input) so navigation and
             Escape keep working when focus moves to a result row. -->
        <div
          class="palette-panel"
          data-testid="command-palette"
          role="dialog"
          aria-label="Command palette"
          aria-modal="true"
          @keydown="onKeydown"
        >
          <!-- Input row -->
          <div class="palette-input-row">
            <IconSearch class="palette-search-icon size-3.5" />
            <input
              ref="inputRef"
              v-model="query"
              type="text"
              placeholder="Search or run a command…"
              class="palette-input"
              data-testid="command-palette-input"
              autocomplete="off"
              spellcheck="false"
            />
            <kbd class="palette-kbd">⌘K</kbd>
          </div>

          <!-- Results list -->
          <div class="hive-scroll palette-results">
            <template v-for="(entry, i) in displayList" :key="i">
              <div v-if="entry.kind === 'header'" class="palette-group-header">
                {{ entry.group }}
              </div>
              <button
                v-else
                :ref="(el) => setRowRef(el as Element | ComponentPublicInstance | null, entry.index)"
                class="palette-row"
                data-testid="command-palette-command"
                :class="{ 'palette-row-selected': entry.index === selectedIndex }"
                @click="run(entry.cmd)"
                @mousemove="selectedIndex = entry.index"
              >
                <span class="min-w-0 flex-1 truncate text-left">{{ entry.cmd.title }}</span>
                <kbd v-if="entry.cmd.kbd" class="palette-row-kbd">{{ entry.cmd.kbd }}</kbd>
              </button>
            </template>

            <div v-if="results.length === 0 && query" class="palette-empty">
              No results for "{{ query }}"
            </div>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
/* Overlay */
.palette-backdrop {
  position: fixed;
  inset: 0;
  z-index: 50;
  display: flex;
  align-items: flex-start;
  justify-content: center;
  padding-top: 20vh;
}

.palette-backdrop-fill {
  position: absolute;
  inset: 0;
  background: var(--color-backdrop);
}

/* Panel */
.palette-panel {
  position: relative;
  z-index: 1;
  width: 440px;
  max-height: 480px;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  border-radius: 12px;
  border: 1px solid var(--color-strong);
  background: var(--color-chip);
  box-shadow: 0 25px 60px var(--color-backdrop), 0 0 0 1px var(--color-border);
}

/* Input row */
.palette-input-row {
  display: flex;
  align-items: center;
  gap: 10px;
  border-bottom: 1px solid var(--color-border);
  padding: 12px 16px;
  flex-shrink: 0;
}

.palette-search-icon {
  color: var(--color-text-4);
  font-family: var(--font-mono);
  font-size: 15px;
  flex-shrink: 0;
  user-select: none;
  line-height: 1;
}

.palette-input {
  flex: 1;
  background: transparent;
  border: none;
  outline: none;
  font-family: var(--font-mono);
  font-size: 13px;
  color: var(--color-text);
  min-width: 0;
  caret-color: var(--color-accent);
}

.palette-input::placeholder {
  color: var(--color-text-4);
}

.palette-kbd {
  flex-shrink: 0;
  font-family: var(--font-mono);
  font-size: 10px;
  color: var(--color-text-4);
  border: 1px solid var(--color-border);
  border-radius: 4px;
  padding: 2px 6px;
  background: var(--color-app);
  user-select: none;
  line-height: 1.6;
}

/* Results */
.palette-results {
  flex: 1;
  overflow-y: auto;
  padding: 6px 0;
}

/* Section header — amber mono uppercase, like sidebar section labels */
.palette-group-header {
  padding: 10px 16px 4px;
  font-family: var(--font-mono);
  font-size: 10px;
  font-weight: 600;
  letter-spacing: 0.12em;
  color: var(--color-accent);
  text-transform: uppercase;
  user-select: none;
}

/* Command row */
.palette-row {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  padding: 8px 16px;
  font-size: 13px;
  color: var(--color-text-2);
  cursor: pointer;
  border: none;
  background: transparent;
  text-align: left;
}

.palette-row:hover,
.palette-row-selected {
  background: var(--color-hover);
  color: var(--color-text);
}

.palette-row-kbd {
  font-family: var(--font-mono);
  font-size: 10px;
  color: var(--color-text-4);
  border: 1px solid var(--color-border);
  border-radius: 4px;
  padding: 2px 6px;
  background: var(--color-app);
  flex-shrink: 0;
  line-height: 1.6;
}

/* Empty state */
.palette-empty {
  padding: 20px 16px;
  font-family: var(--font-mono);
  font-size: 12px;
  color: var(--color-text-4);
  text-align: center;
}

/* Transition */
.palette-enter-active,
.palette-leave-active {
  transition: opacity 0.12s ease, transform 0.12s ease;
}

.palette-enter-from,
.palette-leave-to {
  opacity: 0;
  transform: translateY(-6px) scale(0.98);
}
</style>
