<script setup lang="ts">
import IconBan from '~icons/lucide/ban'
import IconLayers from '~icons/lucide/layers'
import IconTerminal from '~icons/lucide/terminal'
import BaseBadge from '@hive/ui/components/ui/BaseBadge.vue'
import type { TaskItem } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'
import { relativeAge } from '../../lib/age'
import TaskStatusIcon from './TaskStatusIcon.vue'

defineProps<{
  item: TaskItem
  selected: boolean
  dragging: boolean
  counts?: { done: number; total: number }
  parentTitle?: string
  sessionName?: string
}>()
const emit = defineEmits<{ select: [id: string]; dragstart: [event: DragEvent]; dragend: [] }>()
</script>

<template>
  <div
    class="flex cursor-pointer flex-col gap-2 rounded-lg border bg-raised px-3 py-2.5 text-body shadow-sm transition-[border-color,opacity]"
    :class="[selected ? 'border-accent' : 'border-card hover:border-strong', dragging ? 'opacity-40' : '']"
    role="button"
    tabindex="0"
    draggable="true"
    :aria-selected="selected"
    :data-id="item.id"
    data-testid="task-card"
    @click="emit('select', item.id)"
    @keydown.enter.prevent="emit('select', item.id)"
    @dragstart="emit('dragstart', $event)"
    @dragend="emit('dragend')"
  >
    <div class="flex items-center gap-1.5 font-mono text-caption text-text-4">
      <span class="min-w-0 flex-1 truncate">{{ item.id }}</span>
      <IconTerminal
        v-if="item.sessionId"
        class="size-3 shrink-0"
        :aria-label="`Session ${sessionName || item.sessionId}`"
        data-testid="task-card-session"
      />
      <span :title="new Date(item.updatedAt).toLocaleString()">{{ relativeAge(Date.parse(item.updatedAt)) }}</span>
    </div>

    <div class="flex items-start gap-2">
      <TaskStatusIcon :status="item.status" class="mt-[3px]" />
      <span
        class="line-clamp-2 min-w-0 flex-1 leading-[1.4]"
        :class="item.type === 'epic' ? 'font-medium text-text' : 'text-text-2'"
        data-testid="task-card-title"
        >{{ item.title }}</span
      >
    </div>

    <div v-if="item.type === 'epic' || parentTitle || item.blocked" class="flex flex-wrap items-center gap-1.5">
      <span
        v-if="item.type === 'epic'"
        class="inline-flex items-center gap-1 rounded-full bg-accent-tint px-2 py-0.5 text-caption font-medium text-accent"
        data-testid="task-card-epic"
      >
        <IconLayers class="size-2.5" aria-hidden="true" />Epic<template v-if="counts">
          · {{ counts.done }}/{{ counts.total }}</template
        >
      </span>
      <span
        v-if="parentTitle"
        class="inline-flex min-w-0 max-w-full items-center gap-1 rounded-full border border-card px-2 py-0.5 text-caption text-text-3"
        data-testid="task-card-parent"
      >
        <IconLayers class="size-2.5 shrink-0 text-text-4" aria-hidden="true" /><span class="truncate">{{
          parentTitle
        }}</span>
      </span>
      <BaseBadge
        v-if="item.blocked"
        tone="danger"
        variant="pill"
        class="px-2 py-0.5 text-caption font-medium"
        data-testid="task-card-blocked"
      >
        <IconBan class="size-2.5" aria-hidden="true" />Blocked
      </BaseBadge>
    </div>
  </div>
</template>
