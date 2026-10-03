<script setup lang="ts">
// Shown once after an update lands, listing every release crossed to reach the
// running build — a user who skipped a version gets both sets of notes, not
// just the newest — plus the draft of what this build has that no stable
// release does.
import IconSparkles from '~icons/lucide/sparkles'
import BaseModal from './ui/BaseModal.vue'
import BaseButton from './ui/BaseButton.vue'
import ReleaseNoteBody from './ReleaseNoteBody.vue'
import type { ReleaseNote } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'

const props = defineProps<{ version: string; entries: ReleaseNote[] }>()

const emit = defineEmits<{ close: [] }>()
</script>

<template>
  <BaseModal
    :title="`What's new in ${props.version}`"
    :icon="IconSparkles"
    :width="600"
    testid="whats-new"
    @close="emit('close')"
  >
    <div class="flex flex-col gap-6 px-5 py-4">
      <p v-if="props.entries.length === 0" class="text-[13.5px] text-text-2">Hive updated to {{ props.version }}.</p>

      <section v-for="entry in props.entries" :key="entry.draft ? 'draft' : entry.version" class="flex flex-col gap-2">
        <!-- The version heading is suppressed for a single-release update: the
             dialog title already names it, and repeating it reads as a stutter.
             The draft is headed either way — its notes describe work that has
             not shipped a stable release, which the title does not say. -->
        <header v-if="entry.draft || props.entries.length > 1" class="flex items-baseline gap-2.5">
          <span class="font-mono text-[13px] font-semibold text-text">
            {{ entry.draft ? 'Unreleased' : entry.version }}
          </span>
          <span v-if="entry.date" class="text-[11px] text-text-3">{{ entry.date }}</span>
        </header>
        <p v-if="entry.summary" class="text-[13.5px] leading-[1.6] text-text">{{ entry.summary }}</p>
        <ReleaseNoteBody :body="entry.body" />
      </section>
    </div>

    <template #footer>
      <div class="flex flex-1 items-center justify-between">
        <span class="text-[11.5px] text-text-3">Settings &rsaquo; About keeps every release.</span>
        <BaseButton data-testid="whats-new-dismiss" @click="emit('close')">Got it</BaseButton>
      </div>
    </template>
  </BaseModal>
</template>
