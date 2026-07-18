<script setup lang="ts">
import { computed, onMounted, onUnmounted } from 'vue'
import IconAlertTriangle from '~icons/lucide/alert-triangle'
import IconClipboardCopy from '~icons/lucide/clipboard-copy'
import IconFileCode from '~icons/lucide/file-code'
import IconX from '~icons/lucide/x'
import { tokenizeYaml } from '../lib/yamlHighlight'
import type { ConfigInfo } from '../types/feed'

const props = defineProps<{ config: ConfigInfo | null }>()
const emit = defineEmits<{ close: []; 'copy-prompt': []; 'copy-path': [] }>()

const lines = computed(() => tokenizeYaml(props.config?.yaml ?? ''))

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape') emit('close')
}

onMounted(() => window.addEventListener('keydown', onKeydown))
onUnmounted(() => window.removeEventListener('keydown', onKeydown))
</script>

<template>
  <Teleport to="body">
    <div class="fixed inset-0 z-40 bg-backdrop" data-testid="config-sheet-backdrop" @click.self="emit('close')">
      <aside
        class="absolute bottom-0 right-0 top-0 flex w-[620px] flex-col border-l border-strong bg-pane text-text shadow-[-30px_0_80px_-20px_rgba(0,0,0,.7)]"
        role="dialog"
        aria-label="Feeds as code"
        aria-modal="true"
        data-testid="config-sheet"
      >
        <header class="flex shrink-0 items-center gap-3 border-b border-row bg-pane px-6 py-5">
          <span class="flex size-7 items-center justify-center rounded-[7px] bg-accent-tint text-accent"><IconFileCode class="size-4" /></span>
          <div class="flex-1 text-lg font-semibold tracking-[-.01em]">Feeds as code</div>
          <button class="cursor-pointer text-text-3 hover:text-text" aria-label="Close" data-testid="config-sheet-close" @click="emit('close')"><IconX class="size-4.5" /></button>
        </header>

        <div class="flex min-h-0 flex-1 flex-col gap-4.5 px-6 py-5">
          <p class="shrink-0 text-[13px] leading-relaxed text-text-3">
            Profiles and feeds live in a YAML file you own — edit it by hand, keep it in dotfiles, or let a coding
            agent write it. The app reloads it the moment it changes on disk.
          </p>

          <div class="shrink-0">
            <div class="mb-1.5 text-[12.5px] text-text-2">Config file</div>
            <div class="flex gap-2">
              <div class="flex min-w-0 flex-1 items-center rounded-lg border border-card bg-app px-3 py-2 font-mono text-[12.5px] text-text-2">
                <span class="truncate" data-testid="config-sheet-path">{{ config?.path ?? '…' }}</span>
              </div>
              <button
                class="cursor-pointer whitespace-nowrap rounded-lg border border-card bg-sidebar px-3.5 py-2 text-[12.5px] text-text hover:border-strong"
                data-testid="config-sheet-copy-path"
                @click="emit('copy-path')"
              >Copy path</button>
            </div>
            <div v-if="config && !config.exists" class="mt-1.5 font-mono text-[11.5px] text-text-4">
              Not created yet — the preview below is the starting template.
            </div>
          </div>

          <div v-if="config && !config.valid" class="shrink-0 flex items-start gap-2.5 rounded-lg border border-accent/40 bg-selection px-3 py-2.5" data-testid="config-sheet-error">
            <IconAlertTriangle class="mt-0.5 size-4 shrink-0 text-accent" />
            <div class="text-xs leading-relaxed text-text-2">
              <span class="font-semibold text-accent">Config error — last good version still active.</span>
              <span class="mt-0.5 block font-mono">{{ config.error }}</span>
            </div>
          </div>
          <div v-else-if="config?.exists" class="shrink-0 flex items-center gap-1.5 text-[11.5px] text-kind-pr" data-testid="config-sheet-valid">
            <span class="size-1.5 rounded-full bg-kind-pr" />Valid · changes apply live
          </div>

          <div class="flex min-h-0 flex-1 flex-col">
            <div class="mb-1.5 shrink-0 text-[12.5px] text-text-2">{{ config?.exists ? 'Current config' : 'Starting template' }}</div>
            <pre class="hive-scroll min-h-0 flex-1 overflow-auto rounded-lg border border-row bg-app px-3.5 py-3 font-mono text-xs leading-[1.65]" data-testid="config-sheet-yaml"><code><template v-for="(line, i) in lines" :key="i"><span v-for="(token, j) in line" :key="j" :class="{
              'text-code-key': token.kind === 'key',
              'text-code-string': token.kind === 'string',
              'text-code-comment': token.kind === 'comment',
              'text-text-2': token.kind === 'plain',
            }">{{ token.text }}</span>{{ '\n' }}</template></code></pre>
          </div>

          <p class="shrink-0 text-xs leading-relaxed text-text-4">
            Each feed is one GitHub API request per poll. <span class="text-text-3">repos</span> /
            <span class="text-text-3">exclude_repos</span> globs filter client-side, so prefer one broad query plus
            filters over many narrow feeds.
          </p>
        </div>

        <footer class="flex shrink-0 gap-2.5 border-t border-row bg-raised px-6 py-3.5">
          <button
            class="flex flex-1 cursor-pointer items-center justify-center gap-2 rounded-lg bg-accent px-4 py-2.5 text-[13.5px] font-semibold text-accent-contrast hover:brightness-110"
            data-testid="config-sheet-copy-prompt"
            @click="emit('copy-prompt')"
          ><IconClipboardCopy class="size-4" />Copy prompt for coding agent</button>
          <button
            class="cursor-pointer rounded-lg border border-card px-4 py-2.5 text-[13.5px] text-text-2 hover:text-text"
            @click="emit('close')"
          >Done</button>
        </footer>
      </aside>
    </div>
  </Teleport>
</template>
