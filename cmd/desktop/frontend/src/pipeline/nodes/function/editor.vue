<script setup lang="ts">
// The function node's editor: one CodeField for on_message — the node's whole
// lifecycle — plus outputs/timeout as compact footer chips alongside a live
// syntax-status chip. checkSyntax comes straight from config.ts, the same
// implementation the editor's own preview compiles with, so this live check
// can never drift from what it reports.
import { computed } from 'vue'
import { CodeField } from '../../fields'
import { DEFAULT_OUTPUTS, checkSyntax, recipes, type Config } from './config'

const props = defineProps<{ config: Config; errors?: string[] }>()
const emit = defineEmits<{ 'update:config': [config: Config] }>()

function set<K extends keyof Config>(key: K, value: Config[K]) {
  emit('update:config', { ...props.config, [key]: value })
}

const onMessageErrors = computed(() => checkSyntax(props.config.on_message ?? ''))

const outputsValue = computed(() => props.config.outputs ?? DEFAULT_OUTPUTS)

function onOutputsInput(e: Event) {
  const n = Number((e.target as HTMLInputElement).value)
  set('outputs', Number.isFinite(n) ? n : 0)
}

// timeout is stored in Config as milliseconds (D1); the field displays/edits
// it as a short duration string ("5s", "500ms") to match the YAML author
// experience — parse failures simply don't emit (the field stays editable;
// the previous valid value is what's actually saved).
function formatDurationMs(ms: number): string {
  return ms % 1000 === 0 ? `${ms / 1000}s` : `${ms}ms`
}

function parseDurationMs(text: string): number | undefined {
  const match = /^\s*(\d+(?:\.\d+)?)\s*(ms|s)\s*$/.exec(text)
  if (!match) return undefined
  const value = Number(match[1])
  return Math.round(match[2] === 's' ? value * 1000 : value)
}

const timeoutText = computed(() => formatDurationMs(props.config.timeout ?? 5000))

function onTimeoutInput(text: string) {
  const ms = parseDurationMs(text)
  if (ms !== undefined) set('timeout', ms)
}
</script>

<template>
  <div class="flex flex-col gap-4">
    <CodeField
      :model-value="config.on_message"
      label="on_message(msg, node, state, kv)"
      hint="Required. Return msg | msg[] | a port-indexed array | null (discard). kv is durable per-node storage."
      :error="onMessageErrors[0]"
      testid="function-editor-on-message"
      @update:model-value="(v) => set('on_message', v)"
    />

    <div class="flex flex-wrap items-center gap-2 text-[11px] text-text-3" data-testid="function-editor-recipes">
      Recipes
      <button
        v-for="recipe in recipes"
        :key="recipe.id"
        type="button"
        class="rounded-md border border-row bg-app px-2 py-1 text-text-2 hover:border-accent"
        :data-testid="`function-editor-recipe-${recipe.id}`"
        @click="set('on_message', recipe.code)"
      >{{ recipe.label }}</button>
    </div>

    <div class="flex flex-wrap items-center gap-2 font-mono text-[11px] text-text-3" data-testid="function-editor-footer-chips">
      <label class="inline-flex items-center gap-1.5 rounded-md border border-row bg-app px-2 py-1">
        Outputs
        <input
          type="number"
          min="1"
          max="16"
          :value="outputsValue"
          class="w-7 bg-transparent text-text-2 outline-none"
          data-testid="function-editor-outputs"
          @input="onOutputsInput"
        >
      </label>
      <label class="inline-flex items-center gap-1.5 rounded-md border border-row bg-app px-2 py-1">
        Timeout
        <input
          type="text"
          :value="timeoutText"
          placeholder="5s"
          class="w-10 bg-transparent text-text-2 outline-none"
          data-testid="function-editor-timeout"
          @input="(e) => onTimeoutInput((e.target as HTMLInputElement).value)"
        >
      </label>
      <span
        class="ml-1 inline-flex items-center gap-1"
        :class="onMessageErrors.length === 0 ? 'text-severity-success' : 'text-severity-error'"
        data-testid="function-editor-syntax-status"
      >{{ onMessageErrors.length === 0 ? '✓ no syntax errors' : `✕ ${onMessageErrors.length} syntax error${onMessageErrors.length === 1 ? '' : 's'}` }}</span>
    </div>
  </div>
</template>
