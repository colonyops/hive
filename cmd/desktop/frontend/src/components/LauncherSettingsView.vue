<script setup lang="ts">
import { computed, ref } from 'vue'
import IconPlus from '~icons/lucide/plus'
import IconTrash2 from '~icons/lucide/trash-2'
import SettingsError from './settings/SettingsError.vue'
import SettingsHeading from './settings/SettingsHeading.vue'
import SettingsPage from './settings/SettingsPage.vue'
import BaseBadge from './BaseBadge.vue'
import BaseButton from './BaseButton.vue'
import BaseCard from './BaseCard.vue'
import BaseIconBadge from './BaseIconBadge.vue'
import ConfirmationDialog from './ConfirmationDialog.vue'
import LauncherEditor from './LauncherEditor.vue'
import EmptyState from './settings/EmptyState.vue'
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

function blank(): Launcher { return { id: '', label: '', command: '', cwd: '', icon: '' } }
function shortcut(id: string): string { return formatCombo(kb.bindings.value[launcherCommandID(id)]?.[0] ?? '') }
function setEditorTrigger(event: MouseEvent): void { editorTrigger.value = event.currentTarget instanceof HTMLElement ? event.currentTarget : null }
function createNew(event: MouseEvent): void { setEditorTrigger(event); editing.value = blank() }
function edit(launcher: Launcher, event: MouseEvent): void { setEditorTrigger(event); editing.value = { ...launcher } }
async function save(): Promise<void> {
  if (!editing.value || saving.value) return
  saving.value = true
  try {
    const saved = isNew.value ? await createLauncher(editing.value) : await updateLauncher(editing.value.id, editing.value)
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
    onConfirm: async () => { if (!await removeLauncher(launcher.id)) throw new Error(error.value || 'Could not delete this quick terminal.') },
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

    <SettingsError v-if="error && !editing" :message="error" testid="launchers-error" />
    <p v-if="loading" class="text-xs text-text-4">Loading quick terminals…</p>

    <div v-else class="flex flex-col gap-3">
      <BaseCard
        v-for="launcher in launchers"
        :key="launcher.id"
        :padded="false"
        class="flex-wrap items-start gap-3 rounded-[11px] border border-card bg-raised px-4 py-3.5 transition-colors hover:border-strong @[600px]/pane:flex-nowrap @[600px]/pane:items-center @[600px]/pane:gap-4"
        :data-testid="`launcher-row-${launcher.id}`"
      >
        <template #icon>
          <BaseIconBadge :size="38" rounded="rounded-[10px]" class="border border-accent/35 bg-accent-tint text-accent">
            <component :is="launcherIconComponent(launcher.icon)" class="size-[17px]" />
          </BaseIconBadge>
        </template>
        <div class="min-w-0 flex-1">
          <div class="truncate text-[15px] font-semibold tracking-[-.01em] text-text">{{ launcher.label }}</div>
          <div class="mt-1.5 flex flex-wrap items-center gap-1.5">
            <BaseBadge class="border border-row !bg-app px-[7px] py-0.5 font-mono text-[11px]">{{ launcher.command }}</BaseBadge>
            <BaseBadge v-if="launcher.cwd" class="px-2 py-0.5 font-mono text-[11px] !text-text-2">{{ launcher.cwd }}</BaseBadge>
            <BaseBadge class="px-2 py-0.5 text-[11px]" :data-testid="`launcher-shortcut-${launcher.id}`">
              <span class="size-1.5 rounded-full" :class="shortcut(launcher.id) ? 'bg-severity-success' : 'bg-text-4'" />{{ shortcut(launcher.id) || 'Unbound' }}
            </BaseBadge>
          </div>
        </div>
        <template #actions>
          <div class="flex w-full items-center justify-end gap-2 @[600px]/pane:w-auto @[600px]/pane:shrink-0">
            <button class="rounded-[7px] border border-card px-3.5 py-1.5 text-[12.5px] text-text-2 hover:border-strong hover:text-text" @click="edit(launcher, $event)">Edit</button>
            <button class="flex size-[34px] items-center justify-center rounded-[7px] border border-card text-text-3 hover:border-severity-error-border hover:text-severity-error" aria-label="Delete" @click="requestDelete(launcher)"><IconTrash2 class="size-[15px]" /></button>
          </div>
        </template>
      </BaseCard>

      <EmptyState v-if="!launchers.length" message="No quick terminals configured." />
      <div v-else class="mt-1 flex items-center gap-1.5 font-mono text-[11.5px] text-text-4" data-testid="launchers-source">Synced from .hive/actions.yml · {{ launchers.length }} {{ launchers.length === 1 ? 'quick terminal' : 'quick terminals' }}</div>
    </div>

    <LauncherEditor v-if="editing" :launcher="editing" :is-new="isNew" :busy="saving" :error="error" :return-focus-to="editorTrigger" @save="save" @cancel="editing = null" />
    <ConfirmationDialog v-if="confirmation.open.value && confirmation.options.value" :title="confirmation.options.value.title" :description="confirmation.options.value.description" :confirm-label="confirmation.options.value.confirmLabel" :busy="confirmation.busy.value" :error="confirmation.error.value" @confirm="confirmation.confirm" @cancel="confirmation.cancel" />
  </SettingsPage>
</template>
