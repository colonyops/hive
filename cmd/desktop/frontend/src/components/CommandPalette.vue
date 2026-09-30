<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import type { ComponentPublicInstance } from 'vue'
import IconSearch from '~icons/lucide/search'
import IconZap from '~icons/lucide/zap'
import AppIcon from './AppIcon.vue'
import { fuzzyMatch, useCommandPalette, type Command } from '../composables/useCommands'
import { usePaletteRecents } from '../composables/usePaletteRecents'
import { paletteScopes, type PaletteScopeId } from '../palette/scopes'

const { open, query, scope, visibleScopes, results, toggle, run, setQuery, setScope, cycleScope, popScope } =
  useCommandPalette()
const { recentIds } = usePaletteRecents()

const placeholder = computed(
  () => paletteScopes.find((s) => s.id === scope.value)?.placeholder ?? 'Search or run a command…',
)

function selectScope(id: PaletteScopeId): void {
  setScope(id)
  inputRef.value?.focus()
}

// ── Display list (section headers + commands interleaved) ──────────────────────
//
// Declared before selection tracking below: selectedIndex's getter reads
// navList, and the watch(selectedIndex, ...) registered there evaluates that
// getter immediately at setup — navList must already be initialized by then.

interface TitleSegment { text: string; match: boolean }
interface HeaderEntry { kind: 'header'; group: string }
interface CmdEntry { kind: 'cmd'; cmd: Command; index: number; segments: TitleSegment[]; groupPrefix: string }
type DisplayEntry = HeaderEntry | CmdEntry

/**
 * Per-character highlight segments from the same fuzzy matcher that ranks
 * results, so a scattered match (e.g. "mkalrd" on "Mark all as read")
 * highlights the letters it actually matched rather than a single run. A row
 * that matched via keywords/group rather than its title has no title match,
 * so it renders as one unmatched segment. Re-runs per rendered row per
 * keystroke — cheap enough that memoizing isn't worth the complexity.
 */
function titleSegments(title: string, query: string): TitleSegment[] {
  const positions = query ? fuzzyMatch(query, title)?.positions : undefined
  if (!positions || positions.length === 0) return [{ text: title, match: false }]

  const segments: TitleSegment[] = []
  let cursor = 0
  let i = 0
  while (i < positions.length) {
    if (positions[i] > cursor) segments.push({ text: title.slice(cursor, positions[i]), match: false })
    let end = positions[i] + 1
    let j = i + 1
    while (j < positions.length && positions[j] === end) {
      end++
      j++
    }
    segments.push({ text: title.slice(positions[i], end), match: true })
    cursor = end
    i = j
  }
  if (cursor < title.length) segments.push({ text: title.slice(cursor), match: false })
  return segments
}

// `index` on each CmdEntry is its position in DISPLAY order (this list, cmd
// entries only) — not its position in `results` — since Recent reorders rows
// relative to their group. navList below derives from this without reading
// selection, so assigning index here can't cycle back through selectedIndex.
const displayList = computed<DisplayEntry[]>(() => {
  const q = query.value.trim()
  const entries: DisplayEntry[] = []
  let navIndex = 0
  // Ranked results interleave groups, so section headers would mislabel the
  // rows under them; while filtering, each row carries its group as an inline
  // prefix instead.
  if (q) {
    results.value.forEach((cmd) => {
      entries.push({ kind: 'cmd', cmd, index: navIndex++, segments: titleSegments(cmd.title, q), groupPrefix: cmd.group ?? '' })
    })
    return entries
  }

  // Recent is device usage history, so it only makes sense against the
  // unfiltered All scope — a scoped tab is already a narrower question than
  // "what did I run recently". Stale ids (a deleted feed, a gone session) are
  // dropped rather than shown as dead rows.
  const recent = new Set<string>()
  if (scope.value === 'all') {
    const recentCommands = recentIds.value
      .map((id) => results.value.find((cmd) => cmd.id === id))
      .filter((cmd): cmd is Command => !!cmd)
    if (recentCommands.length) {
      entries.push({ kind: 'header', group: 'Recent' })
      for (const cmd of recentCommands) {
        recent.add(cmd.id)
        entries.push({ kind: 'cmd', cmd, index: navIndex++, segments: titleSegments(cmd.title, q), groupPrefix: cmd.group ?? '' })
      }
    }
  }

  let lastGroup: string | undefined = undefined
  results.value.forEach((cmd) => {
    // Shown above under Recent already — selection is held by id, so a
    // second row for it would snap the selection to whichever occurrence
    // comes first.
    if (recent.has(cmd.id)) return
    const group = cmd.group ?? ''
    if (group !== lastGroup) {
      if (group) entries.push({ kind: 'header', group })
      lastGroup = group
    }
    entries.push({ kind: 'cmd', cmd, index: navIndex++, segments: titleSegments(cmd.title, q), groupPrefix: '' })
  })
  return entries
})

// Keyboard/mouse navigation and Enter all walk this — display order — rather
// than `results` order, so the highlighted row always matches the visually
// top row and arrows move visually downward across the Recent/group boundary.
const navList = computed<Command[]>(() =>
  displayList.value.flatMap((entry) => (entry.kind === 'cmd' ? [entry.cmd] : [])),
)

// ── Selection tracking ────────────────────────────────────────────────────────

// The selection is a command, not a position. `results` (and so `navList`) is
// rebuilt whenever anything it reads changes — and in the Code view that
// includes session statuses, which poll — so a selection held as an index and
// reset on every rebuild walks back to the top under the user's own arrow
// keys. Holding the id instead means a rebuild that still contains the row
// leaves it selected, and one that does not falls back to the top.
const selectedID = ref<string | null>(null)
const inputRef = ref<HTMLInputElement | null>(null)
const rowElements = new Map<number, HTMLElement>()

const selectedIndex = computed<number>({
  get() {
    const at = navList.value.findIndex((cmd) => cmd.id === selectedID.value)
    return at >= 0 ? at : 0
  },
  set(index: number) {
    selectedID.value = navList.value[index]?.id ?? null
  },
})

// Typing is a new question, so it answers with the best match rather than
// keeping whatever was highlighted for the last one.
watch(query, () => {
  selectedID.value = null
  rowElements.clear()
})

// Switching tabs is a new question too: Tab/Shift+Tab, a tab click, a sigil
// entering a scope, and Backspace popping one all land here through the same
// `scope` ref, so resetting on it covers all four doors at once. This is
// deliberately narrower than the query watch above — a rebuild within the
// same scope (e.g. session statuses polling in the Code view) must not reset
// the selection, which is what holding it by id rather than index is for.
watch(scope, () => {
  selectedID.value = null
  rowElements.clear()
})

// Autofocus input when palette opens
watch(open, async (v) => {
  if (v) {
    selectedID.value = null
    rowElements.clear()
    pointerAt = null
    await nextTick()
    inputRef.value?.focus()
  }
})

// Scroll selected row into view. Gated on `open` — closed, the watch source
// is a constant -1 that never touches selectedIndex, so the
// selection/navList/results chain (which recomputes on every session-status
// poll in the Code view) is never evaluated for a scroll that has no row to
// land on. Arrows, hover, and the open watch's own reset above all still
// drive it normally once open.
watch(() => (open.value ? selectedIndex.value : -1), (idx) => {
  if (idx < 0) return
  nextTick(() => rowElements.get(idx)?.scrollIntoView({ block: 'nearest' }))
})

function setRowRef(el: Element | ComponentPublicInstance | null, index: number): void {
  if (el instanceof HTMLElement) rowElements.set(index, el)
  else rowElements.delete(index)
}

// WebKit refires a synthetic mousemove at the resting pointer after a
// keyboard-driven scroll shifts the rows beneath it; honoring that event
// would hand the arrow-key selection to whichever row slid under the cursor.
// Only a pointer that actually changed position selects — the first event
// after open just records where the pointer rests.
let pointerAt: { x: number; y: number } | null = null
function onRowPointerMove(e: MouseEvent, index: number): void {
  if (pointerAt && pointerAt.x === e.clientX && pointerAt.y === e.clientY) return
  const moved = pointerAt !== null
  pointerAt = { x: e.clientX, y: e.clientY }
  if (moved) selectedIndex.value = index
}

// setQuery's sigil interception can be a no-op state write (e.g. typing the
// active scope's own sigil again) — no reactive change, so Vue never
// re-renders the input to match `query`. Force it back in sync by hand.
function onInput(e: Event): void {
  const el = e.target as HTMLInputElement
  setQuery(el.value)
  if (el.value !== query.value) el.value = query.value
}

// ── Keyboard navigation ───────────────────────────────────────────────────────

function onKeydown(e: KeyboardEvent): void {
  const len = navList.value.length
  if (e.key === 'ArrowDown') {
    e.preventDefault()
    selectedIndex.value = len ? (selectedIndex.value + 1) % len : 0
  } else if (e.key === 'ArrowUp') {
    e.preventDefault()
    selectedIndex.value = len ? (selectedIndex.value - 1 + len) % len : 0
  } else if (e.key === 'Tab') {
    e.preventDefault()
    cycleScope(e.shiftKey ? -1 : 1)
  } else if (e.key === 'Enter') {
    // preventDefault so a focused row button doesn't also fire its click.
    e.preventDefault()
    const cmd = navList.value[selectedIndex.value]
    if (cmd) run(cmd)
  } else if (e.key === 'Escape') {
    toggle()
  } else if (e.key === 'Backspace') {
    // A non-empty query means Backspace is editing text, not leaving the
    // scope — popScope() already returns false for that case.
    if (popScope()) e.preventDefault()
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
          <!-- Scope tab strip — underline tabs, JetBrains-style -->
          <div class="palette-tabs" role="tablist">
            <button
              v-for="s in visibleScopes"
              :key="s.id"
              type="button"
              role="tab"
              class="palette-tab"
              :class="{ 'palette-tab-active': s.id === scope }"
              :aria-selected="s.id === scope"
              data-testid="command-palette-tab"
              :data-scope="s.id"
              @click="selectScope(s.id)"
            >
              <span v-if="s.sigil" class="palette-tab-sigil">{{ s.sigil }}</span>
              {{ s.label }}
            </button>
          </div>

          <!-- Input row -->
          <div class="palette-input-row">
            <IconSearch class="palette-search-icon" />
            <input
              ref="inputRef"
              :value="query"
              type="text"
              :placeholder="placeholder"
              class="palette-input"
              data-testid="command-palette-input"
              autocomplete="off"
              spellcheck="false"
              @input="onInput"
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
                @mousemove="onRowPointerMove($event, entry.index)"
              >
                <span class="palette-chip" aria-hidden="true">
                  <AppIcon
                    v-if="!entry.cmd.icon && entry.cmd.iconName"
                    :name="entry.cmd.iconName"
                    :style="entry.cmd.iconColor ? { color: entry.cmd.iconColor } : undefined"
                  />
                  <component :is="entry.cmd.icon ?? IconZap" v-else />
                </span>
                <span v-if="entry.groupPrefix" class="palette-scope" data-testid="command-palette-command-scope">{{ entry.groupPrefix }} ›</span>
                <span class="palette-title" data-testid="command-palette-command-title"><template v-for="(seg, si) in entry.segments" :key="si"><span v-if="seg.match" class="palette-title-match">{{ seg.text }}</span><template v-else>{{ seg.text }}</template></template></span>
                <span v-if="entry.cmd.kind" class="palette-kind" data-testid="command-palette-command-kind">{{ entry.cmd.kind }}</span>
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
            <span><span class="palette-footer-key">⇥</span> scope</span>
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
  width: 720px;
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

/* Scope tab strip */
.palette-tabs {
  display: flex;
  align-items: flex-end;
  gap: 4px;
  padding: 10px 14px 0;
  border-bottom: 1px solid var(--color-row);
  flex-shrink: 0;
}

.palette-tab {
  display: flex;
  align-items: center;
  gap: 5px;
  margin-bottom: -1px;
  padding: 6px 10px 8px;
  border: none;
  border-bottom: 2px solid transparent;
  background: transparent;
  font-family: var(--font-sans);
  font-size: 12px;
  color: var(--color-text-3);
  cursor: pointer;
}

.palette-tab:hover {
  color: var(--color-text-2);
}

.palette-tab-active,
.palette-tab-active:hover {
  color: var(--color-text);
  border-bottom-color: var(--color-accent);
}

.palette-tab-sigil {
  font-family: var(--font-mono);
  color: var(--color-text-4);
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

/* Scope prefix while filtering — the group, read per row instead of as a header */
.palette-scope {
  flex-shrink: 0;
  max-width: 40%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--color-text-3);
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

/* Muted object-type word — what Enter lands on. Not mono: it is a word,
   where the hint beside it is a key. */
.palette-kind {
  flex-shrink: 0;
  font-size: 11px;
  color: var(--color-text-3);
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
