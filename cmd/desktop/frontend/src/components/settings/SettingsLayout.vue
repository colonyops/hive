<script setup lang="ts">
import { useEscapeToClose } from '../../composables/useEscapeToClose'
import ViewHeader from './ViewHeader.vue'

const emit = defineEmits<{ close: [] }>()

function close(): void {
  emit('close')
}

useEscapeToClose(close)
</script>

<template>
  <!--
    Two query containers scope every settings page's responsive behaviour (#35).
    They are declared once here so views inherit the breakpoints instead of each
    re-solving them, and a container query — not a viewport `md:` — is the right
    tool: these panes are far narrower than the window (profile rail + this nav
    take ~260px), so only the container tracks the space a control actually has.

      `settings` — this whole row. Below 700px the nav collapses to an icon rail.
      `pane`     — the content column. Rows read @[…]/pane: to stack below ~600px.
  -->
  <div class="@container/settings flex h-full min-h-0 flex-1">
    <aside class="hive-scroll w-14 shrink-0 overflow-y-auto border-r border-row bg-sidebar @[700px]/settings:w-[200px]">
      <div class="hidden border-b border-border px-4 pb-3 pt-4 @[700px]/settings:block">
        <slot name="sidebar-title" />
      </div>
      <nav class="flex flex-col gap-0.5 px-2 py-3 @[700px]/settings:px-2.5">
        <slot name="nav" />
      </nav>
    </aside>

    <section class="@container/pane flex min-w-0 flex-1 flex-col">
      <ViewHeader>
        <template #title><slot name="header-title" /></template>
      </ViewHeader>
      <!-- The pane scrolls, not the page: the nav rail and header stay put
           while content moves. Declared here so every settings page inherits
           the same gutters instead of restating them. -->
      <div class="hive-scroll min-h-0 flex-1 overflow-y-auto px-6 py-6">
        <slot />
      </div>
    </section>
  </div>
</template>
