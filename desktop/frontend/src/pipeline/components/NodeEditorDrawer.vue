<script setup lang="ts">
// The node editor shell every node type shares: a Teleport'd, right-
// anchored full-height side sheet (Node-RED edit-panel style) — header
// (role glyph tile + "Edit node · <label>" + role/port subtitle + an
// Enabled toggle that's the inverse of FlowNode.disabled), a body that owns
// the Name field and mounts `def.editor` over a structuredClone draft
// (unchanged contract: {config, errors} in, `update:config` out), and a
// footer of Delete | Cancel | Done. The delete confirm is still two-step,
// but renders as a small popover anchored over the Delete button (not an
// inline swap) so the footer's Close/Save buttons never shift — see the
// "Delete" section below. Parent-owns-persistence — this never mutates the
// `node` prop; save() emits a whole new FlowNode.
//
// This is meant to become the app-wide editing-surface shape — a reusable
// right sheet, not a one-off for flow nodes — so keep the geometry generic
// (fixed right edge, full height, scrim) rather than anything flow-specific.
import { computed, nextTick, onMounted, onUnmounted, ref, toRaw, watch } from 'vue'
import IconAlertTriangle from '~icons/lucide/alert-triangle'
import IconChevronDown from '~icons/lucide/chevron-down'
import IconChevronRight from '~icons/lucide/chevron-right'
import { hasInputPort, outputPortCount } from '../lib/ports'
import { renderMarkdown, summarize } from '../lib/markdown'
import { useResizablePanel } from '../../composables/useResizablePanel'
import PanelResizeHandle from '../../components/PanelResizeHandle.vue'
import AppSwitch from '../../components/AppSwitch.vue'
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

const { size, startResize, step } = useResizablePanel({
  storageKey: 'hive.panel.node-editor',
  defaultSize: 440,
  min: 360,
  max: 760,
  edge: 'left',
})

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
const enabled = computed({ get: () => !disabled.value, set: (value: boolean) => { disabled.value = !value } })

// ── Header role subtitle ("source · emits 1 output" / "processor · 1 in →
// 1 out") — resolved against the live draft config so a function node's
// outputs count (config-dependent, see lib/ports.ts) updates as it's
// edited. ─────────────────────────────────────────────────────────────────

const subtitle = computed(() => {
  const draftNode: FlowNode = { ...props.node, config: draftConfig.value }
  const outCount = outputPortCount(props.def, draftNode)
  if (props.def.role === 'source') {
    return `source · emits ${outCount} output${outCount === 1 ? '' : 's'}`
  }
  const inCount = hasInputPort(props.def) ? 1 : 0
  return `${props.def.role} · ${inCount} in → ${outCount} out`
})

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

// ── Delete — two-step confirm via a small popover anchored over the Delete
// button, rather than an inline swap in the footer's flex row. An inline
// swap ("Delete" -> "Confirm delete? Delete Cancel") grows the footer's
// left-hand content and pushes Close/Save sideways as the flex-1 spacer
// shrinks; the popover keeps the footer's flex children constant so nothing
// else in the footer ever moves. ────────────────────────────────────────────

const deleteConfirming = ref(false)
const deleteTriggerRef = ref<HTMLButtonElement | null>(null)
const deleteCancelRef = ref<HTMLButtonElement | null>(null)

function requestDelete() {
  deleteConfirming.value = true
}

function cancelDelete() {
  deleteConfirming.value = false
  nextTick(() => deleteTriggerRef.value?.focus())
}

function confirmDelete() {
  emit('delete', props.node.id)
  deleteConfirming.value = false
}

watch(deleteConfirming, (confirming) => {
  if (confirming) nextTick(() => deleteCancelRef.value?.focus())
})

// ── Docs ─────────────────────────────────────────────────────────────────────

const docsOpen = ref(false)
const docsHtml = computed(() => renderMarkdown(props.def.help))
const helpSummary = computed(() => summarize(props.def.help))

// ── Lifecycle ────────────────────────────────────────────────────────────────

const nameRef = ref<HTMLInputElement | null>(null)

function onKeydown(e: KeyboardEvent) {
  if (e.key !== 'Escape') return
  // The delete popover takes precedence: Esc cancels the pending confirm
  // first, and only closes the whole drawer on a second Esc once the
  // popover is dismissed.
  if (deleteConfirming.value) {
    cancelDelete()
    return
  }
  emit('close')
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
    <div class="fixed inset-0 z-40 bg-backdrop" data-testid="node-editor-backdrop" @click="emit('close')" />
    <aside
      class="fixed inset-y-0 right-0 z-40 flex max-w-full flex-col overflow-hidden border-l border-strong bg-pane text-text shadow-[-30px_0_60px_-20px_rgba(0,0,0,.5)]"
      :style="{ width: size + 'px' }"
      role="dialog"
      :aria-label="`Edit ${def.label}`"
      aria-modal="true"
      data-testid="node-editor"
    >
      <PanelResizeHandle edge="left" name="node-editor" :start="startResize" :step="step" />
      <header class="flex shrink-0 items-center gap-[11px] border-b border-row bg-pane px-[18px] py-[15px]">
        <span
          class="flex size-[26px] shrink-0 items-center justify-center rounded-[7px]"
          :style="{ background: def.tint ?? 'var(--color-accent-tint)', color: def.accentToken ?? 'var(--color-accent)' }"
        >
          <component :is="def.glyph" class="size-3.5" />
        </span>
        <div class="min-w-0 flex-1">
          <div class="truncate text-[14px] font-semibold tracking-[-.01em]" data-testid="node-editor-title">Edit node · {{ def.label }}</div>
          <div class="truncate font-mono text-[11px] text-text-3" data-testid="node-editor-subtitle">{{ subtitle }}</div>
        </div>
        <AppSwitch v-model="enabled" label="Enabled" class="shrink-0" testid="node-editor-enabled" />
      </header>

      <div class="hive-scroll flex min-h-0 flex-1 flex-col gap-[14px] overflow-y-auto px-[18px] py-[15px]">
        <div>
          <div class="mb-1.5 text-[12px] text-text-2">Name</div>
          <input
            ref="nameRef"
            v-model="name"
            type="text"
            :placeholder="def.label"
            class="w-full rounded-lg border border-strong bg-app px-[11px] py-[9px] text-[13px] text-text outline-none placeholder:text-text-4 focus:border-accent"
            data-testid="node-editor-name"
            @keydown.enter="submit"
          >
        </div>

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

        <template v-if="def.help">
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
        </template>
      </div>

      <footer class="flex shrink-0 items-center gap-2.5 border-t border-row bg-raised px-[18px] py-[13px]">
        <div class="relative">
          <button
            ref="deleteTriggerRef"
            type="button"
            class="cursor-pointer whitespace-nowrap text-[12px] text-kind-issue hover:brightness-110"
            data-testid="node-editor-delete"
            :aria-expanded="deleteConfirming"
            @click="requestDelete"
          >Delete</button>

          <div
            v-if="deleteConfirming"
            class="absolute bottom-full left-0 z-10 mb-2 flex w-max flex-col gap-2.5 rounded-lg border border-strong bg-pane p-3 shadow-[0_12px_30px_-8px_rgba(0,0,0,.5)]"
            role="group"
            aria-label="Confirm delete node"
            data-testid="node-editor-delete-popover"
          >
            <div class="whitespace-nowrap text-[12px] text-text-2">Delete this node?</div>
            <div class="flex items-center gap-2">
              <button
                ref="deleteCancelRef"
                type="button"
                class="cursor-pointer whitespace-nowrap rounded-md border border-card px-2.5 py-1.5 text-[12px] text-text-2 hover:text-text"
                data-testid="node-editor-delete-cancel"
                @click="cancelDelete"
              >Cancel</button>
              <button
                type="button"
                class="cursor-pointer whitespace-nowrap rounded-md bg-severity-error px-2.5 py-1.5 text-[12px] font-semibold text-accent-contrast hover:brightness-110"
                data-testid="node-editor-delete-confirm"
                @click="confirmDelete"
              >Delete node</button>
            </div>
          </div>
        </div>
        <div class="flex-1" />
        <button
          class="cursor-pointer whitespace-nowrap rounded-lg border border-card px-[15px] py-2 text-[13px] text-text-2 hover:text-text"
          data-testid="node-editor-cancel"
          @click="emit('close')"
        >Cancel</button>
        <button
          class="cursor-pointer whitespace-nowrap rounded-lg bg-accent px-[18px] py-2 text-[13px] font-semibold text-accent-contrast hover:brightness-110"
          data-testid="node-editor-save"
          @click="submit"
        >Done</button>
      </footer>
    </aside>
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
