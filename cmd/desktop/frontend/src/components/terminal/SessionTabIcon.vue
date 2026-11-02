<script setup lang="ts">
import { computed } from 'vue'
import IconTerminal from '~icons/lucide/terminal'
import claudeCodeMark from '../../assets/agents/claude-code.svg'
import codexMark from '../../assets/agents/codex.svg'
import piMark from '../../assets/agents/pi.svg'
import AgentIcon from '../AgentIcon.vue'
import type { AgentIconID } from '../../lib/agentIcon'
import { useTerminalAgentIcons } from '../../stores/useTerminalAgentIcons'
import { isSessionViewKind, sessionView } from './panelViews'

// One mark for a tab and for its tree row, so the two read as the same thing.
const props = defineProps<{ kind: string; agent?: AgentIconID }>()

const MARKS: Partial<Record<AgentIconID, string>> = { claude: claudeCodeMark, codex: codexMark, pi: piMark }
// The full-color marks' shapes, so turning the setting on changes only the color. Pi's
// shape is already AgentIcon's. Codex's color mark is a glyph on a solid tile, which in
// one color is a plain square, so this is the glyph alone, cropped to the same size.
const MONOCHROME_MARKS: Partial<Record<AgentIconID, { viewBox: string; path: string }>> = {
  claude: {
    viewBox: '0 0 24 24',
    path: 'M20.998 10.949H24v3.102h-3v3.028h-1.487V20H18v-2.921h-1.487V20H15v-2.921H9V20H7.488v-2.921H6V20H4.487v-2.921H3V14.05H0V10.95h3V5h17.998v5.949zM6 10.949h1.488V8.102H6v2.847zm10.51 0H18V8.102h-1.49v2.847z',
  },
  codex: {
    viewBox: '3.5 3.5 17 17',
    path: 'M9.064 3.344a4.578 4.578 0 012.285-.312c1 .115 1.891.54 2.673 1.275.01.01.024.017.037.021a.09.09 0 00.043 0 4.55 4.55 0 013.046.275l.047.022.116.057a4.581 4.581 0 012.188 2.399c.209.51.313 1.041.315 1.595a4.24 4.24 0 01-.134 1.223.123.123 0 00.03.115c.594.607.988 1.33 1.183 2.17.289 1.425-.007 2.71-.887 3.854l-.136.166a4.548 4.548 0 01-2.201 1.388.123.123 0 00-.081.076c-.191.551-.383 1.023-.74 1.494-.9 1.187-2.222 1.846-3.711 1.838-1.187-.006-2.239-.44-3.157-1.302a.107.107 0 00-.105-.024c-.388.125-.78.143-1.204.138a4.441 4.441 0 01-1.945-.466 4.544 4.544 0 01-1.61-1.335c-.152-.202-.303-.392-.414-.617a5.81 5.81 0 01-.37-.961 4.582 4.582 0 01-.014-2.298.124.124 0 00.006-.056.085.085 0 00-.027-.048 4.467 4.467 0 01-1.034-1.651 3.896 3.896 0 01-.251-1.192 5.189 5.189 0 01.141-1.6c.337-1.112.982-1.985 1.933-2.618.212-.141.413-.251.601-.33.215-.089.43-.164.646-.227a.098.098 0 00.065-.066 4.51 4.51 0 01.829-1.615 4.535 4.535 0 011.837-1.388zm3.482 10.565a.637.637 0 000 1.272h3.636a.637.637 0 100-1.272h-3.636zM8.462 9.23a.637.637 0 00-1.106.631l1.272 2.224-1.266 2.136a.636.636 0 101.095.649l1.454-2.455a.636.636 0 00.005-.64L8.462 9.23z',
  },
}
const { monochrome } = useTerminalAgentIcons()
const monochromeMark = computed(() => (monochrome.value && props.agent ? MONOCHROME_MARKS[props.agent] : undefined))
const icon = computed(() => (isSessionViewKind(props.kind) ? sessionView(props.kind).icon : IconTerminal))
</script>

<template>
  <svg
    v-if="monochromeMark"
    :viewBox="monochromeMark.viewBox"
    fill="currentColor"
    aria-hidden="true"
    class="size-3.5 shrink-0"
    :data-agent-icon="agent"
    data-monochrome
  >
    <path fill-rule="evenodd" clip-rule="evenodd" :d="monochromeMark.path" />
  </svg>
  <img
    v-else-if="agent && !monochrome && MARKS[agent]"
    :src="MARKS[agent]"
    alt=""
    draggable="false"
    class="size-3.5 shrink-0 rounded-sm"
    :data-agent-icon="agent"
  />
  <AgentIcon v-else-if="agent" :id="agent" class="size-3.5 shrink-0" />
  <component :is="icon" v-else class="size-3.5 shrink-0" aria-hidden="true" />
</template>
