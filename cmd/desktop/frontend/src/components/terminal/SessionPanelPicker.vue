<script setup lang="ts">
import IconPlus from '~icons/lucide/plus'
import IconTerminal from '~icons/lucide/terminal'
import { sessionView, type SessionViewKind } from './panelViews'

defineProps<{
  windows: { windowId: string; name: string }[]
  agents: readonly string[]
  views: readonly SessionViewKind[]
  canAddTerminal: boolean
}>()
const emit = defineEmits<{
  window: [windowId: string]
  terminal: []
  agent: [agent: string]
  view: [kind: SessionViewKind]
  cancel: []
}>()

const heading = 'mb-1 mt-3 font-mono text-micro font-semibold uppercase tracking-wider text-text-3'
const row =
  'flex h-[27px] w-full max-w-[280px] cursor-pointer items-center gap-2 rounded px-2 text-left text-small text-text-2 hover:bg-chip hover:text-text'
</script>

<template>
  <div class="hive-scroll min-h-0 flex-1 overflow-auto bg-app p-3.5" data-testid="panel-picker">
    <h4 class="m-0 text-small font-semibold text-text">What goes here?</h4>

    <template v-if="windows.length">
      <div :class="heading">Open windows</div>
      <button
        v-for="win in windows"
        :key="win.windowId"
        type="button"
        :class="row"
        :data-testid="`panel-picker-window-${win.windowId}`"
        @click="emit('window', win.windowId)"
      >
        <IconTerminal class="size-3.5 shrink-0" aria-hidden="true" />
        <span class="min-w-0 truncate">{{ win.name }}</span>
        <small class="ml-auto shrink-0 text-micro text-text-4">not on screen</small>
      </button>
    </template>

    <template v-if="canAddTerminal">
      <div :class="heading">New terminal</div>
      <button type="button" :class="row" data-testid="panel-picker-terminal" @click="emit('terminal')">
        <IconPlus class="size-3.5 shrink-0" aria-hidden="true" />New terminal
      </button>
      <button
        v-for="agent in agents"
        :key="agent"
        type="button"
        :class="row"
        :data-testid="`panel-picker-agent-${agent}`"
        @click="emit('agent', agent)"
      >
        <IconPlus class="size-3.5 shrink-0" aria-hidden="true" />New {{ agent }} agent
      </button>
    </template>

    <template v-if="views.length">
      <div :class="heading">Session</div>
      <button
        v-for="view in views"
        :key="view"
        type="button"
        :class="row"
        :data-testid="`panel-picker-view-${view}`"
        @click="emit('view', view)"
      >
        <component :is="sessionView(view).icon" class="size-3.5 shrink-0" aria-hidden="true" />{{
          sessionView(view).label
        }}
      </button>
    </template>

    <button
      type="button"
      class="mt-3 cursor-pointer rounded px-2 py-1 text-caption text-text-3 hover:bg-chip hover:text-text"
      data-testid="panel-picker-cancel"
      @click="emit('cancel')"
    >
      Cancel split
    </button>
  </div>
</template>
