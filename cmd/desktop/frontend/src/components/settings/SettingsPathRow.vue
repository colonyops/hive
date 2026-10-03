<script setup lang="ts">
// A single on-disk location row inside a grouped settings card: a
// leading type icon, the label and monospace path on one line, and quiet
// icon-only actions. Copy is handled locally (no backend); open/reveal/
// change/reset are emitted for the parent to route through the SystemService.
// `editable` adds the Change… button and, when overridden, the Reset action
// used by the data and config directories.
import { computed } from 'vue'
import IconCheck from '~icons/lucide/check'
import IconCopy from '~icons/lucide/copy'
import IconDatabase from '~icons/lucide/database'
import IconExternalLink from '~icons/lucide/external-link'
import IconFileText from '~icons/lucide/file-text'
import IconFolder from '~icons/lucide/folder'
import IconFolderOpen from '~icons/lucide/folder-open'
import IconRotateCcw from '~icons/lucide/rotate-ccw'
import { useClipboard } from '../../composables/useClipboard'
import IconButton from '../ui/IconButton.vue'
import BaseBadge from '../ui/BaseBadge.vue'
import BaseIconBadge from '../ui/BaseIconBadge.vue'
import BaseButton from '../ui/BaseButton.vue'

const props = withDefaults(
  defineProps<{
    label: string
    path: string
    hint?: string
    icon?: 'folder' | 'log' | 'database'
    tone?: 'accent' | 'neutral'
    exists?: boolean
    overridden?: boolean
    overriddenLabel?: string
    editable?: boolean
    canOpen?: boolean
    canReveal?: boolean
    testid?: string
  }>(),
  {
    icon: 'folder',
    tone: 'neutral',
    exists: true,
    overridden: false,
    overriddenLabel: 'Custom',
    editable: false,
    canOpen: true,
    canReveal: true,
  },
)
const emit = defineEmits<{ open: []; reveal: []; change: []; reset: [] }>()

const { copy, status: copyStatus } = useClipboard()

const typeIcon = computed(() => ({ folder: IconFolder, log: IconFileText, database: IconDatabase })[props.icon])
const copyLabel = computed(() =>
  copyStatus.value === 'success' ? 'Copied' : copyStatus.value === 'error' ? 'Copy failed' : 'Copy path',
)
</script>

<template>
  <article
    class="flex flex-col gap-2.5 px-4 py-3 transition-colors hover:bg-row-hover @[600px]/pane:flex-row @[600px]/pane:items-center @[600px]/pane:gap-3.5"
    :data-testid="props.testid"
  >
    <div class="flex min-w-0 flex-1 items-center gap-3.5">
      <BaseIconBadge
        :size="32"
        rounded="rounded-lg"
        :class="
          props.tone === 'accent'
            ? 'border border-accent/35 bg-accent-tint text-accent'
            : 'border border-card bg-chip text-text-2'
        "
      >
        <component :is="typeIcon" class="size-4" />
      </BaseIconBadge>

      <div class="min-w-0 flex-1">
        <div class="flex flex-wrap items-baseline gap-x-2.5 gap-y-0.5">
          <span class="shrink-0 text-body font-semibold text-text">{{ props.label }}</span>
          <BaseBadge
            v-if="props.overridden"
            variant="pill"
            class="shrink-0 px-2 py-0.5 text-micro font-semibold"
            :data-testid="props.testid ? `${props.testid}-overridden` : undefined"
            >{{ props.overriddenLabel }}</BaseBadge
          >
          <BaseBadge
            v-if="!props.exists"
            tone="muted"
            variant="pill"
            class="shrink-0 px-2 py-0.5 text-micro font-medium"
            >Not created yet</BaseBadge
          >
          <span
            class="min-w-0 max-w-full truncate font-mono text-xs text-text-2"
            :title="props.path"
            :data-testid="props.testid ? `${props.testid}-path` : undefined"
            >{{ props.path }}</span
          >
        </div>
        <p v-if="props.hint" class="mt-0.5 text-caption text-text-3">{{ props.hint }}</p>
      </div>
    </div>

    <div class="flex shrink-0 flex-wrap items-center justify-end gap-1">
      <IconButton
        :label="copyLabel"
        :icon="copyStatus === 'success' ? IconCheck : IconCopy"
        size="lg"
        :data-testid="props.testid ? `${props.testid}-copy` : undefined"
        @click="copy(props.path)"
      />
      <IconButton
        v-if="props.canOpen"
        label="Open"
        :icon="IconExternalLink"
        size="lg"
        :data-testid="props.testid ? `${props.testid}-open` : undefined"
        @click="emit('open')"
      />
      <IconButton
        v-if="props.canReveal"
        label="Reveal"
        :icon="IconFolderOpen"
        size="lg"
        :data-testid="props.testid ? `${props.testid}-reveal` : undefined"
        @click="emit('reveal')"
      />

      <template v-if="props.editable">
        <IconButton
          v-if="props.overridden"
          label="Reset to default"
          :icon="IconRotateCcw"
          size="lg"
          :data-testid="props.testid ? `${props.testid}-reset` : undefined"
          @click="emit('reset')"
        />
        <BaseButton
          variant="secondary"
          size="xs"
          class="ml-1"
          :data-testid="props.testid ? `${props.testid}-change` : undefined"
          @click="emit('change')"
        >
          Change…
        </BaseButton>
      </template>
    </div>
  </article>
</template>
