<script setup lang="ts">
import { ref, watch } from 'vue'
import IconSlidersHorizontal from '~icons/lucide/sliders-horizontal'
import IconTrash2 from '~icons/lucide/trash-2'
import AppSwitch from './AppSwitch.vue'
import BaseButton from './BaseButton.vue'
import SettingsLayout from './settings/SettingsLayout.vue'
import SettingsNavItem from './settings/SettingsNavItem.vue'
import type { Profile } from '../types/feed'
import type { ProfileSettingsSection } from '../router'

const props = withDefaults(defineProps<{
  profile: Profile
  activeSection: ProfileSettingsSection
  renaming?: boolean
  renameError?: string | null
  toggling?: boolean
  toggleError?: string | null
}>(), {
  renaming: false,
  renameError: null,
  toggling: false,
  toggleError: null,
})
const emit = defineEmits<{
  close: []
  delete: []
  rename: [name: string]
  'toggle-enabled': [enabled: boolean]
  'select-section': [section: ProfileSettingsSection]
}>()

const name = ref(props.profile.name)

watch(() => [props.profile.id, props.profile.name], () => {
  name.value = props.profile.name
})

function submitRename(): void {
  const trimmed = name.value.trim()
  if (!trimmed || trimmed === props.profile.name || props.renaming) return
  emit('rename', trimmed)
}
</script>

<template>
  <SettingsLayout close-testid="profile-settings-close" data-testid="profile-settings-view" @close="emit('close')">
    <template #sidebar-title>
      <div class="text-[15px] font-semibold tracking-[-.01em] text-text">Profile settings</div>
      <div class="mt-1 truncate text-xs text-text-3">{{ props.profile.name }}</div>
    </template>
    <template #nav>
      <SettingsNavItem
        :active="props.activeSection === 'general'"
        :icon="IconSlidersHorizontal"
        label="General"
        testid="profile-settings-general"
        @select="emit('select-section', 'general')"
      />
      <SettingsNavItem
        :active="props.activeSection === 'danger'"
        :icon="IconTrash2"
        label="Danger zone"
        testid="profile-settings-danger"
        tone="danger"
        @select="emit('select-section', 'danger')"
      />
    </template>
    <template #header-title>
      <span class="text-[13px] font-semibold text-text">{{ props.activeSection === 'general' ? 'General' : 'Danger zone' }}</span>
    </template>

    <div class="hive-scroll min-h-0 flex-1 overflow-y-auto px-6 py-6">
      <div class="mx-auto max-w-[560px]">
        <form v-if="props.activeSection === 'general'" class="rounded-lg border border-border bg-raised p-4" @submit.prevent="submitRename">
          <label for="profile-settings-name" class="text-[12.5px] text-text-3">Profile name</label>
          <div class="mt-2 flex items-center gap-2.5">
            <input
              id="profile-settings-name"
              v-model="name"
              type="text"
              class="min-w-0 flex-1 rounded-lg border border-strong bg-app px-3 py-2 text-[13.5px] text-text outline-none focus:border-accent disabled:opacity-60"
              :disabled="props.renaming"
              data-testid="profile-settings-name"
            >
            <BaseButton
              type="submit"
              size="sm"
              :busy="props.renaming"
              :disabled="!name.trim() || name.trim() === props.profile.name"
              data-testid="profile-settings-save-name"
            >{{ props.renaming ? 'Saving…' : 'Save' }}</BaseButton>
          </div>
          <p v-if="props.renameError" class="mt-2 text-xs text-severity-error" data-testid="profile-settings-rename-error">{{ props.renameError }}</p>
          <div class="mt-3 border-t border-border pt-3 text-xs text-text-3">{{ props.profile.sourceSummary }}</div>
          <div class="mt-4 flex items-start justify-between gap-5 border-t border-border pt-4">
            <div>
              <div class="text-[13px] font-medium text-text">Profile polling</div>
              <p class="mt-1 text-xs leading-relaxed text-text-3">Disabled profiles keep their existing feed items but stop polling and running their flow.</p>
            </div>
            <AppSwitch
              :model-value="props.profile.enabled"
              :label="props.profile.enabled ? 'Enabled' : 'Disabled'"
              aria-label="Profile polling"
              :disabled="props.toggling"
              testid="profile-settings-enabled"
              @update:model-value="emit('toggle-enabled', $event)"
            />
          </div>
          <p v-if="props.toggleError" class="mt-2 text-xs text-severity-error" data-testid="profile-settings-toggle-error">{{ props.toggleError }}</p>
        </form>

        <div v-else class="rounded-lg border border-severity-error/35 bg-raised p-4">
          <div class="text-[14px] font-semibold text-text">Delete profile</div>
          <p class="mt-1.5 text-xs leading-relaxed text-text-3">
            Permanently remove this profile, its flow file, and its committed feed items.
          </p>
          <BaseButton
            variant="danger"
            size="sm"
            class="mt-4"
            data-testid="profile-settings-delete"
            @click="emit('delete')"
          ><template #icon><IconTrash2 class="size-3.5" /></template>Delete profile</BaseButton>
        </div>
      </div>
    </div>
  </SettingsLayout>
</template>
