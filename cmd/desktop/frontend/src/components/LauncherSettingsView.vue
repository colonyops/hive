<script setup lang="ts">
import { computed, ref } from 'vue'
import IconPlus from '~icons/lucide/plus'
import InlineError from './ui/InlineError.vue'
import SettingsHeading from './settings/SettingsHeading.vue'
import SettingsPage from './settings/SettingsPage.vue'
import BaseBadge from './ui/BaseBadge.vue'
import BaseButton from './ui/BaseButton.vue'
import ConfigItemCard from './settings/ConfigItemCard.vue'
import ConfirmationHost from './ui/ConfirmationHost.vue'
import LauncherEditor from './LauncherEditor.vue'
import EmptyState from './ui/EmptyState.vue'
import { useConfirmation } from '../composables/useConfirmation'
import { launcherIconComponent } from '../lib/launcherIcons'
import { formatCombo, useKeybindings } from '../composables/useKeybindings'
import { launcherCommandID } from '../keybindings/catalog'
import { useActionsSettings, type Launcher } from '../composables/useActionsSettings'

const { launchers, loading, error, createLauncher, updateLauncher, removeLauncher } = useActionsSettings()
const kb = useKeybindings()
const editing = ref<Launcher | null>(null)
const editorTrigger = ref<HTMLElement | null>(null)
const saving = ref(false)
const confirmation = useConfirmation()
const isNew = computed(() => !editing.value || !launchers.value.some((l) => l.id === editing.value?.id))

function blank(): Launcher {
  return { id: '', label: '', command: '', cwd: '', icon: '' }
}
function shortcut(id: string): string {
  return formatCombo(kb.bindings.value[launcherCommandID(id)]?.[0] ?? '')
}
function setEditorTrigger(event: MouseEvent): void {
  editorTrigger.value = event.currentTarget instanceof HTMLElement ? event.currentTarget : null
}
function createNew(event: MouseEvent): void {
  setEditorTrigger(event)
  editing.value = blank()
}
function edit(launcher: Launcher, event: MouseEvent): void {
  setEditorTrigger(event)
  editing.value = { ...launcher }
}
async function save(): Promise<void> {
  if (!editing.value || saving.value) return
  saving.value = true
  try {
    const saved = isNew.value
      ? await createLauncher(editing.value)
      : await updateLauncher(editing.value.id, editing.value)
    if (saved) editing.value = null
  } finally {
    saving.value = false
  }
}
function requestDelete(launcher: Launcher): void {
  confirmation.request({
    title: 'Delete quick terminal',
    description: `Delete ${launcher.label}? Any shortcut bound to it stops working.`,
    confirmLabel: 'Delete quick terminal',
    onConfirm: async () => {
      if (!(await removeLauncher(launcher.id))) throw new Error(error.value || 'Could not delete this quick terminal.')
    },
  })
}
</script>

<template>
  <SettingsPage testid="launchers-settings">
    <SettingsHeading
      title="Quick terminals"
      description="Open the pop-up terminal straight into a program — lazygit where the terminal you are looking at is, a test watcher, btop. Each one gets a command in the palette and can take a shortcut of its own."
    >
      <template #actions>
        <BaseButton size="sm" data-testid="launcher-create" @click="createNew">
          <template #icon><IconPlus class="size-3.5" :stroke-width="2.4" /></template>New quick terminal
        </BaseButton>
      </template>
    </SettingsHeading>

    <InlineError v-if="error && !editing" :message="error" testid="launchers-error" />
    <p v-if="loading" class="text-xs text-text-4">Loading quick terminals…</p>

    <div v-else class="flex flex-col gap-3">
      <ConfigItemCard
        v-for="launcher in launchers"
        :key="launcher.id"
        :title="launcher.label"
        :icon="launcherIconComponent(launcher.icon)"
        :data-testid="`launcher-row-${launcher.id}`"
        @edit="edit(launcher, $event)"
        @delete="requestDelete(launcher)"
      >
        <template #badges>
          <BaseBadge class="border border-row !bg-app px-[7px] py-0.5 font-mono text-caption">{{
            launcher.command
          }}</BaseBadge>
          <BaseBadge v-if="launcher.cwd" class="px-2 py-0.5 font-mono text-caption !text-text-2">{{
            launcher.cwd
          }}</BaseBadge>
          <BaseBadge class="px-2 py-0.5 text-caption" :data-testid="`launcher-shortcut-${launcher.id}`">
            <span
              class="size-1.5 rounded-full"
              :class="shortcut(launcher.id) ? 'bg-severity-success' : 'bg-text-4'"
            />{{ shortcut(launcher.id) || 'Unbound' }}
          </BaseBadge>
        </template>
      </ConfigItemCard>

      <EmptyState v-if="!launchers.length" message="No quick terminals configured." />
      <div
        v-else
        class="mt-1 flex items-center gap-1.5 font-mono text-caption text-text-4"
        data-testid="launchers-source"
      >
        Synced from .hive/actions.yml · {{ launchers.length }}
        {{ launchers.length === 1 ? 'quick terminal' : 'quick terminals' }}
      </div>
    </div>

    <LauncherEditor
      v-if="editing"
      v-model:launcher="editing"
      :is-new="isNew"
      :busy="saving"
      :error="error"
      :return-focus-to="editorTrigger"
      @save="save"
      @cancel="editing = null"
    />
    <ConfirmationHost :confirmation="confirmation" />
  </SettingsPage>
</template>
