<script setup lang="ts">
// The comment labels, one row per key. A row saves when its title loses focus
// or its icon changes; a new row stays local until it has a title, and one
// left empty goes away.
import { nextTick, ref, watch } from 'vue'
import IconPlus from '~icons/lucide/plus'
import IconTrash2 from '~icons/lucide/trash-2'
import AppSelect from '@hive/ui/components/ui/AppSelect.vue'
import BaseButton from '@hive/ui/components/ui/BaseButton.vue'
import IconButton from '@hive/ui/components/ui/IconButton.vue'
import InlineError from '@hive/ui/components/ui/InlineError.vue'
import Kbd from '@hive/ui/components/ui/Kbd.vue'
import TextInput from '@hive/ui/components/ui/TextInput.vue'
import {
  defaultReviewLabelSettings,
  maxReviewLabels,
  reviewLabelIcons,
  type ReviewLabelSetting,
} from '../../lib/reviewLabels'
import { useReviewLabels } from '../../stores/useReviewLabels'

const store = useReviewLabels()
const rows = ref<ReviewLabelSetting[]>([])
const localError = ref<string | null>(null)
const inputs = ref<{ focus: () => void }[]>([])

watch(
  store.settings,
  (labels) => {
    rows.value = labels.map((label) => ({ ...label }))
  },
  { immediate: true },
)

const iconOptions = Object.entries(reviewLabelIcons).map(([value, icon]) => ({
  value,
  label: value.replace(/(^|-)(\w)/g, (_, dash: string, letter: string) => (dash ? ' ' : '') + letter.toUpperCase()),
  icon,
}))

const isDefault = () =>
  rows.value.length === defaultReviewLabelSettings.length &&
  rows.value.every(
    (row, index) =>
      row.title.trim() === defaultReviewLabelSettings[index].title &&
      row.icon === defaultReviewLabelSettings[index].icon,
  )

async function commit(): Promise<void> {
  rows.value = rows.value.filter((row) => row.title.trim())
  const titles = rows.value.map((row) => row.title.trim())
  const repeated = titles.find((title, index) => titles.indexOf(title) !== index)
  if (repeated) {
    localError.value = `“${repeated}” is listed twice.`
    return
  }
  localError.value = null
  if (!rows.value.length) {
    await store.save(defaultReviewLabelSettings)
    return
  }
  await store.save(rows.value)
}

async function add(): Promise<void> {
  if (rows.value.length >= maxReviewLabels) return
  rows.value.push({ title: '', icon: 'tag' })
  await nextTick()
  inputs.value.at(-1)?.focus()
}

function remove(index: number): void {
  rows.value.splice(index, 1)
  void commit()
}

function setIcon(index: number, icon: string): void {
  rows.value[index].icon = icon
  if (rows.value[index].title.trim()) void commit()
}
</script>

<template>
  <div class="flex flex-col gap-2 px-4 py-3.5" data-testid="settings-drive-labels">
    <div
      v-for="(row, index) in rows"
      :key="index"
      class="flex items-center gap-2"
      :data-testid="`settings-drive-label-${index + 1}`"
    >
      <Kbd class="w-5 text-center text-small text-text-3">{{ index + 1 }}</Kbd>
      <AppSelect
        class="w-[150px]"
        size="sm"
        :model-value="row.icon"
        :options="iconOptions"
        :aria-label="`Icon for label ${index + 1}`"
        :testid="`settings-drive-label-${index + 1}-icon`"
        @update:model-value="setIcon(index, $event)"
      />
      <TextInput
        ref="inputs"
        v-model="row.title"
        class="min-w-0 flex-1"
        size="sm"
        placeholder="Label title"
        :aria-label="`Title for label ${index + 1}`"
        :data-testid="`settings-drive-label-${index + 1}-title`"
        @blur="commit"
        @keydown.enter="($event.target as HTMLInputElement).blur()"
      />
      <IconButton
        :icon="IconTrash2"
        label="Remove label"
        tone="danger"
        :data-testid="`settings-drive-label-${index + 1}-remove`"
        @click="remove(index)"
      />
    </div>
    <InlineError variant="line" :message="localError ?? store.error.value" testid="settings-drive-labels-error" />
    <div class="flex items-center gap-2 pt-1">
      <BaseButton
        variant="secondary"
        size="xs"
        :disabled="rows.length >= maxReviewLabels"
        data-testid="settings-drive-labels-add"
        @click="add"
      >
        <template #icon><IconPlus class="size-3.5" aria-hidden="true" /></template>
        Add label
      </BaseButton>
      <BaseButton
        variant="ghost"
        size="xs"
        :disabled="isDefault()"
        data-testid="settings-drive-labels-reset"
        @click="store.save(defaultReviewLabelSettings)"
      >
        Reset to defaults
      </BaseButton>
    </div>
  </div>
</template>
