<script setup lang="ts">
// The drawer shell every node type shares (D2), mirroring
// components/FeedEditorSheet.vue's structure: Teleport'd backdrop + right
// aside, name/disabled fields owned here, `def.editor` mounted over a
// structuredClone draft, live `def.validate`, a collapsible Docs section
// rendered from `def.help`, and Delete/Cancel/Done. Parent-owns-persistence
// — this never mutates the `node` prop; save() emits a whole new FlowNode.
import { computed, nextTick, onMounted, onUnmounted, ref, toRaw, watch } from 'vue'
import IconAlertTriangle from '~icons/lucide/alert-triangle'
import IconChevronDown from '~icons/lucide/chevron-down'
import IconChevronRight from '~icons/lucide/chevron-right'
import IconX from '~icons/lucide/x'
import { renderMarkdown, summarize } from '../lib/markdown'
import type { NodeTypeDefinition } from '../nodeType'
import type { FlowNode } from '../types'

const props = defineProps<{
  node: FlowNode
  def: NodeTypeDefinition
}>()

const emit = defineEmits<{
  save: [node: FlowNode]
  delete: [id: string]
  close: []
}>()

// ── Draft state — a deep clone of the incoming node; nothing here ever
// writes back into props.node. ──────────────────────────────────────────────

const name = ref('')
const disabled = ref(false)
const draftConfig = ref<Record<string, any>>({})

function loadFrom(node: FlowNode) {
  // toRaw first: `node` here is Vue's reactive proxy (props are reactive),
  // and structuredClone chokes on a Proxy in some environments (happy-dom
  // included) even though its target is a perfectly plain object.
  const raw = toRaw(node)
  name.value = raw.name ?? ''
  disabled.value = raw.disabled ?? false
  draftConfig.value = structuredClone(raw.config)
}

watch(() => props.node, (node) => loadFrom(node), { immediate: true })

function updateConfig(next: Record<string, any>) {
  draftConfig.value = next
}

const errors = computed(() => props.def.validate?.(draftConfig.value) ?? [])

function buildNode(): FlowNode {
  return {
    id: props.node.id,
    type: props.node.type,
    name: name.value.trim() || undefined,
    disabled: disabled.value,
    config: draftConfig.value,
  }
}

function submit() {
  emit('save', buildNode())
}

// ── Delete — two-step inline confirm, no modal (per FeedEditorSheet). ───────

const deleteConfirming = ref(false)

function confirmDelete() {
  emit('delete', props.node.id)
  deleteConfirming.value = false
}

// ── Docs ─────────────────────────────────────────────────────────────────────

const docsOpen = ref(false)
const docsHtml = computed(() => renderMarkdown(props.def.help))
const helpSummary = computed(() => summarize(props.def.help))

// ── Lifecycle ────────────────────────────────────────────────────────────────

const nameRef = ref<HTMLInputElement | null>(null)

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape') emit('close')
}

onMounted(async () => {
  window.addEventListener('keydown', onKeydown)
  await nextTick()
  nameRef.value?.focus()
})
onUnmounted(() => window.removeEventListener('keydown', onKeydown))
</script>

<template>
  <Teleport to="body">
    <div class="fixed inset-0 z-40 bg-backdrop" data-testid="node-editor-backdrop" @click.self="emit('close')">
      <aside
        class="absolute bottom-0 right-0 top-0 flex w-[620px] flex-col border-l border-strong bg-pane text-text shadow-[-30px_0_80px_-20px_rgba(0,0,0,.7)]"
        role="dialog"
        :aria-label="`Edit ${def.label}`"
        aria-modal="true"
        data-testid="node-editor"
      >
        <header class="flex shrink-0 items-center gap-3 border-b border-row bg-pane px-[22px] py-[18px]">
          <span class="flex size-[30px] shrink-0 items-center justify-center rounded-lg bg-accent-tint text-accent">
            <component :is="def.glyph" class="size-4" />
          </span>
          <div class="flex-1 text-[16px] font-semibold tracking-[-.01em]" data-testid="node-editor-title">{{ def.label }}</div>
          <button class="cursor-pointer text-text-3 hover:text-text" aria-label="Close" data-testid="node-editor-close" @click="emit('close')"><IconX class="size-4.5" /></button>
        </header>

        <div class="hive-scroll flex min-h-0 flex-1 flex-col gap-6 overflow-y-auto px-[22px] py-[22px]">
          <div>
            <div class="mb-1.5 text-[12.5px] text-text-2">Name</div>
            <input
              ref="nameRef"
              v-model="name"
              type="text"
              :placeholder="def.label"
              class="w-full rounded-lg border border-strong bg-app px-3 py-2.5 text-[13.5px] text-text outline-none placeholder:text-text-4 focus:border-accent"
              data-testid="node-editor-name"
              @keydown.enter="submit"
            >
          </div>

          <label class="flex cursor-pointer items-center gap-2 text-[13px]" :class="!disabled ? 'text-text' : 'text-text-2'">
            <input v-model="disabled" type="checkbox" class="accent-accent" data-testid="node-editor-disabled">
            Disabled — every message reaching this node becomes a discard
          </label>

          <div class="h-px shrink-0 bg-row" />

          <component
            :is="def.editor"
            :config="draftConfig"
            :errors="errors"
            data-testid="node-editor-body"
            @update:config="updateConfig"
          />

          <div v-if="errors.length > 0" class="flex items-start gap-2.5 rounded-lg border border-accent/40 bg-selection px-3 py-2.5" data-testid="node-editor-errors">
            <IconAlertTriangle class="mt-0.5 size-4 shrink-0 text-accent" />
            <ul class="text-xs leading-relaxed text-text-2">
              <li v-for="(err, i) in errors" :key="i">{{ err }}</li>
            </ul>
          </div>

          <div class="h-px shrink-0 bg-row" />

          <div>
            <button
              class="flex w-full cursor-pointer items-center gap-2 text-left"
              data-testid="node-editor-docs-toggle"
              @click="docsOpen = !docsOpen"
            >
              <component :is="docsOpen ? IconChevronDown : IconChevronRight" class="size-3.5 text-text-3" />
              <span class="text-[12.5px] font-semibold text-text">Docs</span>
              <span v-if="!docsOpen" class="truncate text-[11.5px] text-text-4" data-testid="node-editor-docs-summary">{{ helpSummary }}</span>
            </button>
            <div v-if="docsOpen" class="hive-doc mt-3 text-[13px] leading-relaxed text-text-2" data-testid="node-editor-docs" v-html="docsHtml" />
          </div>
        </div>

        <footer class="flex shrink-0 items-center gap-2.5 border-t border-row bg-raised px-[22px] py-4">
          <div class="flex items-center gap-2.5">
            <button
              v-if="!deleteConfirming"
              class="cursor-pointer whitespace-nowrap text-[13.5px] text-kind-issue hover:brightness-110"
              data-testid="node-editor-delete"
              @click="deleteConfirming = true"
            >Delete node</button>
            <template v-else>
              <span class="whitespace-nowrap text-[13px] text-text-3">Confirm delete?</span>
              <button
                class="cursor-pointer whitespace-nowrap text-[13.5px] font-semibold text-kind-issue hover:brightness-110"
                data-testid="node-editor-delete-confirm"
                @click="confirmDelete"
              >Delete</button>
              <button
                class="cursor-pointer whitespace-nowrap text-[13.5px] text-text-3 hover:text-text"
                data-testid="node-editor-delete-cancel"
                @click="deleteConfirming = false"
              >Cancel</button>
            </template>
          </div>
          <button
            class="flex-1 cursor-pointer rounded-lg bg-accent px-4 py-2.5 text-[13.5px] font-semibold text-accent-contrast hover:brightness-110"
            data-testid="node-editor-save"
            @click="submit"
          >Done</button>
          <button
            class="cursor-pointer whitespace-nowrap rounded-lg border border-card px-4 py-2.5 text-[13.5px] text-text-2 hover:text-text"
            data-testid="node-editor-cancel"
            @click="emit('close')"
          >Cancel</button>
        </footer>
      </aside>
    </div>
  </Teleport>
</template>

<style scoped>
/* Minimal doc typography for the hand-rolled markdown -> HTML output
   (lib/markdown.ts) — no classes are added there since it stays a generic
   renderer, so headings/lists/code get their look from here instead. */
.hive-doc :deep(h1),
.hive-doc :deep(h2),
.hive-doc :deep(h3) {
  margin: 1em 0 0.4em;
  font-weight: 600;
  color: var(--color-text);
}

.hive-doc :deep(h1:first-child),
.hive-doc :deep(h2:first-child),
.hive-doc :deep(h3:first-child) {
  margin-top: 0;
}

.hive-doc :deep(p) {
  margin: 0.6em 0;
}

.hive-doc :deep(ul),
.hive-doc :deep(ol) {
  margin: 0.6em 0;
  padding-left: 1.25em;
}

.hive-doc :deep(li) {
  margin: 0.2em 0;
}

.hive-doc :deep(code) {
  border-radius: 4px;
  background: var(--color-chip);
  padding: 0.1em 0.35em;
  font-family: var(--font-mono);
  font-size: 0.92em;
}

.hive-doc :deep(pre) {
  overflow-x: auto;
  border-radius: 8px;
  border: 1px solid var(--color-row);
  background: var(--color-app);
  padding: 0.65em 0.85em;
}

.hive-doc :deep(pre code) {
  background: none;
  padding: 0;
}
</style>
