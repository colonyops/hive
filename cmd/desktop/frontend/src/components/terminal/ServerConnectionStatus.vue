<script setup lang="ts">
import { computed } from 'vue'
import AppTooltip from '@hive/ui/components/ui/AppTooltip.vue'

const props = defineProps<{
  servers: readonly { name: string; ssh: string; state: string; error: string; sessions: number }[]
  listError: string | null
}>()

defineEmits<{ retry: [] }>()

const details = computed(() =>
  [
    ...(props.listError ? [props.listError] : []),
    ...props.servers.map((server) =>
      [
        `${server.name} ${server.state} · ${server.sessions} ${server.sessions === 1 ? 'session' : 'sessions'}`,
        server.ssh,
        server.error,
      ]
        .filter(Boolean)
        .join('\n'),
    ),
    'Click to retry.',
  ].join('\n\n'),
)
</script>

<template>
  <AppTooltip :text="details" :delay="0">
    <button
      type="button"
      class="flex size-6 shrink-0 cursor-pointer items-center justify-center rounded-lg hover:bg-chip"
      :aria-label="details"
      data-testid="terminal-server-status"
      @click="$emit('retry')"
    >
      <span class="size-2 rounded-full bg-severity-error" aria-hidden="true" />
    </button>
  </AppTooltip>
</template>
