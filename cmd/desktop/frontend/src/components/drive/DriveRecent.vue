<script setup lang="ts">
// The newest documents across every space, above the tree, so something an
// agent just wrote is found without knowing which space or folder holds it.
import { useStorage } from '@vueuse/core'
import { onMounted } from 'vue'
import IconChevronRight from '~icons/lucide/chevron-right'
import InlineError from '@hive/ui/components/ui/InlineError.vue'
import { entryTime } from '@hive/ui/lib/document'
import { documentIcon } from './documentIcon'
import { relativeAge } from '../../lib/age'
import { entryLabel } from '../../lib/drive'
import { useDrive } from '../../stores/useDrive'
import { useDriveActivity } from '../../stores/useDriveActivity'
import type { HitWire } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'

const props = defineProps<{
  /** The space and path on screen, marked when listed. */
  space: string
  selected: string | null
  /** Derives `-recent`, `-recent-toggle`, `-recent-error`, and `-recent-<space>:<path>`. */
  testid: string
}>()
const emit = defineEmits<{ open: [space: string, path: string] }>()

const { recent, recentError, ensureRecent } = useDrive()
const activity = useDriveActivity()
const open = useStorage('hive.drive.recent.open', true)

onMounted(() => void ensureRecent())

const hitKey = (hit: HitWire) => `${hit.space.key}:${hit.entry.path}`
const isSelected = (hit: HitWire) => hit.space.key === props.space && hit.entry.path === props.selected
</script>

<template>
  <section v-if="recent.length || recentError" class="border-b border-border pb-1" :data-testid="`${testid}-recent`">
    <button
      type="button"
      class="flex h-7 w-full cursor-pointer items-center gap-1.5 px-2 text-caption font-medium text-text-3 hover:text-text-2"
      :aria-expanded="open"
      :data-testid="`${testid}-recent-toggle`"
      @click="open = !open"
    >
      <IconChevronRight
        class="size-3 shrink-0 transition-transform"
        :class="open ? 'rotate-90' : ''"
        aria-hidden="true"
      />
      Recent
    </button>
    <template v-if="open">
      <InlineError
        v-if="recentError"
        variant="line"
        class="px-3 py-1 leading-relaxed"
        :message="recentError"
        :testid="`${testid}-recent-error`"
      />
      <button
        v-for="hit in recent"
        :key="hitKey(hit)"
        type="button"
        class="flex h-[26px] w-full cursor-pointer items-center gap-1.5 pl-[22px] pr-2 text-left text-small"
        :class="isSelected(hit) ? 'bg-selection text-text' : 'text-text-2 hover:bg-chip'"
        :title="`${hit.space.name} · ${hit.entry.path}`"
        :aria-current="isSelected(hit) ? 'true' : undefined"
        :data-testid="`${testid}-recent-${hitKey(hit)}`"
        @click="emit('open', hit.space.key, hit.entry.path)"
      >
        <component
          :is="documentIcon(hit.entry.kind)"
          class="size-3.5 shrink-0"
          :class="isSelected(hit) ? 'text-accent' : 'text-text-4'"
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
    </template>
  </section>
</template>
