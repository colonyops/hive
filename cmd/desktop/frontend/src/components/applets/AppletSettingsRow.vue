<script setup lang="ts">
import { computed, type DeepReadonly } from 'vue'
import IconChevronRight from '~icons/lucide/chevron-right'
import AppSwitch from '@hive/ui/components/ui/AppSwitch.vue'
import BaseBadge from '@hive/ui/components/ui/BaseBadge.vue'
import IconButton from '@hive/ui/components/ui/IconButton.vue'
import type { Applet } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/models'
import { launcherIconComponent } from '../../lib/launcherIcons'
import { DECLARED_SURFACES } from '../../stores/appletSurfaces'

const props = defineProps<{ applet: DeepReadonly<Applet>; running: number }>()
const emit = defineEmits<{ details: []; enabled: [value: boolean] }>()

const icon = computed(() => launcherIconComponent(props.applet.icon))
const usable = computed(() => props.applet.runnable && props.applet.enabled)

const status = computed<{ tone: 'danger' | 'accent' | 'neutral'; label: string } | null>(() => {
  if (!props.applet.runnable) return { tone: 'danger', label: 'Invalid' }
  if (props.applet.error) return { tone: 'accent', label: 'Running last good file' }
  if (!props.applet.enabled) return { tone: 'neutral', label: 'Off' }
  return null
})

const warningCount = computed(() => props.applet.warnings?.length ?? 0)

const shownOn = computed(() =>
  DECLARED_SURFACES.filter(({ declared }) => declared.page(props.applet))
    .map(({ declared }) => declared.label)
    .join(', '),
)
</script>

<template>
  <div class="flex min-h-13 items-center gap-1 pr-3 hover:bg-row-hover" :data-testid="`settings-applet-${applet.id}`">
    <button
      type="button"
      class="flex min-w-0 flex-1 cursor-pointer items-center gap-3 py-2.5 pl-4 pr-2 text-left outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-accent"
      :data-testid="`settings-applet-${applet.id}-details`"
      @click="emit('details')"
    >
      <component
        :is="icon"
        class="size-4 shrink-0"
        :class="usable ? 'text-text-2' : 'text-text-4'"
        aria-hidden="true"
      />
      <span class="min-w-0 flex-1">
        <span class="flex min-w-0 items-center gap-2">
          <span class="truncate text-body font-medium" :class="usable ? 'text-text' : 'text-text-3'">{{
            applet.title
          }}</span>
          <BaseBadge
            v-if="status"
            :tone="status.tone"
            variant="pill"
            class="shrink-0 px-2 py-px text-caption font-medium"
            :data-testid="`settings-applet-${applet.id}-state`"
            >{{ status.label }}</BaseBadge
          >
        </span>
        <span class="mt-0.5 flex min-w-0 items-center gap-2 text-small text-text-3">
          <span
            v-if="applet.error"
            class="truncate"
            :class="applet.runnable ? 'text-severity-warning' : 'text-severity-error'"
            :title="applet.error"
            :data-testid="`settings-applet-${applet.id}-error`"
            >{{ applet.error }}</span
          >
          <span
            v-else-if="warningCount"
            class="truncate text-severity-warning"
            :data-testid="`settings-applet-${applet.id}-warnings`"
            >{{ warningCount === 1 ? '1 warning' : `${warningCount} warnings` }}</span
          >
          <span v-else class="truncate font-mono">{{ applet.id }}</span>
          <span v-if="applet.builtin" class="shrink-0">Ships with Hive</span>
          <span v-if="running" class="shrink-0" :data-testid="`settings-applet-${applet.id}-running`"
            >{{ running }} running</span
          >
        </span>
      </span>
    </button>
    <div class="flex shrink-0 items-center gap-2">
      <span
        class="hidden max-w-40 truncate text-small text-text-3 @[600px]/pane:inline"
        :data-testid="`settings-applet-${applet.id}-surfaces`"
        >{{ shownOn || 'Nowhere' }}</span
      >
      <AppSwitch
        :aria-label="`Turn ${applet.title} on`"
        :model-value="applet.enabled"
        :testid="`settings-applet-${applet.id}-enabled`"
        @update:model-value="emit('enabled', $event)"
      />
      <IconButton
        :icon="IconChevronRight"
        :label="`Details for ${applet.title}`"
        size="md"
        tabindex="-1"
        @click="emit('details')"
      />
    </div>
  </div>
</template>
