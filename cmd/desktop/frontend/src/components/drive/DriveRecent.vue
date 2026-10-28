<script setup lang="ts">
// The newest documents across every space, above the tree, so something an
// agent just wrote is found without knowing which space or folder holds it.
import { onMounted } from 'vue'
import InlineError from '@hive/ui/components/ui/InlineError.vue'
import { entryTime } from '@hive/ui/lib/document'
import DriveSection from './DriveSection.vue'
import { documentIcon } from './driveIcon'
import { relativeAge } from '../../lib/age'
import { entryLabel } from '../../lib/drive'
import { useDrive } from '../../stores/useDrive'
import { useDriveActivity } from '../../stores/useDriveActivity'
import type { HitWire } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'

const props = defineProps<{
  /** The space and path on screen, marked when listed. */
  space: string
  selected: string | null
  /** Derives `-recent`, `-recent-toggle`, `-recent-body`, `-recent-error`, and `-recent-<space>:<path>`. */
  testid: string
}>()
const emit = defineEmits<{ open: [space: string, path: string] }>()

const { recent, recentError, ensureRecent } = useDrive()
const activity = useDriveActivity()

onMounted(() => void ensureRecent())

const hitKey = (hit: HitWire) => `${hit.space.key}:${hit.entry.path}`
const isSelected = (hit: HitWire) => hit.space.key === props.space && hit.entry.path === props.selected
</script>

<template>
  <DriveSection
    v-if="recent.length || recentError"
    title="Recent"
    storage-key="hive.drive.recent.open"
    :testid="`${testid}-recent`"
  >
    <InlineError
      v-if="recentError"
      variant="line"
      class="px-3 py-1 leading-relaxed"
      :message="recentError"
      :testid="`${testid}-recent-error`"
    />
    <!-- Indented to line the icons up with the tree's top-level rows. -->
    <button
      v-for="hit in recent"
      :key="hitKey(hit)"
      type="button"
      class="flex h-[26px] w-full shrink-0 cursor-pointer items-center gap-1.5 pl-[26px] pr-2 text-left text-small"
      :class="isSelected(hit) ? 'bg-selection text-text' : 'text-text-2 hover:bg-chip'"
      :title="`${hit.space.name} · ${hit.entry.path}`"
      :aria-current="isSelected(hit) ? 'true' : undefined"
      :data-testid="`${testid}-recent-${hitKey(hit)}`"
      @click="emit('open', hit.space.key, hit.entry.path)"
    >
      <component
        :is="documentIcon(hit.entry.name, hit.entry.kind).icon"
        class="size-3.5 shrink-0"
        :class="documentIcon(hit.entry.name, hit.entry.kind).color"
        aria-hidden="true"
      />
      <span class="min-w-0 flex-1 truncate">{{ entryLabel(hit.entry) }}</span>
      <span
        v-if="activity.isUnopened(hit.space.key, hit.entry.path)"
        class="size-1.5 shrink-0 rounded-full bg-accent"
        aria-label="Written since you last opened it"
      />
      <span class="shrink-0 font-mono text-micro text-text-4">{{ relativeAge(entryTime(hit.entry.created)) }}</span>
    </button>
  </DriveSection>
</template>
