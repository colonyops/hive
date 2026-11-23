<script setup lang="ts">
import AppTooltip from '@hive/ui/components/ui/AppTooltip.vue'
import { computed } from 'vue'
import type { PresenceViewer } from '../../types'

const props = defineProps<{ viewers: readonly PresenceViewer[]; connected: boolean }>()

const shown = 4
const visible = computed(() => props.viewers.slice(0, shown))
const more = computed(() => props.viewers.slice(shown))

function initials(name: string): string {
  const words = name.trim().split(/\s+/).filter(Boolean)
  return (words.length > 1 ? words[0][0] + words[1][0] : (words[0] ?? '?').slice(0, 2)).toUpperCase()
}

function label(v: PresenceViewer): string {
  return v.self ? `${v.name} (you)` : v.name
}
</script>

<template>
  <div class="flex items-center gap-2" data-testid="presence">
    <span v-if="!connected" class="text-small text-text-4" data-testid="presence-reconnecting">Reconnecting</span>
    <div class="flex -space-x-1.5 transition-opacity" :class="connected ? 'opacity-100' : 'opacity-50'">
      <AppTooltip v-for="v in visible" :key="v.id" :text="label(v)">
        <span
          class="flex size-7 items-center justify-center rounded-full border-2 border-app text-micro font-semibold text-white"
          :style="{ background: `var(--presence-${v.color})` }"
          :aria-label="label(v)"
          :data-testid="`presence-${v.id}`"
        >
          {{ initials(v.name) }}
        </span>
      </AppTooltip>
      <AppTooltip v-if="more.length" :text="more.map(label).join(', ')">
        <span
          class="flex size-7 items-center justify-center rounded-full border-2 border-app bg-chip text-micro font-semibold text-text-2"
          data-testid="presence-more"
        >
          +{{ more.length }}
        </span>
      </AppTooltip>
    </div>
  </div>
</template>
