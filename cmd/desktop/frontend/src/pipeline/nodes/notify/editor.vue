<script setup lang="ts">
// Sound, severity and cooldown are stored only when they differ from the
// default, so a flow file stays free of keys the author never touched.
import { NumberField, SelectField, TextField, TextareaField, ToggleField } from '../../fields'
import { defaultCooldownSeconds, defaultSeverity, severities, type Config } from './config'

const props = defineProps<{ config: Config; errors?: string[] }>()
const emit = defineEmits<{ 'update:config': [config: Config] }>()

const severityOptions = severities.map((value) => ({
  value,
  label: value.charAt(0).toUpperCase() + value.slice(1),
}))

function setTitle(title: string) {
  emit('update:config', { ...props.config, title })
}

function setBody(body: string) {
  emit('update:config', { ...props.config, body: body || undefined })
}

function setSeverity(severity: string) {
  emit('update:config', { ...props.config, severity: severity === defaultSeverity ? undefined : severity })
}

function setSound(sound: boolean) {
  emit('update:config', { ...props.config, sound: sound ? undefined : false })
}

function setCooldown(cooldownSeconds: number) {
  emit('update:config', { ...props.config, cooldownSeconds: cooldownSeconds === defaultCooldownSeconds ? undefined : cooldownSeconds })
}
</script>

<template>
  <div class="flex flex-col gap-4 text-[13px] leading-relaxed" data-testid="notify-node-editor">
    <p class="text-text-2">
      Messages arriving here raise a system notification. Clicking it focuses
      Hive on the item that triggered it. The app's notification settings
      always win — a flow cannot notify while notifications are off.
    </p>

    <TextField
      label="Title"
      :model-value="config.title"
      placeholder="{{ .Payload.repo }} needs review"
      hint="Go template rendered over the message. `.Payload` is the item."
      monospace
      testid="notify-node-editor-title"
      @update:model-value="setTitle"
    />

    <TextareaField
      label="Body"
      :model-value="config.body ?? ''"
      placeholder="{{ .Payload.title }}"
      hint="Optional second line, rendered the same way."
      monospace
      testid="notify-node-editor-body"
      @update:model-value="setBody"
    />

    <SelectField
      label="Severity"
      :model-value="config.severity || defaultSeverity"
      :options="severityOptions"
      hint="Warning and error raise the notification's interruption level."
      testid="notify-node-editor-severity"
      @update:model-value="setSeverity"
    />

    <ToggleField
      label="Play sound"
      :model-value="config.sound ?? true"
      hint="Silences this node only; the global notification sound setting still applies."
      testid="notify-node-editor-sound"
      @update:model-value="setSound"
    />

    <NumberField
      label="Cooldown (seconds)"
      :model-value="config.cooldownSeconds ?? defaultCooldownSeconds"
      :min="0"
      :step="1"
      hint="Delivery floor per item, not dedup: after interrupting about an item this node stays quiet about it for this long. 0 disables the cooldown."
      testid="notify-node-editor-cooldown"
      @update:model-value="setCooldown"
    />
  </div>
</template>
