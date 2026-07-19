<script setup lang="ts">
// Searchable, category-grouped node palette (Phase 6b) — the app registry's
// `palette` (registry.ts) grouped by NodeCategory, filtered by label/type.
// Clicking an entry adds it to the active flow; an entry is also draggable
// (dragstart fires the same `add` — there is no canvas drop-target in v1,
// see FlowsCanvas.vue's module docs on wire creation for the same
// "basic first, full drag-and-drop later" posture).
import { computed, ref } from 'vue'
import IconSearch from '~icons/lucide/search'
import { palette } from '../registry'
import { summarize } from '../lib/markdown'
import type { NodeCategory, NodeTypeDefinition } from '../nodeType'

const emit = defineEmits<{ add: [type: string] }>()

const query = ref('')

const CATEGORIES: NodeCategory[] = ['Sources', 'Process', 'Destinations']

const filtered = computed<Record<NodeCategory, NodeTypeDefinition[]>>(() => {
  const q = query.value.trim().toLowerCase()
  const result = {} as Record<NodeCategory, NodeTypeDefinition[]>
  for (const category of CATEGORIES) {
    const defs = palette[category] ?? []
    result[category] = q ? defs.filter((def) => matches(def, q)) : defs
  }
  return result
})

function matches(def: NodeTypeDefinition, q: string): boolean {
  return def.label.toLowerCase().includes(q) || def.type.toLowerCase().includes(q)
}

const hasResults = computed(() => CATEGORIES.some((c) => filtered.value[c].length > 0))

function onDragStart(e: DragEvent, type: string) {
  e.dataTransfer?.setData('text/plain', type)
  emit('add', type)
}
</script>

<template>
  <div class="flex h-full flex-col" data-testid="node-palette">
    <div class="shrink-0 border-b border-row p-2.5">
      <div class="flex items-center gap-2 rounded-lg border border-strong bg-app px-2.5 py-2">
        <IconSearch class="size-3.5 shrink-0 text-text-4" />
        <input
          v-model="query"
          type="text"
          placeholder="filter nodes…"
          class="w-full min-w-0 bg-transparent text-[12.5px] text-text outline-none placeholder:text-text-4"
          data-testid="palette-search"
        >
      </div>
    </div>

    <div class="hive-scroll min-h-0 flex-1 overflow-y-auto p-2.5">
      <div v-if="!hasResults" class="px-1 py-6 text-center text-[12px] text-text-4" data-testid="palette-empty">
        No node types match &ldquo;{{ query }}&rdquo;
      </div>

      <template v-for="category in CATEGORIES" :key="category">
        <div v-if="filtered[category].length > 0" class="mb-3">
          <div class="mb-1.5 px-1 text-[10.5px] font-semibold uppercase tracking-wide text-text-4">{{ category }}</div>
          <button
            v-for="def in filtered[category]"
            :key="def.type"
            type="button"
            class="flex w-full cursor-grab items-center gap-2.5 rounded-lg border border-transparent px-2 py-1.5 text-left hover:border-strong hover:bg-hover active:cursor-grabbing"
            draggable="true"
            :title="summarize(def.help)"
            data-testid="palette-entry"
            :data-type="def.type"
            @click="emit('add', def.type)"
            @dragstart="onDragStart($event, def.type)"
          >
            <span
              class="flex size-[22px] shrink-0 items-center justify-center rounded-md"
              :style="{ background: def.tint ?? 'var(--color-accent-tint)', color: def.accentToken ?? 'var(--color-accent)' }"
            >
              <component :is="def.glyph" class="size-3.5" />
            </span>
            <span class="min-w-0 flex-1">
              <span class="block truncate text-[12.5px] font-medium text-text" data-testid="palette-entry-label">{{ def.label }}</span>
              <span class="block truncate text-[10.5px] text-text-4" data-testid="palette-entry-summary">{{ summarize(def.help) }}</span>
            </span>
            <span class="shrink-0 font-mono text-[12px] leading-none text-text-4" aria-hidden="true">⠿</span>
          </button>
        </div>
      </template>
    </div>
  </div>
</template>
