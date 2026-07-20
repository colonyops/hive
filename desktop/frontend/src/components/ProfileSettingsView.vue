<script setup lang="ts">
import { onMounted, onUnmounted } from 'vue'
import IconArrowLeft from '~icons/lucide/arrow-left'
import IconSlidersHorizontal from '~icons/lucide/sliders-horizontal'
import IconTrash2 from '~icons/lucide/trash-2'
import type { Profile } from '../types/feed'
import type { ProfileSettingsSection } from '../router'

const props = defineProps<{ profile: Profile; activeSection: ProfileSettingsSection }>()
const emit = defineEmits<{ close: []; delete: []; 'select-section': [section: ProfileSettingsSection] }>()

function onKeydown(e: KeyboardEvent): void {
  if (e.key === 'Escape') emit('close')
}

onMounted(() => window.addEventListener('keydown', onKeydown))
onUnmounted(() => window.removeEventListener('keydown', onKeydown))
</script>

<template>
  <div class="flex h-full min-h-0 flex-1" data-testid="profile-settings-view">
    <aside class="hive-scroll w-[200px] shrink-0 overflow-y-auto border-r border-row bg-sidebar">
      <div class="border-b border-border px-4 pb-3 pt-4">
        <div class="text-[15px] font-semibold tracking-[-.01em] text-text">Profile settings</div>
        <div class="mt-1 truncate text-xs text-text-3">{{ props.profile.name }}</div>
      </div>
      <nav class="flex flex-col gap-0.5 px-2.5 py-3">
        <button
          type="button"
          class="flex w-full cursor-pointer items-center gap-2.5 rounded-md px-2.5 py-2 text-left text-[13px]"
          :class="props.activeSection === 'general' ? 'bg-hover font-medium text-accent' : 'text-text-2 hover:bg-chip hover:text-text'"
          :aria-current="props.activeSection === 'general' ? 'true' : undefined"
          data-testid="profile-settings-general"
          @click="emit('select-section', 'general')"
        ><IconSlidersHorizontal class="size-3.5 shrink-0" />General</button>
        <button
          type="button"
          class="flex w-full cursor-pointer items-center gap-2.5 rounded-md px-2.5 py-2 text-left text-[13px]"
          :class="props.activeSection === 'danger' ? 'bg-hover font-medium text-severity-error' : 'text-text-2 hover:bg-chip hover:text-text'"
          :aria-current="props.activeSection === 'danger' ? 'true' : undefined"
          data-testid="profile-settings-danger"
          @click="emit('select-section', 'danger')"
        ><IconTrash2 class="size-3.5 shrink-0" />Danger zone</button>
      </nav>
    </aside>

    <section class="flex min-w-0 flex-1 flex-col">
      <header class="flex h-11 shrink-0 items-center gap-2.5 border-b border-row bg-canvas-toolbar px-4">
        <span class="text-[13px] font-semibold text-text">{{ props.activeSection === 'general' ? 'General' : 'Danger zone' }}</span>
        <div class="flex-1" />
        <button
          type="button"
          class="flex cursor-pointer items-center gap-1.5 rounded-md px-2.5 py-1.5 text-[12.5px] text-text-2 hover:bg-chip hover:text-text"
          data-testid="profile-settings-close"
          @click="emit('close')"
        ><IconArrowLeft class="size-3.5" />Back to feed</button>
      </header>

      <div class="hive-scroll min-h-0 flex-1 overflow-y-auto px-6 py-6">
        <div class="mx-auto max-w-[560px]">
          <div v-if="props.activeSection === 'general'" class="rounded-lg border border-border bg-raised p-4">
            <div class="text-[12.5px] text-text-3">Profile name</div>
            <div class="mt-1 text-[15px] font-semibold text-text" data-testid="profile-settings-name">{{ props.profile.name }}</div>
            <div class="mt-3 border-t border-border pt-3 text-xs text-text-3">{{ props.profile.sourceSummary }}</div>
          </div>

          <div v-else class="rounded-lg border border-severity-error/35 bg-raised p-4">
            <div class="text-[14px] font-semibold text-text">Delete profile</div>
            <p class="mt-1.5 text-xs leading-relaxed text-text-3">
              Permanently remove this profile, its flow file, and its committed feed items.
            </p>
            <button
              type="button"
              class="mt-4 flex cursor-pointer items-center gap-1.5 rounded-md bg-severity-error px-3 py-2 text-[12.5px] font-semibold text-accent-contrast hover:brightness-110"
              data-testid="profile-settings-delete"
              @click="emit('delete')"
            ><IconTrash2 class="size-3.5" />Delete profile</button>
          </div>
        </div>
      </div>
    </section>
  </div>
</template>
