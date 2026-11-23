<script setup lang="ts">
import { computed, ref } from 'vue'
import IconCheck from '~icons/lucide/check'
import IconChevronDown from '~icons/lucide/chevron-down'
import IconChevronRight from '~icons/lucide/chevron-right'
import IconCode from '~icons/lucide/code'
import IconCopy from '~icons/lucide/copy'
import { useClipboard } from '../../composables/useClipboard'
import IconButton from '../ui/IconButton.vue'

const props = withDefaults(
  defineProps<{
    /** Small caps label in the header, such as "Tasks" or "Files". */
    kind: string
    title?: string
    /** Muted text in the header; the `summary` slot replaces it. */
    summary?: string
    /** The raw fence body, which Source shows and Copy puts on the clipboard. */
    source: string
    /** The fence info string. Source shows it on the opening line when set. */
    lang?: string
    /** Hides the Collapse control, for a body that is already one line. */
    collapsible?: boolean
    /** Derives `-source-toggle`, `-copy`, `-collapse`, `-source` and `-body`. */
    testid: string
  }>(),
  { title: undefined, summary: undefined, lang: undefined, collapsible: true },
)

const collapsed = defineModel<boolean>('collapsed', { default: false })
const showSource = ref(false)
const { copy, copied } = useClipboard()

const sourceText = computed(() => {
  const body = props.source.replace(/\n+$/, '')
  return props.lang ? `\`\`\`${props.lang}\n${body}\n\`\`\`` : body
})
</script>

<template>
  <section class="fence-card" :class="{ 'is-collapsed': collapsed }" :data-testid="testid">
    <header class="fence-card-head">
      <span class="fence-card-kind">{{ kind }}</span>
      <span v-if="title" class="fence-card-title" :title="title">{{ title }}</span>
      <span class="fence-card-meta">
        <slot name="summary">{{ summary }}</slot>
      </span>
      <span class="fence-card-tools">
        <slot name="tools" />
        <IconButton
          :icon="IconCode"
          :label="showSource ? 'Show rendered' : 'Show source'"
          size="md"
          :active="showSource"
          :aria-pressed="showSource"
          :data-testid="`${testid}-source-toggle`"
          @click="showSource = !showSource"
        />
        <IconButton
          :icon="copied ? IconCheck : IconCopy"
          :label="copied ? 'Copied' : 'Copy source'"
          size="md"
          :data-testid="`${testid}-copy`"
          @click="copy(source)"
        />
        <IconButton
          v-if="collapsible"
          :icon="collapsed ? IconChevronRight : IconChevronDown"
          :label="collapsed ? 'Expand' : 'Collapse'"
          size="md"
          :aria-expanded="!collapsed"
          :data-testid="`${testid}-collapse`"
          @click="collapsed = !collapsed"
        />
      </span>
    </header>
    <template v-if="!collapsed">
      <pre v-if="showSource" class="fence-card-source" :data-testid="`${testid}-source`">{{ sourceText }}</pre>
      <div v-else class="fence-card-body" :data-testid="`${testid}-body`">
        <slot />
      </div>
      <slot name="footer" />
    </template>
  </section>
</template>

<style scoped>
.fence-card {
  overflow: hidden;
  border: 1px solid var(--color-border);
  border-radius: 8px;
  background: var(--color-app);
}
.fence-card-head {
  display: flex;
  align-items: center;
  gap: 10px;
  min-height: 30px;
  padding: 3px 6px 3px 12px;
  background: var(--color-raised);
}
.fence-card:not(.is-collapsed) .fence-card-head {
  border-bottom: 1px solid var(--color-border);
}
.fence-card-kind {
  flex: none;
  color: var(--color-text-3);
  font: 600 10px/1 var(--font-mono);
  letter-spacing: 0.1em;
  text-transform: uppercase;
}
.fence-card-title {
  min-width: 0;
  overflow: hidden;
  font-size: 13px;
  font-weight: 500;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.fence-card-meta {
  display: flex;
  flex: none;
  align-items: center;
  gap: 8px;
  margin-left: auto;
  color: var(--color-text-3);
  font: 11.5px/1 var(--font-mono);
}
.fence-card-tools {
  display: flex;
  flex: none;
  gap: 2px;
}
.fence-card-source {
  margin: 0;
  padding: 10px 12px;
  overflow-x: auto;
  background: var(--color-pane);
  color: var(--color-text-2);
  font: 12px/1.55 var(--font-mono);
}
</style>
