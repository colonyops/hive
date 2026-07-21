<script setup lang="ts">
import { nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'
import IconChevronDown from '~icons/lucide/chevron-down'
import IconFolderGit2 from '~icons/lucide/folder-git-2'
import IconMonitor from '~icons/lucide/monitor'

interface MockSession {
  id: string
  name: string
  branch: string
  path: string
}

// Deliberately local fixture data for the UI-only prototype. The backend stage
// replaces this list with Hive sessions and connects the selected item to tmux.
const mockSessions: MockSession[] = [
  { id: 'terminal-mode', name: 'terminal-mode-poc', branch: 'feat/desktop-terminal-mode-poc', path: '~/src/hive' },
  { id: 'review', name: 'review-cli-output', branch: 'fix/review-cli-output', path: '~/src/hive-review' },
  { id: 'docs', name: 'docs-refresh', branch: 'chore/docs-refresh', path: '~/src/hive-docs' },
]

const selectedId = ref(mockSessions[0].id)
const terminalHost = ref<HTMLElement | null>(null)
let terminal: Terminal | undefined
let fitAddon: FitAddon | undefined
let resizeObserver: ResizeObserver | undefined

function selectedSession(): MockSession {
  return mockSessions.find((session) => session.id === selectedId.value) ?? mockSessions[0]
}

function writeDemo(session: MockSession): void {
  if (!terminal) return
  terminal.clear()
  terminal.write('\x1b[1;33mHive Terminal Mode\x1b[0m\r\n')
  terminal.write(`\x1b[2mPrototype attachment for\x1b[0m ${session.name}\r\n`)
  terminal.write(`\x1b[2mbranch:\x1b[0m ${session.branch}\r\n`)
  terminal.write(`\x1b[2mpath:\x1b[0m   ${session.path}\r\n\r\n`)
  terminal.write('\x1b[32m❯\x1b[0m Backend tmux stream will attach here. ')
}

function selectSession(id: string): void {
  selectedId.value = id
  terminal?.focus()
}

onMounted(() => {
  if (!terminalHost.value) return

  const styles = getComputedStyle(document.documentElement)
  terminal = new Terminal({
    cursorBlink: true,
    convertEol: true,
    fontFamily: '"IBM Plex Mono", monospace',
    fontSize: 13,
    lineHeight: 1.25,
    scrollback: 5_000,
    theme: {
      background: styles.getPropertyValue('--hv-app').trim() || '#181a1f',
      foreground: styles.getPropertyValue('--hv-text').trim() || '#fafafa',
      cursor: styles.getPropertyValue('--hv-accent').trim() || '#f59e0b',
      selectionBackground: styles.getPropertyValue('--hv-selection').trim() || 'rgba(245, 158, 11, 0.2)',
    },
  })
  fitAddon = new FitAddon()
  terminal.loadAddon(fitAddon)
  terminal.open(terminalHost.value)
  writeDemo(selectedSession())

  void nextTick(() => {
    fitAddon?.fit()
    terminal?.focus()
  })
  if (typeof ResizeObserver !== 'undefined') {
    resizeObserver = new ResizeObserver(() => fitAddon?.fit())
    resizeObserver.observe(terminalHost.value)
  }
})

watch(selectedId, () => writeDemo(selectedSession()))

onUnmounted(() => {
  resizeObserver?.disconnect()
  terminal?.dispose()
})
</script>

<template>
  <section class="flex min-h-0 flex-1" data-testid="terminal-mode">
    <aside class="flex w-72 shrink-0 flex-col border-r border-border bg-sidebar" aria-label="Terminal sessions">
      <div class="flex h-11 shrink-0 items-center border-b border-border px-3">
        <h1 class="text-[13px] font-semibold text-text">Sessions</h1>
        <span class="ml-auto rounded bg-chip px-1.5 py-0.5 font-mono text-[10px] text-text-3">mock</span>
      </div>
      <div class="min-h-0 flex-1 overflow-y-auto p-2">
        <div class="mb-1 flex items-center gap-1.5 px-1.5 py-1 text-[11px] font-semibold uppercase tracking-[0.08em] text-text-3">
          <IconChevronDown class="size-3" />
          Hive sessions
        </div>
        <div class="space-y-0.5 pl-2" role="tree">
          <button
            v-for="session in mockSessions"
            :key="session.id"
            type="button"
            role="treeitem"
            :aria-selected="selectedId === session.id"
            class="group flex w-full cursor-pointer items-start gap-2 rounded-md px-2 py-2 text-left"
            :class="selectedId === session.id ? 'bg-selection text-text' : 'text-text-2 hover:bg-row-hover hover:text-text'"
            :data-testid="`terminal-session-${session.id}`"
            @click="selectSession(session.id)"
          >
            <IconFolderGit2 class="mt-0.5 size-3.5 shrink-0" :class="selectedId === session.id ? 'text-accent' : 'text-text-3 group-hover:text-text-2'" />
            <span class="min-w-0">
              <span class="block truncate text-[12.5px] font-medium">{{ session.name }}</span>
              <span class="mt-0.5 block truncate font-mono text-[10.5px] text-text-3">{{ session.branch }}</span>
            </span>
          </button>
        </div>
      </div>
    </aside>

    <div class="flex min-w-0 flex-1 flex-col bg-app">
      <div class="flex h-11 shrink-0 items-center gap-2 border-b border-border bg-pane px-3">
        <IconMonitor class="size-3.5 text-accent" />
        <span class="truncate text-[12.5px] font-medium text-text">{{ selectedSession().name }}</span>
        <span class="truncate font-mono text-[10.5px] text-text-3">{{ selectedSession().path }}</span>
        <span class="ml-auto rounded border border-border px-1.5 py-0.5 font-mono text-[10px] text-text-3">UI prototype</span>
      </div>
      <div class="min-h-0 flex-1 p-3" data-terminal-input-scope>
        <div ref="terminalHost" class="h-full w-full overflow-hidden rounded-md border border-border bg-app p-2" data-testid="terminal-surface" />
      </div>
    </div>
  </section>
</template>

<style scoped>
:deep(.xterm) {
  height: 100%;
}

:deep(.xterm-viewport) {
  border-radius: 0.25rem;
}
</style>
