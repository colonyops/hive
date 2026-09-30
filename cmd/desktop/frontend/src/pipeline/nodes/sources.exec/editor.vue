<script setup lang="ts">
// sources.exec has no runtime.ts: the command runs in Go, on the poll tick.
import { computed } from 'vue'
import BaseButton from '../../../components/BaseButton.vue'
import { defaultExecSourceIcon, feedIconComponent, feedIconOptions } from '../../../lib/feedIcons'
import { IntervalField, MarkImageField, SelectField, TextField, TextareaField, type MarkImageClient } from '../../fields'
import IconTrash from '~icons/lucide/trash-2'
import type { Config } from './config'

const props = defineProps<{
  config: Config
  errors?: string[]
  /** The mark picker's backend seam, forwarded to MarkImageField for tests. */
  client?: MarkImageClient
}>()
const emit = defineEmits<{ 'update:config': [config: Config] }>()

const iconOptions = feedIconOptions.map((o) => ({ value: o.value, label: o.label, icon: o.component }))

function update(patch: Partial<Config>) {
  emit('update:config', { ...props.config, ...patch })
}

// An entry is [name, value]; the list is the editable form of the config's
// map, which has no order of its own.
const envEntries = computed<Array<[string, string]>>(() => Object.entries(props.config.env ?? {}))

function updateEnv(entries: Array<[string, string]>) {
  const env: Record<string, string> = {}
  for (const [name, value] of entries) {
    if (name.trim()) env[name.trim()] = value
  }
  update({ env: Object.keys(env).length > 0 ? env : undefined })
}

function setEnvName(index: number, name: string) {
  const entries = envEntries.value.map((entry, i): [string, string] => (i === index ? [name, entry[1]] : entry))
  updateEnv(entries)
}

function setEnvValue(index: number, value: string) {
  const entries = envEntries.value.map((entry, i): [string, string] => (i === index ? [entry[0], value] : entry))
  updateEnv(entries)
}

function removeEnv(index: number) {
  updateEnv(envEntries.value.filter((_, i) => i !== index))
}

// A blank row is only a rendering concern until it is named — updateEnv drops
// unnamed entries, so an added-and-abandoned row never reaches the config.
function addEnv() {
  emit('update:config', { ...props.config, env: { ...(props.config.env ?? {}), '': '' } })
}

// The mark picker's fallback preview, and what items render with no image set.
const iconGlyph = computed(() => feedIconComponent(props.config.icon || defaultExecSourceIcon))
</script>

<template>
  <div class="flex flex-col gap-4">
    <p class="rounded-lg border border-strong bg-app px-3 py-2.5 text-[11.5px] text-text-3" data-testid="sources.exec-editor-notice">
      This node runs a command on your machine every poll, with your environment. Treat a flow file from
      elsewhere the way you would treat a shell script from the same place.
    </p>
    <TextareaField
      label="Command"
      :model-value="config.command ?? ''"
      placeholder="gcx irm oncall alert-groups list -o json"
      hint="Run through sh -c on each poll tick. It must print a JSON array of items, each with a top-level id, and exit 0."
      monospace
      testid="sources.exec-editor-command"
      @update:model-value="(command: string) => update({ command })"
    />
    <TextField
      label="Timeout"
      :model-value="config.timeout ?? ''"
      placeholder="30s"
      hint="How long one run may take before it is killed and the tick fails. At most 2m."
      monospace
      testid="sources.exec-editor-timeout"
      @update:model-value="(timeout: string) => update({ timeout })"
    />
    <TextField
      label="Minimum interval"
      :model-value="config.interval ?? ''"
      placeholder="every poll"
      hint="Shortest time between runs, for a command not worth running every poll. Rounds up to the next tick."
      monospace
      testid="sources.exec-editor-interval"
      @update:model-value="(interval: string) => update({ interval: interval || undefined })"
    />
    <TextField
      label="Working directory"
      :model-value="config.cwd ?? ''"
      placeholder="~/src/repo"
      hint="Absolute path the command runs in. Empty runs in the app's own directory."
      monospace
      testid="sources.exec-editor-cwd"
      @update:model-value="(cwd: string) => update({ cwd: cwd || undefined })"
    />

    <div>
      <div class="mb-1.5 text-[12px] text-text-2">Environment</div>
      <div v-for="(entry, index) in envEntries" :key="index" class="mb-2 flex items-center gap-2">
        <input
          type="text"
          :value="entry[0]"
          placeholder="NAME"
          class="w-2/5 rounded-lg border border-strong bg-app px-3 py-2 font-mono text-[12.5px] text-text outline-none placeholder:text-text-4 focus:border-accent"
          :data-testid="`sources.exec-editor-env-name-${index}`"
          @input="setEnvName(index, ($event.target as HTMLInputElement).value)"
        >
        <input
          type="text"
          :value="entry[1]"
          placeholder="value"
          class="min-w-0 flex-1 rounded-lg border border-strong bg-app px-3 py-2 font-mono text-[12.5px] text-text outline-none placeholder:text-text-4 focus:border-accent"
          :data-testid="`sources.exec-editor-env-value-${index}`"
          @input="setEnvValue(index, ($event.target as HTMLInputElement).value)"
        >
        <button
          type="button"
          class="field-action"
          title="Remove variable"
          aria-label="Remove variable"
          :data-testid="`sources.exec-editor-env-remove-${index}`"
          @click="removeEnv(index)"
        ><IconTrash class="size-[14px]" /></button>
      </div>
      <BaseButton variant="secondary" size="sm" data-testid="sources.exec-editor-env-add" @click="addEnv">
        Add variable
      </BaseButton>
      <p class="mt-1.5 text-[11.5px] text-text-4">Added to the environment the command inherits. Values are literal — nothing is expanded.</p>
    </div>

    <SelectField
      label="Item icon"
      :model-value="config.icon || defaultExecSourceIcon"
      :options="iconOptions"
      searchable
      search-placeholder="Search icons…"
      hint="Shown on this source's items when no image is set."
      testid="sources.exec-editor-icon"
      @update:model-value="(icon: string) => update({ icon: icon || undefined })"
    />

    <MarkImageField
      :model-value="config.image"
      :glyph="iconGlyph"
      :client="props.client"
      testid="sources.exec-editor-mark"
      @update:model-value="(image: string | undefined) => update({ image })"
    />
    <IntervalField
      :model-value="config.interval"
      testid="sources.exec-editor-interval"
      @update:model-value="(interval?: string) => update({ interval })"
    />
  </div>
</template>
