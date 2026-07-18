<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import type { ComponentPublicInstance } from 'vue'
import IconSearch from '~icons/lucide/search'
import IconZap from '~icons/lucide/zap'
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

interface TitleParts { pre: string; match: string; post: string }
interface HeaderEntry { kind: 'header'; group: string }
interface CmdEntry { kind: 'cmd'; cmd: Command; index: number; parts: TitleParts }
type DisplayEntry = HeaderEntry | CmdEntry

/** Split a title around the first case-insensitive occurrence of the query. */
function titleParts(title: string, query: string): TitleParts {
  if (query) {
    const idx = title.toLowerCase().indexOf(query.toLowerCase())
    if (idx >= 0) {
      return {
        pre: title.slice(0, idx),
        match: title.slice(idx, idx + query.length),
        post: title.slice(idx + query.length),
      }
    }
  }
  return { pre: title, match: '', post: '' }
}

const displayList = computed<DisplayEntry[]>(() => {
  const q = query.value.trim()
  const entries: DisplayEntry[] = []
  let lastGroup: string | undefined = undefined
  results.value.forEach((cmd, i) => {
    const group = cmd.group ?? ''
    if (group !== lastGroup) {
      if (group) entries.push({ kind: 'header', group })
      lastGroup = group
    }
    entries.push({ kind: 'cmd', cmd, index: i, parts: titleParts(cmd.title, q) })
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
      <!-- Dimmed backdrop — click outside the panel to close -->
      <div v-if="open" class="palette-backdrop" @click.self="toggle">
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
            <IconSearch class="palette-search-icon" />
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
            <kbd class="palette-kbd">esc</kbd>
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
                <span class="palette-chip" aria-hidden="true">
                  <component :is="entry.cmd.icon ?? IconZap" />
                </span>
                <span class="palette-title">{{ entry.parts.pre }}<span class="palette-title-match">{{ entry.parts.match }}</span>{{ entry.parts.post }}</span>
                <span v-if="entry.cmd.hint" class="palette-hint">{{ entry.cmd.hint }}</span>
                <span v-if="entry.index === selectedIndex" class="palette-enter-badge" aria-hidden="true">↵</span>
              </button>
            </template>

            <div v-if="results.length === 0 && query" class="palette-empty">
              No results for "{{ query }}"
            </div>
          </div>

          <!-- Footer key hints -->
          <div class="palette-footer">
            <span><span class="palette-footer-key">↑↓</span> navigate</span>
            <span><span class="palette-footer-key">↵</span> run</span>
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
  padding-top: 12vh;
  background: var(--color-backdrop);
}

/* Panel */
.palette-panel {
  position: relative;
  z-index: 1;
  width: 660px;
  max-width: calc(100vw - 48px);
  max-height: min(608px, 76vh);
  display: flex;
  flex-direction: column;
  overflow: hidden;
  border-radius: 14px;
  border: 1px solid var(--color-strong);
  background: var(--color-pane);
  box-shadow: 0 40px 90px -20px var(--color-backdrop);
}

/* Input row */
.palette-input-row {
  display: flex;
  align-items: center;
  gap: 12px;
  border-bottom: 1px solid var(--color-row);
  padding: 16px 18px;
  flex-shrink: 0;
}

.palette-search-icon {
  width: 18px;
  height: 18px;
  color: var(--color-text-3);
  flex-shrink: 0;
  user-select: none;
}

.palette-input {
  flex: 1;
  background: transparent;
  border: none;
  outline: none;
  font-family: var(--font-sans);
  font-size: 17px;
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
  font-size: 11px;
  color: var(--color-text-3);
  border: 1px solid var(--color-card);
  border-radius: 5px;
  padding: 2px 7px;
  user-select: none;
  line-height: 1.5;
}

/* Results */
.palette-results {
  flex: 1;
  overflow-y: auto;
  padding: 0 8px 8px;
}

/* Section header — muted mono uppercase */
.palette-group-header {
  padding: 12px 6px 4px;
  font-family: var(--font-mono);
  font-size: 10px;
  font-weight: 600;
  letter-spacing: 0.12em;
  color: var(--color-text-3);
  text-transform: uppercase;
  user-select: none;
}

/* Command row */
.palette-row {
  display: flex;
  align-items: center;
  gap: 12px;
  width: 100%;
  padding: 9px 10px;
  border-radius: 9px;
  font-size: 14px;
  color: var(--color-text-2);
  cursor: pointer;
  border: none;
  background: transparent;
  text-align: left;
}

.palette-row:hover {
  background: var(--color-row);
}

.palette-row-selected,
.palette-row-selected:hover {
  background: var(--color-selection);
  color: var(--color-text);
}

/* Leading icon chip */
.palette-chip {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 26px;
  height: 26px;
  border-radius: 7px;
  background: var(--color-chip);
  color: var(--color-text-2);
  flex-shrink: 0;
}

.palette-chip svg {
  width: 14px;
  height: 14px;
}

/* Title with matched-substring highlight */
.palette-title {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.palette-title-match {
  color: var(--color-accent);
  font-weight: 600;
}

/* Right-aligned hint */
.palette-hint {
  flex-shrink: 0;
  font-family: var(--font-mono);
  font-size: 11px;
  color: var(--color-text-3);
}

/* Enter badge on the selected row */
.palette-enter-badge {
  flex-shrink: 0;
  font-size: 12px;
  font-weight: 600;
  color: var(--color-accent-contrast);
  background: var(--color-accent);
  border-radius: 6px;
  padding: 4px 9px;
  line-height: 1;
}

/* Empty state */
.palette-empty {
  padding: 20px 16px;
  font-family: var(--font-mono);
  font-size: 12px;
  color: var(--color-text-4);
  text-align: center;
}

/* Footer key hints */
.palette-footer {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 10px 16px;
  border-top: 1px solid var(--color-row);
  background: var(--color-raised);
  font-family: var(--font-mono);
  font-size: 11px;
  color: var(--color-text-3);
  flex-shrink: 0;
  user-select: none;
}

.palette-footer-key {
  color: var(--color-text-2);
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
