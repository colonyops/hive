<script setup lang="ts">
// The function node's editor: a CodeField per lifecycle hook (switched via
// TabStrip so on_start/on_stop don't have to stay always-visible), plus
// outputs/timeout. checkSyntax comes straight from config.ts — the same
// implementation the worker runtime compiles with (D2's single-source
// principle), so this live check can never drift from what actually runs.
import { computed, ref } from 'vue'
import { CodeField, NumberField, TabStrip, TextField } from '../../fields'
import { DEFAULT_OUTPUTS, checkSyntax, type Config } from './config'

const props = defineProps<{ config: Config; errors?: string[] }>()
const emit = defineEmits<{ 'update:config': [config: Config] }>()

const tabs = [
  { value: 'on_message', label: 'On message' },
  { value: 'on_start', label: 'On start' },
  { value: 'on_stop', label: 'On stop' },
]
const activeTab = ref('on_message')

function set<K extends keyof Config>(key: K, value: Config[K]) {
  emit('update:config', { ...props.config, [key]: value })
}

const onMessageErrors = computed(() => checkSyntax(props.config.on_message ?? ''))
const onStartErrors = computed(() => (props.config.on_start ? checkSyntax(props.config.on_start) : []))
const onStopErrors = computed(() => (props.config.on_stop ? checkSyntax(props.config.on_stop) : []))

const outputsValue = computed(() => props.config.outputs ?? DEFAULT_OUTPUTS)

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
    <div>
      <TabStrip v-model="activeTab" :tabs="tabs" testid="function-editor-tab" />
      <div class="mt-3">
        <CodeField
          v-if="activeTab === 'on_message'"
          :model-value="config.on_message"
          label="on_message(msg, node, state)"
          hint="Required. Return msg | msg[] | a port-indexed array | null (discard)."
          :error="onMessageErrors[0]"
          testid="function-editor-on-message"
          @update:model-value="(v) => set('on_message', v)"
        />
        <CodeField
          v-if="activeTab === 'on_start'"
          :model-value="config.on_start ?? ''"
          label="on_start(undefined, node, state)"
          hint="Optional. Runs once per instance before the first message."
          :error="onStartErrors[0]"
          testid="function-editor-on-start"
          @update:model-value="(v) => set('on_start', v || undefined)"
        />
        <CodeField
          v-if="activeTab === 'on_stop'"
          :model-value="config.on_stop ?? ''"
          label="on_stop(undefined, node, state)"
          hint="Optional. Runs once per instance on teardown (Deploy drain)."
          :error="onStopErrors[0]"
          testid="function-editor-on-stop"
          @update:model-value="(v) => set('on_stop', v || undefined)"
        />
      </div>
    </div>

    <div class="grid grid-cols-2 gap-3">
      <NumberField
        label="Outputs"
        :model-value="outputsValue"
        :min="1"
        :max="16"
        testid="function-editor-outputs"
        @update:model-value="(v) => set('outputs', v)"
      />
      <TextField
        label="Timeout"
        :model-value="timeoutText"
        placeholder="5s"
        hint="Go duration string, e.g. 5s, 500ms (100ms..60s)."
        testid="function-editor-timeout"
        @update:model-value="onTimeoutInput"
      />
    </div>
  </div>
</template>
